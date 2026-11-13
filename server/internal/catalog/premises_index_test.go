package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
)

func TestPremisesIndexIntegration(t *testing.T) {
	dsn := os.Getenv("UKINFO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	schema := fmt.Sprintf("ukinfo_premises_index_%d", os.Getpid())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, edition := range []struct {
		key, date string
		n         int
	}{{"premise-wide", "2005-06", 4096}, {"premise-old", "1999-04", 256}} {
		var input bytes.Buffer
		enc := json.NewEncoder(&input)
		_ = enc.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: edition.key, Edition: edition.date, Expected: int64(edition.n), SHA256: map[string]string{"invented": "fake"}})
		for i := 0; i < edition.n; i++ {
			house := fmt.Sprint(i + 1000)
			if i%1024 == 0 {
				house = "1"
			}
			row := map[string]any{"surname": "Invented", "firstName": "June", "houseno": house, "street": "Invented Road", "town": "Invented Town", "postcode": "AA1 1AA"}
			if i%1024 == 1 {
				row["houseno"] = "1"
				row["subbuilding"] = "Flat 2"
			}
			if i == 299 {
				row["houseno"] = "1"
				row["source_warning"] = "unresolved"
			}
			if i == 1024 {
				row["entry_type"] = "business"
				row["organisation"] = "Invented shop"
			}
			_ = enc.Encode(map[string]any{"type": "record", "source_id": fmt.Sprint(i), "record": row})
		}
		_ = enc.Encode(map[string]any{"type": "end", "count": edition.n})
		if _, err = Import(ctx, db, &input); err != nil {
			t.Fatal(err)
		}
	}
	api := NewAPI(db)
	defer api.decoder.Close()
	base := "/api/address?dataset=premise-wide&id=1&scope=all&limit=2"
	walk := func() [][]byte {
		t.Helper()
		pages := [][]byte{}
		cursor := ""
		for i := 0; i < 20; i++ {
			w := httptest.NewRecorder()
			api.Handler().ServeHTTP(w, httptest.NewRequest("GET", base+"&cursor="+url.QueryEscape(cursor), nil))
			if w.Code != 200 {
				t.Fatalf("address: %d %s", w.Code, w.Body.String())
			}
			pages = append(pages, append([]byte(nil), w.Body.Bytes()...))
			var body struct {
				Next string `json:"next_cursor"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			cursor = body.Next
			if cursor == "" {
				return pages
			}
		}
		t.Fatal("address pagination did not finish")
		return nil
	}
	baseline := walk()
	if _, err = IndexPremises(ctx, db, "premise-wide", AddressIndexOptions{}); err != nil {
		t.Fatal(err)
	}
	// Missing complete coverage must retain the complete fallback.
	partial := walk()
	if len(partial) != len(baseline) {
		t.Fatal("partial index truncated pages")
	}
	for i := range baseline {
		if !bytes.Equal(baseline[i], partial[i]) {
			t.Fatal("partial projection changed results")
		}
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	var copied int64
	if _, e := IndexPremises(cancelCtx, db, "premise-old", AddressIndexOptions{Progress: func(p Progress) { copied = p.Done; cancel() }}); e == nil || copied == 0 {
		t.Fatal("populated build did not cancel/rollback", e, copied)
	}
	if _, e := IndexPremises(ctx, db, "premise-old", AddressIndexOptions{CheckResources: func() error { return errors.New("stop test") }}); e == nil {
		t.Fatal("resource guard ignored")
	}
	var partialReady bool
	if e := db.QueryRow(ctx, "SELECT EXISTS(SELECT FROM premises_indexes WHERE dataset_id=2)").Scan(&partialReady); e != nil || partialReady {
		t.Fatal("aborted build published", e)
	}
	var child *string
	if e := db.QueryRow(ctx, "SELECT to_regclass('premises_d2')::text").Scan(&child); e != nil || child != nil {
		t.Fatal("aborted build left a partition", e)
	}
	if _, err = IndexPremises(ctx, db, "premise-old", AddressIndexOptions{}); err != nil {
		t.Fatal(err)
	}
	// Corrupt candidate keys simulate digest collisions. Raw identity validation
	// must still reject another flat, another house and an unresolved source row.
	if _, e := db.Exec(ctx, `UPDATE premises_keys p SET premises_key=a.premises_key FROM premises_keys a
 WHERE a.dataset_id=p.dataset_id AND a.id=1 AND p.id IN (2,3,300)`); e != nil {
		t.Fatal(e)
	}
	indexed := walk()
	if len(indexed) != len(baseline) {
		t.Fatal("indexed pages changed")
	}
	for i := range baseline {
		if !bytes.Equal(baseline[i], indexed[i]) {
			t.Fatal("indexed address results/cursors differ")
		}
	}
	before := api.metrics.decodedBlocks.Load()
	w := httptest.NewRecorder()
	api.Handler().ServeHTTP(w, httptest.NewRequest("GET", base, nil))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if decoded := api.metrics.decodedBlocks.Load() - before; decoded > 8 {
		t.Fatalf("same-address page decoded unrelated postcode blocks: %d", decoded)
	}
	if db.Stat().AcquiredConns() != 0 {
		t.Fatal("address index leaked connection")
	}
	if _, e := db.Exec(ctx, "UPDATE premises_keys SET premises_key=NULL WHERE dataset_id=1 AND id=1"); e != nil {
		t.Fatal(e)
	}
	w = httptest.NewRecorder()
	api.Handler().ServeHTTP(w, httptest.NewRequest("GET", base, nil))
	if w.Code != 503 {
		t.Fatal("corrupt anchor projection accepted", w.Code)
	}

	// Legacy catalogues have no optional projection at all. Household lookup and
	// edition deletion must retain their original behavior without a migration.
	if _, e := db.Exec(ctx, "DROP TABLE premises_keys CASCADE; DROP TABLE premises_indexes"); e != nil {
		t.Fatal(e)
	}
	w = httptest.NewRecorder()
	api.Handler().ServeHTTP(w, httptest.NewRequest("GET", base, nil))
	if w.Code != 200 {
		t.Fatal("legacy household lookup failed", w.Code)
	}
	if e := RemoveDataset(ctx, db, "premise-old", 2); e != nil {
		t.Fatal("legacy removal requires optional schema", e)
	}

}
