package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestParse(t *testing.T) {
	for _, s := range []string{"dataset=a&surname=Li", "dataset=a&postcode=aa1+1aa", "dataset=a&kind=business&organisation=X", "dataset=all&surname=Sm*&first_name=J&match=prefix", "dataset=a&postcode=WS13", "dataset=a&postcode=WS13+7"} {
		if _, err := parse(mustValues(s)); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []string{"surname=Li", "dataset=a", "dataset=a&first_name=J", "dataset=a&surname=Li&limit=101", "dataset=a&kind=business&surname=Li", "dataset=a&kind=other&postcode=A", "dataset=a&surname=*", "dataset=a&surname=S*", "dataset=a&surname=Sm?th", "dataset=a&surname=Sm%25", "dataset=a&surname=Smith&match=contains", "dataset=a&postcode=WS*13"} {
		if _, err := parse(mustValues(s)); err == nil {
			t.Fatalf("accepted %s", s)
		}
	}
	if Normalize("  O'NEILL   J ") != "o'neill j" || Postcode("aB1  2cD") != "AB12CD" {
		t.Fatal("normalization")
	}
}
func TestPostcodeBoundaries(t *testing.T) {
	for _, tc := range []struct {
		input  string
		length int
	}{{"W11", 6}, {"W1 1", 5}, {"W1 1*", 5}, {"WS13", 7}, {"WS13*", 7}, {"WS13 7", 7}, {"WS137", 7}, {"WS13 7SW", 0}, {"W1 1AA", 0}, {"W11AA", 0}, {"GIR", 6}, {"GIR 0", 6}, {"GIR0", 6}} {
		q, err := parse(url.Values{"dataset": {"all"}, "postcode": {tc.input}})
		if err != nil || q.PostcodeLength != tc.length {
			t.Fatalf("%s: %+v %v", tc.input, q, err)
		}
	}
	a, _ := parse(mustValues("dataset=all&postcode=W11"))
	b, _ := parse(mustValues("dataset=all&postcode=W1+1"))
	if a.fingerprint() == b.fingerprint() {
		t.Fatal("district and sector cursors overlap")
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
		strings.Replace(fixture("missing-sha", true), `"protocol":1`, `"protocol":2`, 1),
		strings.Replace(strings.Replace(fixture("bad-sha", true), `"protocol":1`, `"protocol":2`, 1), `"count":5`, `"count":5,"sha256":"`+strings.Repeat("0", 64)+`"`, 1),
	} {
		if _, e := Import(ctx, db, strings.NewReader(input)); e == nil {
			t.Fatal("invalid stream accepted")
		}
	}
	if err = db.QueryRow(ctx, "SELECT count(*) FROM datasets").Scan(&count); err != nil || count != 1 {
		t.Fatal("failed import left a visible dataset", count, err)
	}
	verified, err := Verify(ctx, db, "invented", nil)
	if err != nil || verified.Entries != 5 || !verified.ProjectionChecked {
		t.Fatal("full verification", verified, err)
	}
	// A correct row count alone must not conceal a damaged search projection.
	if _, err = db.Exec(ctx, "UPDATE entries SET name_id=0 WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if _, err = Verify(ctx, db, "invented", nil); err == nil {
		t.Fatal("projection corruption went undetected")
	}
	if _, err = db.Exec(ctx, "UPDATE entries SET name_id=(SELECT id FROM terms WHERE key='li') WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	testDesktopAdmin(t, db)
	testMultipleBlocksAndBatches(t, db)
	testLegacyRepack(t, db)
	testTermReuseDoesNotAllocateIdentity(t, db)
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
	testExpandedSearch(t, db)
	testGeography(t, db)
	testAddresses(t, db)
	testPremises(t, db)
	testRefinements(t, db)
	testCatalogueManagement(t, db)
}

func testDesktopAdmin(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	admin := &Admin{DB: db, Token: strings.Repeat("a", 64)}
	server := httptest.NewUnstartedServer(admin.Handler())
	admin.Host = server.Listener.Addr().String()
	server.Start()
	defer server.Close()
	defer admin.Cancel()
	request := func(method, path string, body io.Reader, token, origin, host string) *http.Response {
		t.Helper()
		r, e := http.NewRequest(method, server.URL+path, body)
		if e != nil {
			t.Fatal(e)
		}
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if host != "" {
			r.Host = host
		}
		if method == "POST" && path == "/import" {
			r.Header.Set("Content-Type", "application/x-ndjson")
		}
		response, e := server.Client().Do(r)
		if e != nil {
			t.Fatal(e)
		}
		return response
	}
	for _, c := range []struct{ token, origin, host string }{{"", "", ""}, {"wrong", "", ""}, {admin.Token, "https://other.invalid", ""}, {admin.Token, "", "other.invalid"}} {
		response := request("GET", "/progress", nil, c.token, c.origin, c.host)
		_ = response.Body.Close()
		if response.StatusCode != 403 {
			t.Fatal("desktop capability bypass", response.StatusCode)
		}
	}
	reader, writer := io.Pipe()
	finished := make(chan error, 1)
	req, _ := http.NewRequest("POST", server.URL+"/import", reader)
	req.Header.Set("Authorization", "Bearer "+admin.Token)
	req.Header.Set("Content-Type", "application/x-ndjson")
	go func() {
		response, e := server.Client().Do(req)
		if e == nil {
			_ = response.Body.Close()
			if response.StatusCode != 422 {
				e = fmt.Errorf("canceled import status %d", response.StatusCode)
			}
		}
		finished <- e
	}()
	go func() { _, _ = io.WriteString(writer, fixture("canceled-admin", false)) }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		admin.mu.Lock()
		running := admin.state.Running
		admin.mu.Unlock()
		if running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("admin import did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	response := request("POST", "/cancel", nil, admin.Token, "", "")
	_ = response.Body.Close()
	if response.StatusCode != 202 {
		t.Fatal("cancel status", response.StatusCode)
	}
	_ = writer.Close()
	select {
	case e := <-finished:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not interrupt a stalled producer")
	}
	var count int
	if e := db.QueryRow(context.Background(), "SELECT count(*) FROM datasets WHERE key='canceled-admin'").Scan(&count); e != nil || count != 0 {
		t.Fatal("canceled import published", count, e)
	}
}

func testMultipleBlocksAndBatches(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	var input bytes.Buffer
	encoder := json.NewEncoder(&input)
	count := importBatchRows + 1
	_ = encoder.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: "multiple-batches", Edition: "invented", Title: "Invented boundary fixture", SHA256: map[string]string{"fixture": "invented"}, Expected: int64(count)})
	for i := 1; i <= count; i++ {
		sid := fmt.Sprintf("invented:%d", i)
		_ = encoder.Encode(frame{Type: "record", SourceID: sid, Record: json.RawMessage(`{"surname":"Li","firstName":"Same","postcode":"ZZ1 1ZZ","large_integer":9007199254740993}`)})
	}
	_ = encoder.Encode(frame{Type: "end", Count: int64(count)})
	if n, e := Import(ctx, db, &input); e != nil || n != int64(count) {
		t.Fatal("multi-batch import", n, e)
	}
	var dataset int16
	if e := db.QueryRow(ctx, "SELECT id FROM datasets WHERE key='multiple-batches'").Scan(&dataset); e != nil {
		t.Fatal(e)
	}
	api := NewAPI(db)
	ids := []int32{1, blockRows, blockRows + 1, importBatchRows, importBatchRows + 1}
	results, e := api.loadRecords(ctx, dataset, ids)
	if e != nil {
		t.Fatal(e)
	}
	for i, r := range results {
		if r.SourceID != fmt.Sprintf("invented:%d", ids[i]) || !strings.Contains(string(r.Record), "9007199254740993") {
			t.Fatal("block/batch boundary or numeric precision", r.SourceID)
		}
	}
	w := httptest.NewRecorder()
	api.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/export?dataset=multiple-batches&surname=Li&limit=1", nil))
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/x-ndjson" || !strings.Contains(w.Body.String(), "9007199254740993") || strings.Count(w.Body.String(), "\n") != 1 {
		t.Fatal("bounded export lost source precision", w.Code, w.Body.String())
	}
	if v, e := Verify(ctx, db, "multiple-batches", nil); e != nil || v.Entries != int64(count) {
		t.Fatal("multi-batch verification", v, e)
	}
}

