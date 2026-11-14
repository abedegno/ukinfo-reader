package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
)

func TestIndexedPremisesBatchIntegration(t *testing.T) {
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
	schema := fmt.Sprintf("premises_batch_%d", os.Getpid())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	trace := &pageBlockTracer{}
	cfg.ConnConfig.Tracer = trace
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	// Import order is deliberately the reverse of edition date order. Numeric
	// record IDs collide across editions; both people and businesses share premises.
	for year := 2005; year >= 2001; year-- {
		var input bytes.Buffer
		e := json.NewEncoder(&input)
		key := fmt.Sprintf("household-%d", year)
		e.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: key, Edition: fmt.Sprintf("%d-01", year), Expected: 3, SHA256: map[string]string{"invented": "fake"}})
		for id := 1; id <= 3; id++ {
			row := map[string]any{"surname": "Invented", "firstName": "June", "houseno": "1", "street": "Invented Road", "town": "Invented Town", "postcode": "AA1 1AA"}
			if id == 2 {
				row["entry_type"] = "business"
				row["organisation"] = "Invented shop"
			}
			if id == 3 {
				row["subbuilding"] = "Other flat"
			}
			e.Encode(map[string]any{"type": "record", "source_id": fmt.Sprint(id), "record": row})
		}
		e.Encode(map[string]any{"type": "end", "count": 3})
		if _, err = Import(ctx, db, &input); err != nil {
			t.Fatal(err)
		}
	}
	api := NewAPI(db)
	defer api.decoder.Close()
	base := "/api/address?dataset=household-2005&id=1&scope=all"
	walk := func(limit int) [][]byte {
		t.Helper()
		pages := [][]byte{}
		cursor := ""
		for n := 0; n < 20; n++ {
			w := httptest.NewRecorder()
			api.Handler().ServeHTTP(w, httptest.NewRequest("GET", base+fmt.Sprintf("&limit=%d&cursor=", limit)+url.QueryEscape(cursor), nil))
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			pages = append(pages, append([]byte(nil), w.Body.Bytes()...))
			var body struct {
				Next string `json:"next_cursor"`
			}
			if err = json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			cursor = body.Next
			if cursor == "" {
				return pages
			}
		}
		t.Fatal("household pagination did not finish")
		return nil
	}
	reference := walk(3)
	wideReference := walk(100)
	for year := 2001; year <= 2005; year++ {
		if _, err = IndexPremises(ctx, db, fmt.Sprintf("household-%d", year), AddressIndexOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	// A falsely matching key must never cause a different flat to be returned.
	if _, err = db.Exec(ctx, `UPDATE premises_keys p SET premises_key=a.premises_key FROM premises_keys a WHERE p.dataset_id=a.dataset_id AND a.id=1 AND p.id=3`); err != nil {
		t.Fatal(err)
	}
	actual := walk(3)
	if len(actual) != len(reference) {
		t.Fatal("indexed page count changed")
	}
	for i := range actual {
		if !bytes.Equal(actual[i], reference[i]) {
			t.Fatal("indexed household payload/order/cursor changed")
		}
	}
	trace.blocks.Store(0)
	trace.enabled.Store(true)
	wide := walk(100)
	trace.enabled.Store(false)
	if len(wide) != 1 || !bytes.Equal(wide[0], wideReference[0]) {
		t.Fatal("wide household changed")
	}
	var body struct {
		Items []result `json:"items"`
	}
	if err = json.Unmarshal(wide[0], &body); err != nil || len(body.Items) != 10 {
		t.Fatal("household fixture count", err)
	}
	for i, item := range body.Items {
		if item.Dataset != fmt.Sprintf("household-%d", 2001+i/2) || item.ID != int64(1+i%2) {
			t.Fatal("household source order changed")
		}
	}
	if n := trace.blocks.Load(); n != 2 {
		t.Fatalf("indexed household used %d block statements; want anchor plus one global batch", n)
	}
	// Cancellation during the shared record batch must not expose a partial page.
	cancelled, cancel := context.WithCancel(ctx)
	defer cancel()
	trace.blocks.Store(0)
	trace.cancel, trace.cancelAfter = cancel, 2
	trace.enabled.Store(true)
	w := httptest.NewRecorder()
	api.Handler().ServeHTTP(w, httptest.NewRequest("GET", base+"&limit=100", nil).WithContext(cancelled))
	trace.enabled.Store(false)
	trace.cancel = nil
	if w.Code != 503 || bytes.Contains(w.Body.Bytes(), []byte(`"items"`)) {
		t.Fatal("cancelled household exposed a partial page", w.Code, w.Body.String())
	}
	waitForPoolIdle(t, db)
	if db.Stat().AcquiredConns() != 0 {
		t.Fatal("household leaked a connection")
	}
}
