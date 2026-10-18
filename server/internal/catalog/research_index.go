package catalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/klauspost/compress/zstd"
)

//go:embed research.sql
var researchSchema string

// Source identity is independent of catalogue key, sample/full status, ingest order
// and decoder build. Sorted original component names and SHA-256s define version 1.
func archiveIdentity(manifest Manifest) string {
	if len(manifest.SHA256) == 0 {
		return ""
	}
	names := make([]string, 0, len(manifest.SHA256))
	for name, value := range manifest.SHA256 {
		decoded, e := hex.DecodeString(value)
		if e != nil || len(decoded) != 32 {
			return ""
		}
		names = append(names, name)
	}
	sort.Strings(names)
	pairs := make([][2]string, 0, len(names))
	for _, name := range names {
		pairs = append(pairs, [2]string{name, strings.ToLower(manifest.SHA256[name])})
	}
	raw, _ := json.Marshal(pairs)
	hash := sha256.Sum256(append([]byte("ukinfo-source-v1\x00"), raw...))
	return hex.EncodeToString(hash[:])
}
func sourceHash(source string) []byte { h := sha256.Sum256([]byte(source)); return h[:] }
func listingKey(row map[string]any, archive, source, dataset string) ([]byte, error) {
	p, reason := premises(row)
	// A warning/incomplete address is always its own occurrence, including when a
	// display field happens to agree. Do not infer a person/household identity.
	var raw []byte
	if reason != "" {
		raw, _ = json.Marshal([]string{"separate-v1", archive, dataset, source})
	} else {
		name := Normalize(Text(row, "surname"))
		first := Normalize(Text(row, "firstName"))
		kind := Text(row, "entry_type")
		if kind == "business" {
			name = Normalize(Text(row, "organisation"))
			first = ""
		}
		if name == "" || (kind != "business" && first == "") {
			raw, _ = json.Marshal([]string{"separate-v1", archive, dataset, source})
		} else {
			raw, _ = json.Marshal([]any{"listing-v1", kind, name, first, Normalize(Text(row, "initial2")), p, Normalize(Text(row, "phone")), Normalize(Text(row, "industry")), Normalize(Text(row, "sic_code"))})
		}
	}
	h := sha256.Sum256(raw)
	return h[:], nil
}

type ResearchIndexReport struct {
	Dataset    string  `json:"dataset"`
	Records    int64   `json:"records"`
	Businesses int64   `json:"businesses"`
	Bytes      int64   `json:"bytes"`
	Seconds    float64 `json:"seconds"`
}

