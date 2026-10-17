package catalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/klauspost/compress/zstd"
)

type Verification struct {
	Dataset             string           `json:"dataset"`
	Entries             int64            `json:"entries"`
	Blocks              int64            `json:"blocks"`
	SHA256              string           `json:"payload_sha256"`
	Seconds             float64          `json:"seconds"`
	ProjectionChecked   bool             `json:"projection_checked"`
	SourceDigestMatched bool             `json:"source_digest_matched"`
	EntryTypes          map[string]int64 `json:"entry_types"`
	WarningEntries      int64            `json:"entries_with_source_warnings"`
	EmptyPostcodes      int64            `json:"entries_without_displayed_postcode"`
}

// Verify checks every compressed block and every indexed key against its original
// JSON record, not just row counts. Vocabulary memory is proportional to distinct
// terms, not occurrences; it is an explicit offline operation, not an HTTP query.
func Verify(ctx context.Context, db *pgxpool.Pool, key string, progress func(Progress)) (Verification, error) {
	started := time.Now()
	report := Verification{Dataset: key, EntryTypes: map[string]int64{}}
	var dataset int16
	var expected int64
	var rawStats []byte
	if e := db.QueryRow(ctx, "SELECT id,entries,stats FROM datasets WHERE key=$1", key).Scan(&dataset, &expected, &rawStats); e != nil {
		return report, e
	}
	var stats ImportStats
	if e := json.Unmarshal(rawStats, &stats); e != nil {
		return report, e
	}
	terms := map[int32]string{}
	rows, e := db.Query(ctx, "SELECT id,key FROM terms")
	if e != nil {
		return report, e
	}
	for rows.Next() {
		var id int32
		var key string
		if e = rows.Scan(&id, &key); e != nil {
			rows.Close()
			return report, e
		}
		terms[id] = key
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return report, e
	}
	projection, e := db.Query(ctx, "SELECT id,kind,name_id,first_id,postcode_id FROM entries WHERE dataset_id=$1 ORDER BY id", dataset)
	if e != nil {
		return report, e
	}
	defer projection.Close()
	blocks, e := db.Query(ctx, "SELECT id,payload FROM record_blocks WHERE dataset_id=$1 ORDER BY id", dataset)
	if e != nil {
		return report, e
	}
	defer blocks.Close()
	decoder, e := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(16<<20))
	if e != nil {
		return report, e
	}
	defer decoder.Close()
	digest := sha256.New()
	for blocks.Next() {
		if e = ctx.Err(); e != nil {
			return report, e
		}
		var block int32
		var compressed []byte
		if e = blocks.Scan(&block, &compressed); e != nil {
			return report, e
		}
		if int64(block) != report.Blocks {
			return report, fmt.Errorf("nonconsecutive source block")
		}
		raw, err := unpackBlock(decoder, compressed)
		if err != nil {
			return report, err
		}
		digest.Write(raw[7:])
		n := int(binary.LittleEndian.Uint16(raw[5:7]))
		if n != blockRows && report.Entries+int64(n) != expected {
			return report, fmt.Errorf("short nonterminal source block")
		}
		for offset := 0; offset < n; offset++ {
			r, err := recordAt(raw, offset)
			if err != nil {
				return report, err
			}
			var original map[string]any
			d := json.NewDecoder(bytes.NewReader(r.Record))
			d.UseNumber()
			if err = d.Decode(&original); err != nil {
				return report, err
			}
			if !projection.Next() {
				return report, fmt.Errorf("missing search projection: %w", projection.Err())
			}
			var id, name, first, postcode int32
			var kind int16
			if err = projection.Scan(&id, &kind, &name, &first, &postcode); err != nil {
				return report, err
			}
			wantName, wantFirst, wantKind := Normalize(Text(original, "surname")), Normalize(Text(original, "firstName")), int16(0)
			if Text(original, "entry_type") == "business" {
				wantName = Normalize(Text(original, "organisation"))
				wantFirst = ""
				wantKind = 1
			}
			entryType := Text(original, "entry_type")
			switch entryType {
			case "electoral", "telephone", "business":
			default:
				entryType = "directory"
			}
			report.EntryTypes[entryType]++
			if Text(original, "source_warning") != "" {
				report.WarningEntries++
			}
			if Text(original, "postcode") == "" {
				report.EmptyPostcodes++
			}
			report.Entries++
			a, aok := terms[name]
			b, bok := terms[first]
			c, cok := terms[postcode]
			if int64(id) != report.Entries || kind != wantKind || !aok || !bok || !cok || a != wantName || b != wantFirst || c != Postcode(Text(original, "postcode")) {
				return report, fmt.Errorf("search projection differs at ordinal %d", report.Entries)
			}
		}
		report.Blocks++
		if progress != nil && report.Blocks%2048 == 0 {
			progress(Progress{"verifying", report.Entries, expected})
		}
	}
	if e = blocks.Err(); e != nil {
		return report, e
	}
	if projection.Next() {
		return report, fmt.Errorf("extra search projection rows")
	}
	if e = projection.Err(); e != nil {
		return report, e
	}
	report.SHA256 = hex.EncodeToString(digest.Sum(nil))
	if report.Entries != expected || report.Blocks != stats.Blocks || report.SHA256 != stats.SHA256 {
		return report, fmt.Errorf("stored counts or source digest differ")
	}
	report.Seconds = time.Since(started).Seconds()
	report.ProjectionChecked = true
	report.SourceDigestMatched = true
	return report, nil
}
