package catalog

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/klauspost/compress/zstd"
)

//go:embed premises_index.sql
var premisesSchema string

type PremisesIndexReport struct {
	Dataset   string  `json:"dataset"`
	Records   int64   `json:"records"`
	Matchable int64   `json:"matchable"`
	Bytes     int64   `json:"bytes"`
	Seconds   float64 `json:"seconds"`
}

// IndexPremises publishes only a complete edition. Invalid/ambiguous addresses
// retain a NULL key, and every source ID is still counted. No source is rewritten.
func IndexPremises(ctx context.Context, db *pgxpool.Pool, key string, options AddressIndexOptions) (report PremisesIndexReport, err error) {
	if options.SampleBlocks != 0 {
		return report, errors.New("premises index builds must cover the complete imported scope")
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
	if _, err = tx.Exec(ctx, "SET LOCAL statement_timeout=0; SET LOCAL work_mem='32MB'; SET LOCAL maintenance_work_mem='64MB'; SET LOCAL max_parallel_maintenance_workers=0"); err != nil {
		return report, err
	}
	for _, sql := range []string{"SELECT pg_try_advisory_xact_lock(742918304)", "SELECT pg_try_advisory_xact_lock(742918309,hashtext(current_schema()))"} {
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
	if err = tx.QueryRow(ctx, "SELECT id,entries FROM datasets WHERE key=$1 FOR UPDATE", key).Scan(&dataset, &expected); err != nil {
		return report, err
	}
	var ready bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM premises_indexes WHERE dataset_id=$1)", dataset).Scan(&ready); err != nil {
		return report, err
	}
	if ready {
		return report, errors.New("edition already has a premises index")
	}
	name := fmt.Sprintf("premises_d%d", dataset)
	if _, err = tx.Exec(ctx, "CREATE TABLE "+pgx.Identifier{name}.Sanitize()+" (LIKE premises_keys INCLUDING DEFAULTS INCLUDING CONSTRAINTS, CHECK(dataset_id="+fmt.Sprint(dataset)+"))"); err != nil {
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
			return report, errors.New("missing source block during premises indexing")
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
				identity, reason, e := premisesRecord(record.Record)
				if e != nil {
					return report, e
				}
				var key []byte
				if reason == "" {
					sum := sha256.Sum256([]byte(identity.key()))
					key = sum[:]
					report.Matchable++
				}
				values = append(values, []any{dataset, b.id*blockRows + int32(offset) + 1, key})
				report.Records++
			}
		}
		if _, err = tx.CopyFrom(ctx, pgx.Identifier{name}, []string{"dataset_id", "id", "premises_key"}, pgx.CopyFromRows(values)); err != nil {
			return report, err
		}
		if options.Progress != nil {
			options.Progress(Progress{"indexing premises", report.Records, expected})
		}
	}
	if report.Records != expected {
		return report, errors.New("premises index count differs from source")
	}
	if err = check(); err != nil {
		return report, err
	}

	if options.Progress != nil {
		options.Progress(Progress{"building premises indexes", report.Records, expected})
	}
	if _, err = tx.Exec(ctx, "ALTER TABLE premises_keys ATTACH PARTITION "+pgx.Identifier{name}.Sanitize()+" FOR VALUES IN ("+fmt.Sprint(dataset)+")"); err != nil {
		return report, err
	}
	if _, err = tx.Exec(ctx, "ANALYZE "+pgx.Identifier{name}.Sanitize()); err != nil {
		return report, err
	}
	if err = tx.QueryRow(ctx, "SELECT pg_total_relation_size($1::regclass)", pgx.Identifier{name}.Sanitize()).Scan(&report.Bytes); err != nil {
		return report, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO premises_indexes(dataset_id,version,entries,matchable) VALUES($1,1,$2,$3)", dataset, expected, report.Matchable); err != nil {
		return report, err
	}
	if err = tx.Commit(ctx); err != nil {
		return report, err
	}
	report.Seconds = time.Since(started).Seconds()
	return report, nil
}
