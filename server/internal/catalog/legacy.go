package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RepackV1 copies losslessly into a separate schema-2 database. The source is
// opened read-only and is never renamed, truncated or upgraded in place.
func RepackV1(ctx context.Context, destination, source *pgxpool.Pool, options ImportOptions) error {
	var version int
	if e := source.QueryRow(ctx, "SELECT max(version) FROM catalog_schema").Scan(&version); e != nil {
		return e
	}
	if version != 1 {
		return fmt.Errorf("repack-v1 requires a schema-1 source")
	}
	rows, e := source.Query(ctx, "SELECT key,manifest,entries FROM datasets ORDER BY id")
	if e != nil {
		return e
	}
	type dataset struct {
		key      string
		manifest json.RawMessage
		count    int64
	}
	var datasets []dataset
	for rows.Next() {
		var d dataset
		if e = rows.Scan(&d.key, &d.manifest, &d.count); e != nil {
			rows.Close()
			return e
		}
		datasets = append(datasets, d)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, d := range datasets {
		reader, writer := io.Pipe()
		done := make(chan error, 1)
		go func(d dataset) {
			err := func() error {
				encoder := json.NewEncoder(writer)
				if err := encoder.Encode(d.manifest); err != nil {
					return err
				}
				rows, err := source.Query(ctx, "SELECT source_id,record FROM entries WHERE dataset_id=(SELECT id FROM datasets WHERE key=$1) ORDER BY id", d.key)
				if err != nil {
					return err
				}
				defer rows.Close()
				var count int64
				for rows.Next() {
					var f frame
					f.Type = "record"
					if err = rows.Scan(&f.SourceID, &f.Record); err != nil {
						return err
					}
					if err = encoder.Encode(f); err != nil {
						return err
					}
					count++
				}
				if err = rows.Err(); err != nil {
					return err
				}
				if count != d.count {
					return fmt.Errorf("legacy source count mismatch")
				}
				return encoder.Encode(frame{Type: "end", Count: count})
			}()
			_ = writer.CloseWithError(err)
			done <- err
		}(d)
		_, e = ImportWithOptions(ctx, destination, reader, options)
		_ = reader.CloseWithError(e)
		producer := <-done
		if e != nil {
			return e
		}
		if producer != nil {
			return producer
		}
	}
	return nil
}
