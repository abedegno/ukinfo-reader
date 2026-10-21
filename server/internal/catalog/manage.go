package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ManagedDataset struct {
	ID            int16     `json:"id"`
	Key           string    `json:"key"`
	Manifest      Manifest  `json:"manifest"`
	Entries       int64     `json:"entries"`
	ImportedAt    time.Time `json:"imported_at"`
	Bytes         int64     `json:"bytes"`
	ResearchReady bool      `json:"research_ready"`
	AddressReady  bool      `json:"address_ready"`
}
type CatalogueStorage struct {
	Datasets       []ManagedDataset  `json:"datasets"`
	DatabaseBytes  int64             `json:"database_bytes"`
	SharedBytes    int64             `json:"shared_bytes"`
	GeographyBytes int64             `json:"geography_bytes"`
	Geography      *GeographyRelease `json:"geography"`
	State          ImportState       `json:"state"`
}

func CatalogueInventory(ctx context.Context, db *pgxpool.Pool) (report CatalogueStorage, err error) {
	report.Datasets = []ManagedDataset{}
	rows, err := db.Query(ctx, `SELECT d.id,d.key,d.manifest,d.entries,d.imported_at,
 COALESCE((SELECT sum(pg_total_relation_size(c.oid)) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
 WHERE n.nspname=current_schema() AND c.relname IN ('entries_d'||d.id,'blocks_d'||d.id,'research_d'||d.id)),0)::bigint,
 EXISTS(SELECT 1 FROM address_indexes i WHERE i.dataset_id=d.id AND i.entries=d.entries AND i.version=1),
 EXISTS(SELECT 1 FROM research_indexes i WHERE i.dataset_id=d.id AND i.entries=d.entries AND i.version=1)
 FROM datasets d ORDER BY d.imported_at DESC,d.id DESC`)
	if err != nil {
		return report, err
	}
	for rows.Next() {
		var d ManagedDataset
		var raw []byte
		if err = rows.Scan(&d.ID, &d.Key, &raw, &d.Entries, &d.ImportedAt, &d.Bytes, &d.AddressReady, &d.ResearchReady); err != nil {
			rows.Close()
			return report, err
		}
		if err = json.Unmarshal(raw, &d.Manifest); err != nil {
			rows.Close()
			return report, err
		}
		report.Datasets = append(report.Datasets, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return report, err
	}
	err = db.QueryRow(ctx, `SELECT pg_database_size(current_database()),
 pg_total_relation_size('terms')+pg_total_relation_size('address_locations')+pg_total_relation_size('address_members')+pg_total_relation_size('address_indexes')+pg_total_relation_size('research_indexes'),
 (SELECT COALESCE(sum(pg_total_relation_size(relid)),0)::bigint FROM pg_partition_tree('postcode_locations'))`).Scan(&report.DatabaseBytes, &report.SharedBytes, &report.GeographyBytes)
	if err != nil {
		return report, err
	}
	geo, err := activeGeography(ctx, db)
	if errors.Is(err, pgx.ErrNoRows) {
		return report, nil
	}
	if err != nil {
		return report, err
	}
	report.Geography = &geo
	return report, nil
}

// RemoveDataset drops only the two verified partitions owned by this immutable
// dataset identity. Shared dictionaries and native research snapshots are retained.
func RemoveDataset(ctx context.Context, db *pgxpool.Pool, key string, expectedID int16) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SET LOCAL statement_timeout=0; SET LOCAL lock_timeout='3s'"); err != nil {
		return err
	}
	for _, sql := range []string{"SELECT pg_try_advisory_xact_lock(742918304)", "SELECT pg_try_advisory_xact_lock(742918306,hashtext(current_schema()))"} {
		var locked bool
		if err = tx.QueryRow(ctx, sql).Scan(&locked); err != nil {
			return err
		}
		if !locked {
			return errors.New("an import or address index build is running; retry after it finishes")
		}
	}
	var id int16
	if err = tx.QueryRow(ctx, "SELECT id FROM datasets WHERE key=$1 FOR UPDATE", key).Scan(&id); err != nil {
		return err
	}
	if id != expectedID || id < 1 {
		return errors.New("edition changed since review; refresh the catalogue")
	}
	for _, table := range []string{"address_record_blocks", "address_record_indexes"} {
		if _, err = tx.Exec(ctx, "DELETE FROM "+table+" WHERE dataset_id=$1", id); err != nil {
			return err
		}
	}
	for _, part := range []struct{ prefix, parent string }{{"entries_d", "entries"}, {"blocks_d", "record_blocks"}} {
		name := fmt.Sprintf("%s%d", part.prefix, id)
		var attached bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_inherits WHERE inhrelid=to_regclass($1) AND inhparent=to_regclass($2))`, pgx.Identifier{name}.Sanitize(), part.parent).Scan(&attached); err != nil {
			return err
		}
		if !attached {
			return errors.New("edition partition ownership could not be verified")
		}
		if _, err = tx.Exec(ctx, "DROP TABLE "+pgx.Identifier{name}.Sanitize()); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, "DELETE FROM address_members WHERE dataset_id=$1", id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "DELETE FROM address_indexes WHERE dataset_id=$1", id); err != nil {
		return err
	}
	var researchReady bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM research_indexes WHERE dataset_id=$1)", id).Scan(&researchReady); err != nil {
		return err
	}
	if researchReady {
		name := fmt.Sprintf("research_d%d", id)
		var attached bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_inherits WHERE inhrelid=to_regclass($1) AND inhparent=to_regclass('research_keys'))", pgx.Identifier{name}.Sanitize()).Scan(&attached); err != nil {
			return err
		}
		if !attached {
			return errors.New("research index ownership could not be verified")
		}
		if _, err = tx.Exec(ctx, "DROP TABLE "+pgx.Identifier{name}.Sanitize()); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "DELETE FROM research_indexes WHERE dataset_id=$1", id); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, "DELETE FROM datasets WHERE id=$1", id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (a *Admin) inventory(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	report, err := CatalogueInventory(ctx, a.DB)
	if err != nil {
		fail(w, 503, "Unable to read catalogue storage")
		return
	}
	a.mu.Lock()
	report.State = a.state
	a.mu.Unlock()
	respond(w, 200, report)
}
func (a *Admin) manageDataset(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Key string `json:"key"`
		ID  int16  `json:"id"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	decoder.DisallowUnknownFields()
	if r.Header.Get("Content-Type") != "application/json" || decoder.Decode(&input) != nil || decoder.Decode(&struct{}{}) != io.EOF || input.ID < 1 || input.Key == "" || len(input.Key) > 256 {
		fail(w, 400, "Choose an edition from the catalogue")
		return
	}
	timeout := 30 * time.Minute
	if r.URL.Path == "/index-research" {
		timeout = 4 * time.Hour
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	a.mu.Lock()
	if a.state.Running {
		a.mu.Unlock()
		fail(w, 409, "An import or catalogue operation is already running")
		return
	}
	phase := "removing edition"
	if r.URL.Path == "/index-research" {
		phase = "indexing research"
	}
	if r.URL.Path == "/index-dataset" {
		phase = "indexing addresses"
	}
	a.state = ImportState{Running: true, Progress: Progress{Phase: phase}}
	a.cancel = cancel
	a.mu.Unlock()
	var err error
	if r.URL.Path == "/index-dataset" || r.URL.Path == "/index-research" {
		var id int16
		err = a.DB.QueryRow(ctx, "SELECT id FROM datasets WHERE key=$1", input.Key).Scan(&id)
		if err == nil && id != input.ID {
			err = errors.New("edition changed since review; refresh the catalogue")
		}
		if err == nil {
			options := AddressIndexOptions{CheckResources: a.CheckResources, Progress: func(p Progress) { a.mu.Lock(); a.state.Progress = p; a.mu.Unlock() }}
			if r.URL.Path == "/index-research" {
				_, err = IndexResearch(ctx, a.DB, input.Key, options)
			} else {
				_, err = IndexAddresses(ctx, a.DB, input.Key, options)
			}
		}
	} else {
		err = RemoveDataset(ctx, a.DB, input.Key, input.ID)
	}
	a.mu.Lock()
	a.state.Running = false
	a.cancel = nil
	if err != nil {
		a.state.Error = err.Error()
		a.state.Progress.Phase = "failed"
	} else {
		a.state.Progress.Phase = "complete"
	}
	a.mu.Unlock()
	if err != nil {
		fail(w, 422, "Catalogue operation stopped: "+err.Error())
		return
	}
	respond(w, 200, map[string]bool{"complete": true})
}