// An old catalogue is copied through a read-only pool into new partitions. The
// legacy manifest, source IDs, nulls and warning values must survive the move.
func testLegacyRepack(t *testing.T, destination *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	schema := fmt.Sprintf("ukinfo_legacy_test_%d", os.Getpid())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, e := destination.Exec(ctx, "CREATE SCHEMA "+quoted); e != nil {
		t.Fatal(e)
	}
	defer destination.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE")
	cfg := destination.Config().Copy()
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	source, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer source.Close()
	_, e = source.Exec(ctx, `CREATE TABLE catalog_schema(version integer); INSERT INTO catalog_schema VALUES(1);
 CREATE TABLE datasets(id integer,key text,manifest jsonb,entries bigint);
 CREATE TABLE entries(id integer,dataset_id integer,source_id text,record jsonb);`)
	if e != nil {
		t.Fatal(e)
	}
	lines := strings.Split(strings.TrimSpace(fixture("legacy-copy", true)), "\n")
	if _, e = source.Exec(ctx, "INSERT INTO datasets VALUES(1,'legacy-copy',$1,5)", lines[0]); e != nil {
		t.Fatal(e)
	}
	for i, line := range lines[1 : len(lines)-1] {
		var f frame
		if e = json.Unmarshal([]byte(line), &f); e != nil {
			t.Fatal(e)
		}
		if _, e = source.Exec(ctx, "INSERT INTO entries VALUES($1,1,$2,$3)", i+1, f.SourceID, f.Record); e != nil {
			t.Fatal(e)
		}
	}
	cfg = source.Config().Copy()
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	readonly, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer readonly.Close()
	if e = Migrate(ctx, source); e == nil {
		t.Fatal("schema-1 migration should require a separate destination")
	}
	if e = RepackV1(ctx, destination, readonly, ImportOptions{}); e != nil {
		t.Fatal(e)
	}
	if report, e := Verify(ctx, destination, "legacy-copy", nil); e != nil || report.Entries != 5 {
		t.Fatal(report, e)
	}
	w := httptest.NewRecorder()
	NewAPI(destination).Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?dataset=legacy-copy&surname=Li&first_name=Ann", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"source_warning":"invented_warning"`) || !strings.Contains(w.Body.String(), `"building":null`) {
		t.Fatal("legacy fields lost", w.Code, w.Body.String())
	}
	var count int
	if e = source.QueryRow(ctx, "SELECT count(*) FROM entries").Scan(&count); e != nil || count != 5 {
		t.Fatal("legacy source changed", count, e)
	}
}

