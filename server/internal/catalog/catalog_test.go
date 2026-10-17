package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestParse(t *testing.T) {
	for _, s := range []string{"dataset=a&surname=Li", "dataset=a&postcode=aa1+1aa", "dataset=a&kind=business&organisation=X"} {
		if _, err := parse(mustValues(s)); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []string{"surname=Li", "dataset=a", "dataset=a&first_name=J", "dataset=a&surname=Li&limit=101", "dataset=a&kind=business&surname=Li", "dataset=a&kind=other&postcode=A"} {
		if _, err := parse(mustValues(s)); err == nil {
			t.Fatalf("accepted %s", s)
		}
	}
	if Normalize("  O'NEILL   J ") != "o'neill j" || Postcode("aB1  2cD") != "AB12CD" {
		t.Fatal("normalization")
	}
}
func mustValues(s string) url.Values { v, _ := url.ParseQuery(s); return v }

func fixture(dataset string, terminal bool) string {
	var output bytes.Buffer
	enc := json.NewEncoder(&output)
	_ = enc.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: dataset, Edition: "invented-test", Title: "Synthetic test edition", SHA256: map[string]string{"invented": "not-a-real-source"}, Expected: 5})
	rows := []map[string]any{
		{"entry_id": "a", "surname": "Li", "firstName": "Ann", "postcode": "AA1 1AA", "source_warning": "invented_warning", "building": nil, "postcode_precision": 12, "postcode_raw": "AA1 1AB"},
		{"entry_id": "b", "surname": "LI", "firstName": "Ann", "postcode": "AA1 1AA"},
		{"entry_id": "c", "surname": "O'Neill", "firstName": "<script>", "postcode": "BB1 1BB"},
		{"entry_id": "d", "entry_type": "business", "organisation": "Invented & Sons", "postcode": "AA1 1AA", "industry_reference_raw": 123456},
		{"entry_id": "e", "surname": "Li", "firstName": "Zoe", "postcode": "AA1 1AA"},
	}
	for _, r := range rows {
		_ = enc.Encode(map[string]any{"type": "record", "source_id": r["entry_id"], "record": r})
	}
	if terminal {
		_ = enc.Encode(map[string]any{"type": "end", "count": len(rows)})
	}
	return output.String()
}

func TestPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("UKINFO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set UKINFO_TEST_DATABASE_URL to a disposable PostgreSQL 18 database")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	schema := fmt.Sprintf("ukinfo_test_%d", os.Getpid())
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
	if err = Migrate(ctx, db); err != nil {
		t.Fatalf("second migration: %v", err)
	}
	if _, err = Import(ctx, db, strings.NewReader(fixture("broken", false))); err == nil {
		t.Fatal("truncated import published")
	}
	var count int
	if err = db.QueryRow(ctx, "SELECT count(*) FROM datasets").Scan(&count); err != nil || count != 0 {
		t.Fatal("rollback failed", count, err)
	}
	if n, e := Import(ctx, db, strings.NewReader(fixture("invented", true))); e != nil || n != 5 {
		t.Fatal("import", n, e)
	}
	if _, e := Import(ctx, db, strings.NewReader(fixture("invented", true))); e == nil {
		t.Fatal("duplicate dataset accepted")
	}
	for _, input := range []string{
		strings.Replace(fixture("wrong-count", true), `"count":5`, `"count":4`, 1),
		strings.Replace(fixture("duplicate", true), `"source_id":"b"`, `"source_id":"a"`, 1),
		fixture("extra", true) + "{}\n",
	} {
		if _, e := Import(ctx, db, strings.NewReader(input)); e == nil {
			t.Fatal("invalid stream accepted")
		}
	}
	handler := NewAPI(db).Handler()
	get := func(path string, status int) map[string]json.RawMessage {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != status {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		var result map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	type response struct {
		Items []result `json:"items"`
		Next  string   `json:"next_cursor"`
	}
	decode := func(r map[string]json.RawMessage) response {
		b, _ := json.Marshal(r)
		var v response
		_ = json.Unmarshal(b, &v)
		return v
	}
	first := decode(get("/api/search?dataset=invented&surname=LI&limit=1", 200))
	if len(first.Items) != 1 || first.Items[0].SourceID != "a" || first.Next == "" {
		t.Fatalf("first page: %+v", first)
	}
	var original map[string]any
	_ = json.Unmarshal(first.Items[0].Record, &original)
	if original["source_warning"] != "invented_warning" || original["building"] != nil || original["postcode_raw"] != "AA1 1AB" {
		t.Fatal("lost original fields", original)
	}
	second := decode(get("/api/search?dataset=invented&surname=li&limit=1&cursor="+first.Next, 200))
	if len(second.Items) != 1 || second.Items[0].SourceID != "b" {
		t.Fatalf("duplicate-name pagination: %+v", second)
	}
	third := decode(get("/api/search?dataset=invented&surname=li&limit=1&cursor="+second.Next, 200))
	if len(third.Items) != 1 || third.Items[0].SourceID != "e" || third.Next != "" {
		t.Fatal("last page", third)
	}
	get("/api/search?dataset=invented&surname=other&cursor="+first.Next, 400)
	if len(decode(get("/api/search?dataset=invented&surname="+url.QueryEscape("Li' OR true --"), 200)).Items) != 0 {
		t.Fatal("SQL injection")
	}
	if len(decode(get("/api/search?dataset=invented&kind=business&organisation=Invented+%26+Sons", 200)).Items) != 1 {
		t.Fatal("business")
	}
	if len(decode(get("/api/search?dataset=invented&postcode=AA11AA", 200)).Items) != 3 {
		t.Fatal("postcode / kind")
	}
	if len(decode(get("/api/search?dataset=invented&postcode=AA11AB", 200)).Items) != 0 {
		t.Fatal("raw postcode leaked into precise search")
	}
	get("/api/search?dataset=invented&surname=Li&limit=10000", 400)
	get("/api/search?dataset=missing&surname=Li", 404)
	get("/api/search?dataset=invented", 400)
	get("/api/health", 200)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/datasets", nil))
	if !strings.Contains(w.Body.String(), `"entries":5`) || strings.Contains(w.Body.String(), "broken") {
		t.Fatal("catalogue", w.Body.String())
	}
}
