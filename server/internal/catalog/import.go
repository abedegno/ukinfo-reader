package catalog

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/klauspost/compress/zstd"
)

const SchemaVersion = 2
const importBatchRows = 65536

//go:embed schema.sql
var schema string

func Migrate(ctx context.Context, db *pgxpool.Pool) error {
	tx, e := db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(742918303)"); e != nil {
		return e
	}
	var existing *string
	if e = tx.QueryRow(ctx, "SELECT to_regclass('catalog_schema')::text").Scan(&existing); e != nil {
		return e
	}
	if existing != nil {
		var version int
		if e = tx.QueryRow(ctx, "SELECT max(version) FROM catalog_schema").Scan(&version); e != nil {
			return e
		}
		if version != SchemaVersion {
			return fmt.Errorf("catalogue schema %d differs from %d; use a separate database and repack-v1 or re-import (existing data is unchanged)", version, SchemaVersion)
		}
	}
	if e = migrateGeography(ctx, tx); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, schema); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, addressSchema); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, addressLookupSchema); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, researchSchema); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

type Manifest struct {
	Type          string            `json:"type"`
	Protocol      int               `json:"protocol"`
	Dataset       string            `json:"dataset"`
	Edition       string            `json:"edition"`
	Title         string            `json:"title"`
	ReaderVersion string            `json:"reader_version"`
	SHA256        map[string]string `json:"sha256"`
	Sample        bool              `json:"sample"`
	Expected      int64             `json:"expected"`
}
type frame struct {
	Type     string          `json:"type"`
	SourceID string          `json:"source_id"`
	Record   json.RawMessage `json:"record"`
	Count    int64           `json:"count"`
	SHA256   string          `json:"sha256"`
}