func TestDesktopDiskPreflight(t *testing.T) {
	admin := &Admin{Token: strings.Repeat("d", 64), Host: "127.0.0.1", CheckResources: func() error { return ErrDiskHeadroom }}
	req := httptest.NewRequest("POST", "http://127.0.0.1/import", strings.NewReader("not read"))
	req.Header.Set("Authorization", "Bearer "+admin.Token)
	req.Header.Set("Content-Type", "application/x-ndjson")
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)
	if w.Code != 507 || admin.state.Running {
		t.Fatal("disk guard admitted import", w.Code)
	}
}

// A disk guard must interrupt a producer stalled before its first complete frame;
// canceling SQL alone cannot unblock an HTTP request-body read.
func TestDesktopDiskGuardInterruptsProducer(t *testing.T) {
	var checks atomic.Int32
	admin := &Admin{Token: strings.Repeat("e", 64), CheckResources: func() error {
		if checks.Add(1) > 1 {
			return ErrDiskHeadroom
		}
		return nil
	}}
	server := httptest.NewUnstartedServer(admin.Handler())
	admin.Host = server.Listener.Addr().String()
	server.Start()
	defer server.Close()
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	req, _ := http.NewRequest("POST", server.URL+"/import", reader)
	req.Header.Set("Authorization", "Bearer "+admin.Token)
	req.Header.Set("Content-Type", "application/x-ndjson")
	go func() { _, _ = io.WriteString(writer, " ") }()
	client := server.Client()
	client.Timeout = 5 * time.Second
	response, e := client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer response.Body.Close()
	if response.StatusCode != 507 {
		t.Fatal("disk guard did not interrupt upload", response.StatusCode)
	}
	admin.mu.Lock()
	running := admin.state.Running
	admin.mu.Unlock()
	if running {
		t.Fatal("disk guard left import active")
	}
}

