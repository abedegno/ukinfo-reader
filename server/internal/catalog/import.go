package catalog

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schema string

func Migrate(ctx context.Context, db *pgxpool.Pool) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Serialize concurrent initialization without holding a table lock while serving.
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(742918303)"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, schema); err != nil {
		return err
	}
	var version int
	if err = tx.QueryRow(ctx, "SELECT max(version) FROM catalog_schema").Scan(&version); err != nil {
		return err
	}
	if version != 1 {
		return fmt.Errorf("unsupported catalogue schema %d", version)
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
}

func Text(row map[string]any, key string) string { s, _ := row[key].(string); return s }
func Normalize(s string) string                  { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
func Postcode(s string) string                   { return strings.ToUpper(strings.Join(strings.Fields(s), "")) }

// Import publishes only after manifest, all rows, terminal count and EOF validate.
// A failed producer, duplicate source identity or canceled context rolls back everything.
func Import(ctx context.Context, db *pgxpool.Pool, input io.Reader) (int64, error) {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 65536), 4<<20)
	if !scanner.Scan() {
		return 0, errors.New("missing manifest")
	}
	var m Manifest
	if err := json.Unmarshal(scanner.Bytes(), &m); err != nil {
		return 0, err
	}
	if m.Type != "manifest" || m.Protocol != 1 || m.Dataset == "" || m.Edition == "" || m.Expected < 1 || len(m.SHA256) == 0 {
		return 0, errors.New("invalid manifest")
	}
	rawManifest := append([]byte(nil), scanner.Bytes()...)
	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(context.Background())
	var dataset int16
	err = tx.QueryRow(ctx, "INSERT INTO datasets(key,manifest,entries) VALUES($1,$2,0) RETURNING id", m.Dataset, rawManifest).Scan(&dataset)
	if err != nil {
		return 0, fmt.Errorf("create dataset (already imported?): %w", err)
	}
	batch := make([][]any, 0, 2000)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		_, e := tx.CopyFrom(ctx, pgx.Identifier{"entries"}, []string{"dataset_id", "id", "source_id", "kind", "surname", "first_name", "organisation", "postcode", "record"}, pgx.CopyFromRows(batch))
		batch = batch[:0]
		return e
	}
	var count int64
	ended := false
	for scanner.Scan() {
		var f frame
		if err = json.Unmarshal(scanner.Bytes(), &f); err != nil {
			return 0, err
		}
		if ended {
			return 0, errors.New("data after terminal frame")
		}
		if f.Type == "end" {
			if f.Count != count || count != m.Expected {
				return 0, fmt.Errorf("count mismatch: %d / %d / %d", count, f.Count, m.Expected)
			}
			ended = true
			continue
		}
		if f.Type != "record" || f.SourceID == "" || f.Record == nil {
			return 0, errors.New("invalid record frame")
		}
		var source map[string]any
		decoder := json.NewDecoder(bytes.NewReader(f.Record))
		decoder.UseNumber()
		if e := decoder.Decode(&source); e != nil || source == nil {
			return 0, errors.New("record must be a JSON object")
		}
		count++
		if count > m.Expected {
			return 0, errors.New("too many entries")
		}
		kind := "person"
		// Early SE4 fields are preserved, but their business codes are not reinterpreted.
		if Text(source, "entry_type") == "business" {
			kind = "business"
		}
		record := []byte(f.Record)
		batch = append(batch, []any{dataset, count, f.SourceID, kind, Normalize(Text(source, "surname")), Normalize(Text(source, "firstName")), Normalize(Text(source, "organisation")), Postcode(Text(source, "postcode")), record})
		if len(batch) == cap(batch) {
			if err = flush(); err != nil {
				return 0, err
			}
		}
	}
	if err = scanner.Err(); err != nil {
		return 0, err
	}
	if !ended {
		return 0, errors.New("incomplete stream: no terminal frame; dataset not published")
	}
	if err = flush(); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, "UPDATE datasets SET entries=$1 WHERE id=$2", count, dataset); err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	if _, err = db.Exec(ctx, "ANALYZE entries"); err != nil {
		log.Print("Import committed, but ANALYZE failed; run ANALYZE entries before serving")
	}
	return count, nil
}