func Text(row map[string]any, key string) string { s, _ := row[key].(string); return s }
func Normalize(s string) string                  { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
func Postcode(s string) string                   { return strings.ToUpper(strings.Join(strings.Fields(s), "")) }

type Progress struct {
	Phase string `json:"phase"`
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
}
type ImportOptions struct {
	Progress func(Progress)
	// IndexAddresses builds the optional index after source ingestion commits.
	// Failure returns the committed count and an error; source records remain.
	IndexAddresses bool
	CheckResources func() error
}
type projected struct {
	id                    int32
	kind                  int16
	name, first, postcode string
}
type ImportStats struct {
	SHA256          string  `json:"payload_sha256"`
	RawBytes        int64   `json:"raw_record_bytes"`
	CompressedBytes int64   `json:"compressed_block_bytes"`
	Blocks          int64   `json:"blocks"`
	LoadSeconds     float64 `json:"load_seconds"`
	ValidateSeconds float64 `json:"identity_validation_seconds"`
	IndexSeconds    float64 `json:"index_seconds"`
	Seconds         float64 `json:"seconds"`
}

func Import(ctx context.Context, db *pgxpool.Pool, input io.Reader) (int64, error) {
	return ImportWithOptions(ctx, db, input, ImportOptions{})
}

// ImportWithOptions keeps large new tables unattached until validation and indexing
// finish. Existing datasets remain readable, and rollback unlinks the new files.
func ImportWithOptions(ctx context.Context, db *pgxpool.Pool, input io.Reader, options ImportOptions) (int64, error) {
	started := time.Now()
	var stats ImportStats
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 65536), maxRecordBytes*2)
	if !scanner.Scan() {
		return 0, errors.New("missing manifest")
	}
	rawManifest := append([]byte(nil), scanner.Bytes()...)
	var m Manifest
	if e := json.Unmarshal(rawManifest, &m); e != nil {
		return 0, e
	}
	if m.Type != "manifest" || (m.Protocol != 1 && m.Protocol != 2) || m.Dataset == "" || len(m.Dataset) > 256 || m.Edition == "" || m.Expected < 1 || m.Expected > math.MaxInt32 || len(m.SHA256) == 0 {
		return 0, errors.New("invalid manifest")
	}
	tx, e := db.Begin(ctx)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback(context.Background())
	// Ingestion/index creation is bounded by the caller's cancellable context,
	// not the serving pool's short search-statement deadline. SET LOCAL ensures
	// ordinary queries keep their deadline when this connection is returned.
	if _, e = tx.Exec(ctx, "SET LOCAL statement_timeout=0"); e != nil {
		return 0, e
	}
	var locked bool
	if e = tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(742918304)").Scan(&locked); e != nil {
		return 0, e
	}
	if !locked {
		return 0, errors.New("another import is running; retry when it finishes")
	}
	var dataset int16
	if e = tx.QueryRow(ctx, "INSERT INTO datasets(key,manifest,entries) VALUES($1,$2,0) RETURNING id", m.Dataset, rawManifest).Scan(&dataset); e != nil {
		return 0, fmt.Errorf("create dataset (already imported?): %w", e)
	}
	entriesName := fmt.Sprintf("entries_d%d", dataset)
	blocksName := fmt.Sprintf("blocks_d%d", dataset)
	entriesTable := pgx.Identifier{entriesName}
	blocksTable := pgx.Identifier{blocksName}
	if _, e = tx.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (LIKE entries INCLUDING DEFAULTS INCLUDING CONSTRAINTS INCLUDING STORAGE, CHECK(dataset_id=%d)); CREATE TABLE %s (LIKE record_blocks INCLUDING ALL, CHECK(dataset_id=%d))", entriesTable.Sanitize(), dataset, blocksTable.Sanitize(), dataset)); e != nil {
		return 0, e
	}
	identities, e := newIdentities(500000)
	if e != nil {
		return 0, e
	}
	defer identities.close()
	encoder, e := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1), zstd.WithEncoderLevel(zstd.SpeedDefault), zstd.WithEncoderCRC(true))
	if e != nil {
		return 0, e
	}
	defer encoder.Close()
	cache := newTermCache(300000)
	projection := make([]projected, 0, importBatchRows)
	blocks := make([][]any, 0, importBatchRows/blockRows)
	pending := make([]storedRecord, 0, blockRows)
	digest := sha256.New()
	var expectedSHA string
	var count int64
	var blockID int32
	ended := false
	notify := func(phase string) {
		if options.Progress != nil {
			options.Progress(Progress{phase, count, m.Expected})
		}
	}
	pack := func() error {
		if len(pending) == 0 {
			return nil
		}
		compressed, err := packBlock(encoder, pending)
		if err != nil {
			return err
		}
		for _, r := range pending {
			var header [6]byte
			binary.LittleEndian.PutUint16(header[:2], uint16(len(r.SourceID)))
			binary.LittleEndian.PutUint32(header[2:], uint32(len(r.Record)))
			digest.Write(header[:])
			digest.Write([]byte(r.SourceID))
			digest.Write(r.Record)
		}
		blocks = append(blocks, []any{dataset, blockID, compressed})
		blockID++
		stats.Blocks++
		stats.CompressedBytes += int64(len(compressed))
		clear(pending)
		pending = pending[:0]
		return nil
	}
	flush := func() error {
		if len(projection) == 0 {
			return nil
		}
		keys := make([]string, 0, len(projection)*3)
		for _, r := range projection {
			keys = append(keys, r.name, r.first, r.postcode)
		}
		resolved, err := cache.resolve(ctx, tx, keys)
		if err != nil {
			return err
		}
		values := make([][]any, 0, len(projection))
		for _, r := range projection {
			values = append(values, []any{r.id, dataset, r.kind, resolved[r.name], resolved[r.first], resolved[r.postcode]})
		}
		if _, err = tx.CopyFrom(ctx, entriesTable, []string{"id", "dataset_id", "kind", "name_id", "first_id", "postcode_id"}, pgx.CopyFromRows(values)); err != nil {
			return err
		}
		if len(blocks) > 0 {
			if _, err = tx.CopyFrom(ctx, blocksTable, []string{"dataset_id", "id", "payload"}, pgx.CopyFromRows(blocks)); err != nil {
				return err
			}
		}
		clear(projection)
		projection = projection[:0]
		clear(blocks)
		blocks = blocks[:0]
		notify("loading")
		return nil
	}
	notify("loading")
	for scanner.Scan() {
		if e = ctx.Err(); e != nil {
			return 0, e
		}
		var f frame
		if e = json.Unmarshal(scanner.Bytes(), &f); e != nil {
			return 0, e
		}
		if ended {
			return 0, errors.New("data after terminal frame")
		}
		if f.Type == "end" {
			if f.Count != count || count != m.Expected {
				return 0, fmt.Errorf("count mismatch: %d / %d / %d", count, f.Count, m.Expected)
			}
			expectedSHA = f.SHA256
			ended = true
			continue
		}
		if f.Type != "record" || len(f.SourceID) == 0 || len(f.SourceID) > maxSourceIDBytes || len(f.Record) > maxRecordBytes || f.Record == nil {
			return 0, errors.New("invalid or oversized record frame")
		}
		var row map[string]any
		decoder := json.NewDecoder(bytes.NewReader(f.Record))
		decoder.UseNumber()
		if e = decoder.Decode(&row); e != nil || row == nil {
			return 0, errors.New("record must be a JSON object")
		}
		count++
		if count > m.Expected {
			return 0, errors.New("too many entries")
		}
		if e = identities.add(f.SourceID); e != nil {
			return 0, e
		}
		r := projected{id: int32(count), name: Normalize(Text(row, "surname")), first: Normalize(Text(row, "firstName")), postcode: Postcode(Text(row, "postcode"))}
		if Text(row, "entry_type") == "business" {
			r.kind = 1
			r.name = Normalize(Text(row, "organisation"))
			r.first = ""
		}
		for _, key := range []string{r.name, r.first, r.postcode} {
			if strings.ContainsRune(key, 0) {
				return 0, errors.New("NUL in search field")
			}
		}
		projection = append(projection, r)
		pending = append(pending, storedRecord{f.SourceID, append(json.RawMessage(nil), f.Record...)})
		stats.RawBytes += int64(len(f.Record))
		if len(pending) == blockRows {
			if e = pack(); e != nil {
				return 0, e
			}
		}
		if len(projection) == cap(projection) {
			if e = flush(); e != nil {
				return 0, e
			}
		}
	}
	if e = scanner.Err(); e != nil {
		return 0, e
	}
	if !ended {
		return 0, errors.New("incomplete stream: no terminal frame; dataset not published")
	}
	if e = pack(); e != nil {
		return 0, e
	}
	if e = flush(); e != nil {
		return 0, e
	}
	stats.SHA256 = hex.EncodeToString(digest.Sum(nil))
	if (m.Protocol == 2 && expectedSHA == "") || (expectedSHA != "" && expectedSHA != stats.SHA256) {
		return 0, errors.New("source stream SHA-256 mismatch; import rejected")
	}
	stats.LoadSeconds = time.Since(started).Seconds()
	notify("checking identities")
	phase := time.Now()
	if e = identities.verify(ctx); e != nil {
		return 0, e
	}
	stats.ValidateSeconds = time.Since(phase).Seconds()
	// The exact identity check is complete; release its private scratch files
	// before index sorting needs its own temporary disk space.
	identities.close()
	notify("building indexes")
	phase = time.Now()
	for _, sql := range []string{
		fmt.Sprintf("ALTER TABLE %s ADD PRIMARY KEY(dataset_id,id)", entriesTable.Sanitize()),
		fmt.Sprintf("CREATE INDEX ON %s(dataset_id,kind,name_id,first_id,id)", entriesTable.Sanitize()),
		fmt.Sprintf("CREATE INDEX ON %s(dataset_id,kind,postcode_id,name_id,first_id,id)", entriesTable.Sanitize()),
		fmt.Sprintf("ANALYZE %s", entriesTable.Sanitize()),
	} {
		if _, e = tx.Exec(ctx, sql); e != nil {
			return 0, e
		}
	}
	stats.IndexSeconds = time.Since(phase).Seconds()
	notify("publishing")
	if _, e = tx.Exec(ctx, fmt.Sprintf("ALTER TABLE entries ATTACH PARTITION %s FOR VALUES IN(%d); ALTER TABLE record_blocks ATTACH PARTITION %s FOR VALUES IN(%d)", entriesTable.Sanitize(), dataset, blocksTable.Sanitize(), dataset)); e != nil {
		return 0, e
	}
	stats.Seconds = time.Since(started).Seconds()
	rawStats, _ := json.Marshal(stats)
	if _, e = tx.Exec(ctx, "UPDATE datasets SET entries=$1,stats=$2 WHERE id=$3", count, rawStats, dataset); e != nil {
		return 0, e
	}
	if e = tx.Commit(ctx); e != nil {
		return 0, e
	}
	// Parent partition statistics need a manual refresh; autovacuum ignores empty parents.
	if _, e = db.Exec(ctx, "ANALYZE terms; ANALYZE entries"); e != nil {
		log.Print("Import committed, but ANALYZE failed; run ANALYZE terms; ANALYZE entries")
	}
	if options.IndexAddresses {
		if _, e = IndexAddresses(ctx, db, m.Dataset, AddressIndexOptions{Progress: options.Progress, CheckResources: options.CheckResources}); e != nil {
			return count, fmt.Errorf("source edition imported, but address index is not ready: %w", e)
		}
	}
	notify("complete")
	return count, nil
}