func testTermReuseDoesNotAllocateIdentity(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	tx, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	cache := newTermCache(1)
	first, e := cache.resolve(ctx, tx, []string{"invented-vocabulary-a"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = cache.resolve(ctx, tx, []string{"invented-vocabulary-b"}); e != nil {
		t.Fatal(e)
	}
	var before, after int64
	if e = tx.QueryRow(ctx, "SELECT last_value FROM terms_id_seq").Scan(&before); e != nil {
		t.Fatal(e)
	}
	again, e := cache.resolve(ctx, tx, []string{"invented-vocabulary-a"})
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.QueryRow(ctx, "SELECT last_value FROM terms_id_seq").Scan(&after); e != nil {
		t.Fatal(e)
	}
	if before != after || again["invented-vocabulary-a"] != first["invented-vocabulary-a"] {
		t.Fatal("cache eviction wasted or changed a vocabulary identity", before, after)
	}
}

func testExpandedSearch(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	for _, dataset := range []string{"prefix-one", "prefix-two", "prefix-sample"} {
		var input bytes.Buffer
		enc := json.NewEncoder(&input)
		_ = enc.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: dataset, Edition: "prefix-fixture", Title: dataset, SHA256: map[string]string{"invented": "not-a-real-source"}, Sample: dataset == "prefix-sample", Expected: 5})
		for i, postcode := range []string{"W11 1AA", "W1 1AA", "WS13 7SW", "WS13", "WS1 3AA"} {
			_ = enc.Encode(map[string]any{"type": "record", "source_id": fmt.Sprint(i), "record": map[string]any{"surname": "Quincetest", "firstName": "June", "postcode": postcode}})
		}
		_ = enc.Encode(map[string]any{"type": "end", "count": 5})
		if _, err := Import(context.Background(), db, &input); err != nil {
			t.Fatal(err)
		}
	}
	handler := NewAPI(db).Handler()
	fetch := func(params string) ([]result, string) {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/search?"+params, nil))
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", params, w.Code, w.Body.String())
		}
		var v struct {
			Items []result `json:"items"`
			Next  string   `json:"next_cursor"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		return v.Items, v.Next
	}
	for _, tc := range []struct {
		query string
		count int
	}{
		{"dataset=prefix-one&postcode=W11", 1}, {"dataset=prefix-one&postcode=W1+1", 1},
		{"dataset=prefix-one&postcode=WS13", 2}, {"dataset=prefix-one&postcode=WS13*", 2}, {"dataset=prefix-one&postcode=WS13+7", 1},
		{"dataset=prefix-one&postcode=WS13+7SW", 1}, {"dataset=prefix-one&postcode=WS1", 1},
		{"dataset=prefix-one&surname=Quince*", 5}, {"dataset=prefix-one&surname=Quince&match=prefix&first_name=J", 5},
		{"dataset=all&surname=Quincetest", 10},
	} {
		rows, _ := fetch(tc.query)
		if len(rows) != tc.count {
			t.Fatalf("%s: got %d want %d", tc.query, len(rows), tc.count)
		}
	}
	// More than 8,192 shared-vocabulary terms need not correspond to listings
	// of the requested type. Matching entries must not be lost behind that noise.
	if _, err := db.Exec(context.Background(), `INSERT INTO terms(key) SELECT prefix || lpad(i::text,5,'0') FROM unnest(ARRAY['paaa','jaaa']) prefix CROSS JOIN generate_series(1,8300) i ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	var broadInput bytes.Buffer
	broadEncoder := json.NewEncoder(&broadInput)
	_ = broadEncoder.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: "broad-prefix", Edition: "broad-fixture", Title: "Broad invented fixture", SHA256: map[string]string{"invented": "not-a-real-source"}, Expected: 130})
	for i := 0; i < 130; i++ {
		_ = broadEncoder.Encode(map[string]any{"type": "record", "source_id": fmt.Sprint(i), "record": map[string]any{"surname": fmt.Sprintf("Pagingprefix%03d", i), "firstName": "June", "postcode": "ZZ1 1ZZ"}})
	}
	_ = broadEncoder.Encode(map[string]any{"type": "end", "count": 130})
	if _, err := Import(context.Background(), db, &broadInput); err != nil {
		t.Fatal(err)
	}
	broadSeen := map[string]bool{}
	broadCursor := ""
	for pages := 0; ; pages++ {
		if pages > 10 {
			t.Fatal("broad-prefix pagination did not terminate")
		}
		rows, next := fetch("dataset=all&surname=Pa*&limit=17&cursor=" + broadCursor)
		for _, row := range rows {
			if row.Dataset != "broad-prefix" || broadSeen[row.SourceID] {
				t.Fatal("unexpected or duplicate broad-prefix occurrence", row.Dataset, row.SourceID)
			}
			broadSeen[row.SourceID] = true
		}
		if next == "" {
			break
		}
		broadCursor = next
	}
	if len(broadSeen) != 130 {
		t.Fatal("broad-prefix pagination lost records", len(broadSeen))
	}
	if rows, _ := fetch("dataset=all&surname=Paaa*"); len(rows) != 0 {
		t.Fatal("unused vocabulary terms matched entries")
	}
	if rows, _ := fetch("dataset=prefix-one&surname=Quincetest&first_name=J*"); len(rows) != 5 {
		t.Fatal("broad first-name prefix lost records", len(rows))
	}
	firstEditions, _ := fetch("dataset=all&surname=Quincetest&limit=2")
	if len(firstEditions) != 2 || firstEditions[0].Dataset == firstEditions[1].Dataset {
		t.Fatal("first page did not interleave editions", firstEditions)
	}
	seen := map[string]bool{}
	cursor := ""
	for {
		rows, next := fetch("dataset=all&surname=Quincetest&limit=3&cursor=" + cursor)
		for _, row := range rows {
			key := row.Dataset + ":" + row.SourceID
			if seen[key] || row.Dataset == "prefix-sample" {
				t.Fatal("duplicate or sample", key)
			}
			seen[key] = true
		}
		if next == "" {
			break
		}
		cursor = next
	}
	seenPostcodes := map[string]bool{}
	nextPostcode := ""
	for {
		rows, next := fetch("dataset=all&postcode=WS13&surname=Quincetest&limit=1&cursor=" + nextPostcode)
		for _, row := range rows {
			key := row.Dataset + ":" + row.SourceID
			if seenPostcodes[key] {
				t.Fatal("duplicate postcode page", key)
			}
			seenPostcodes[key] = true
		}
		if next == "" {
			break
		}
		nextPostcode = next
	}
	if len(seenPostcodes) != 4 {
		t.Fatal("postcode prefix pagination", len(seenPostcodes))
	}
	if len(seen) != 10 {
		t.Fatal("cross-edition pagination lost records", len(seen))
	}
}