// IndexResearch walks bounded source-block windows and publishes one complete,
// count-checked partition atomically. Cancellation rolls back its new partition.
func IndexResearch(ctx context.Context, db *pgxpool.Pool, key string, options AddressIndexOptions) (report ResearchIndexReport, err error) {
	if options.SampleBlocks != 0 {
		return report, errors.New("research index builds must cover the complete imported scope")
	}
	ctx, cancel := context.WithCancelCause(ctx)
	var watcherDone chan struct{}
	defer func() {
		cancel(nil)
		if watcherDone != nil {
			<-watcherDone
		}
		if err != nil && context.Cause(ctx) != nil && !errors.Is(context.Cause(ctx), context.Canceled) {
			err = context.Cause(ctx)
		}
	}()
	if options.CheckResources != nil {
		original := options.CheckResources
		var mutex sync.Mutex
		options.CheckResources = func() error { mutex.Lock(); defer mutex.Unlock(); return original() }
		watcherDone = make(chan struct{})
		go func() {
			defer close(watcherDone)
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if e := options.CheckResources(); e != nil {
						cancel(e)
						return
					}
				}
			}
		}()
	}

	started := time.Now()
	report.Dataset = key
	tx, err := db.Begin(ctx)
	if err != nil {
		return report, err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SET LOCAL statement_timeout=0; SET LOCAL work_mem='32MB'; SET LOCAL maintenance_work_mem='64MB'"); err != nil {
		return report, err
	}
	for _, sql := range []string{"SELECT pg_try_advisory_xact_lock(742918304)", "SELECT pg_try_advisory_xact_lock(742918306,hashtext(current_schema()))"} {
		var locked bool
		if err = tx.QueryRow(ctx, sql).Scan(&locked); err != nil {
			return report, err
		}
		if !locked {
			return report, errors.New("an import or index build is running")
		}
	}
	var dataset int16
	var expected int64
	var rawManifest []byte
	if err = tx.QueryRow(ctx, "SELECT id,entries,manifest FROM datasets WHERE key=$1 FOR UPDATE", key).Scan(&dataset, &expected, &rawManifest); err != nil {
		return report, err
	}
	var ready bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM research_indexes WHERE dataset_id=$1)", dataset).Scan(&ready); err != nil {
		return report, err
	}
	if ready {
		return report, errors.New("edition already has a research index")
	}
	var manifest Manifest
	if err = json.Unmarshal(rawManifest, &manifest); err != nil {
		return report, err
	}
	archive := archiveIdentity(manifest)
	name := fmt.Sprintf("research_d%d", dataset)
	if _, err = tx.Exec(ctx, "CREATE TABLE "+pgx.Identifier{name}.Sanitize()+" (LIKE research_keys INCLUDING DEFAULTS INCLUDING CONSTRAINTS, CHECK(dataset_id="+fmt.Sprint(dataset)+"))"); err != nil {
		return report, err
	}
	decoder, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(16<<20))
	if err != nil {
		return report, err
	}
	defer decoder.Close()
	check := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if options.CheckResources != nil {
			return options.CheckResources()
		}
		return nil
	}
	blocks := (expected + blockRows - 1) / blockRows
	for start := int64(0); start < blocks; start += 128 {
		if err = check(); err != nil {
			return report, err
		}
		rows, e := tx.Query(ctx, "SELECT id,payload FROM record_blocks WHERE dataset_id=$1 AND id >= $2 AND id < $3 ORDER BY id", dataset, start, min(start+128, blocks))
		if e != nil {
			return report, e
		}
		type block struct {
			id      int32
			payload []byte
		}
		batch := []block{}
		for rows.Next() {
			var b block
			if e = rows.Scan(&b.id, &b.payload); e != nil {
				break
			}
			batch = append(batch, b)
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			return report, e
		}
		if int64(len(batch)) != min(int64(128), blocks-start) {
			return report, errors.New("missing source block during research indexing")
		}
		values := make([][]any, 0, len(batch)*blockRows)
		for _, b := range batch {
			raw, e := unpackBlock(decoder, b.payload)
			if e != nil {
				return report, e
			}
			if int64(binary.LittleEndian.Uint16(raw[5:7])) != min(int64(blockRows), expected-int64(b.id)*blockRows) {
				return report, errors.New("source block row count differs from catalogue")
			}
			position := 7
			for offset := 0; offset < blockRows && int64(b.id)*blockRows+int64(offset) < expected; offset++ {
				sourceLength := int(binary.LittleEndian.Uint16(raw[position:]))
				recordLength := int(binary.LittleEndian.Uint32(raw[position+2:]))
				position += 6
				record := storedRecord{SourceID: string(raw[position : position+sourceLength]), Record: raw[position+sourceLength : position+sourceLength+recordLength]}
				position += sourceLength + recordLength
				var row map[string]any
				d := json.NewDecoder(bytes.NewReader(record.Record))
				d.UseNumber()
				if e = d.Decode(&row); e != nil {
					return report, e
				}
				group, e := listingKey(row, archive, record.SourceID, key)
				if e != nil {
					return report, e
				}
				sic, industry := "", ""
				if Text(row, "entry_type") == "business" {
					sic = Normalize(Text(row, "sic_code"))
					industry = Normalize(Text(row, "industry"))
					report.Businesses++
				}
				if len(sic) > 256 || len(industry) > 2048 || strings.ContainsRune(sic+industry, 0) {
					return report, errors.New("invalid resolved business classification")
				}
				values = append(values, []any{dataset, b.id*blockRows + int32(offset) + 1, sourceHash(record.SourceID), group, sic, industry})
				report.Records++
			}
		}
		if _, err = tx.CopyFrom(ctx, pgx.Identifier{name}, []string{"dataset_id", "id", "source_hash", "group_key", "sic", "industry"}, pgx.CopyFromRows(values)); err != nil {
			return report, err
		}
		if options.Progress != nil {
			options.Progress(Progress{"indexing research", report.Records, expected})
		}
	}
	if report.Records != expected {
		return report, errors.New("research index count differs from source")
	}
	if err = check(); err != nil {
		return report, err
	}

	if options.Progress != nil {
		options.Progress(Progress{"building research indexes", report.Records, expected})
	}
	if _, err = tx.Exec(ctx, "ALTER TABLE research_keys ATTACH PARTITION "+pgx.Identifier{name}.Sanitize()+" FOR VALUES IN ("+fmt.Sprint(dataset)+")"); err != nil {
		return report, err
	}
	if _, err = tx.Exec(ctx, "ANALYZE "+pgx.Identifier{name}.Sanitize()); err != nil {
		return report, err
	}
	if err = tx.QueryRow(ctx, "SELECT pg_total_relation_size($1::regclass)", pgx.Identifier{name}.Sanitize()).Scan(&report.Bytes); err != nil {
		return report, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO research_indexes(dataset_id,version,entries) VALUES($1,1,$2)", dataset, expected); err != nil {
		return report, err
	}
	if err = tx.Commit(ctx); err != nil {
		return report, err
	}
	report.Seconds = time.Since(started).Seconds()
	return report, nil
}
