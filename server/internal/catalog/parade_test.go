package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestParadeIntegration(t *testing.T) {
	sourceDSN, targetDSN := os.Getenv("UKINFO_TEST_DATABASE_URL"), os.Getenv("UKINFO_PARADE_TEST_DATABASE_URL")
	if sourceDSN == "" || targetDSN == "" {
		t.Skip("requires disposable source and separate pg_search 0.26.3 target databases")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, sourceDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	schema := fmt.Sprintf("parade_source_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, err := pgxpool.ParseConfig(sourceDSN)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	source, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	target, err := pgxpool.New(ctx, targetDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	var occupied bool
	if err = target.QueryRow(ctx, "SELECT to_regnamespace('ukinfo_search') IS NOT NULL").Scan(&occupied); err != nil || occupied {
		t.Fatal("target must be disposable with no existing ukinfo_search schema", err)
	}
	defer target.Exec(ctx, "DROP SCHEMA IF EXISTS ukinfo_search CASCADE")
	if _, err = BuildParadeIndex(ctx, source, source, "all", ParadeBuildOptions{}); err == nil {
		t.Fatal("same database accepted")
	}
	if err = Migrate(ctx, source); err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	for i, year := range []string{"1997", "1999", "2003", "2004", "2005"} {
		key := "parade-" + year
		keys = append(keys, key)
		data := researchFixture(key, "invented-"+year+"-01", i%2 == 0)
		if _, err = Import(ctx, source, strings.NewReader(data)); err != nil {
			t.Fatal(err)
		}
	}
	sample := strings.Replace(researchFixture("parade-sample", "invented-2005-01", false), `"sample":false`, `"sample":true`, 1)
	if _, err = Import(ctx, source, strings.NewReader(sample)); err != nil {
		t.Fatal(err)
	}
	for _, key := range append(keys, "parade-sample") {
		if _, err = IndexAddresses(ctx, source, key, AddressIndexOptions{}); err != nil {
			t.Fatal(err)
		}
		if _, err = IndexResearch(ctx, source, key, AddressIndexOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	// More than two internal batches with widely separated matches exercise
	// complete refinement paging, rather than a shallow positive-only fixture.
	var sparse bytes.Buffer
	enc := json.NewEncoder(&sparse)
	enc.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: "parade-sparse", Edition: "invented-sparse", Title: "Invented sparse test", SHA256: map[string]string{"invented": strings.Repeat("b", 64)}, Expected: 260})
	for i := 0; i < 260; i++ {
		house := "1"
		if i == 98 || i == 198 || i >= 257 {
			house = "999"
		}
		enc.Encode(map[string]any{"type": "record", "source_id": fmt.Sprint(i), "record": map[string]any{"surname": "Sparseonly", "firstName": "June", "houseno": house, "postcode": "AA1 1AA", "town": "Invented Sparse Town"}})
	}
	enc.Encode(map[string]any{"type": "end", "count": 260})
	if _, err = Import(ctx, source, &sparse); err != nil {
		t.Fatal(err)
	}
	if _, err = IndexAddresses(ctx, source, "parade-sparse", AddressIndexOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err = IndexResearch(ctx, source, "parade-sparse", AddressIndexOptions{}); err != nil {
		t.Fatal(err)
	}
	var postal bytes.Buffer
	postalEncoder := json.NewEncoder(&postal)
	postalEncoder.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: "parade-postal", Edition: "invented-postal", Title: "Invented postcode boundaries", SHA256: map[string]string{"invented": strings.Repeat("c", 64)}, Expected: 4})
	for i, postcode := range []string{"W1 1AA", "W11 1AA", "W11", ""} {
		postalEncoder.Encode(map[string]any{"type": "record", "source_id": fmt.Sprint(i), "record": map[string]any{"surname": "Boundaryonly", "firstName": "June", "postcode": postcode, "postcode_raw": "W1 1AA", "town": "Boundary Town"}})
	}
	postalEncoder.Encode(map[string]any{"type": "end", "count": 4})
	if _, err = Import(ctx, source, &postal); err != nil {
		t.Fatal(err)
	}
	if _, err = IndexAddresses(ctx, source, "parade-postal", AddressIndexOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err = IndexResearch(ctx, source, "parade-postal", AddressIndexOptions{}); err != nil {
		t.Fatal(err)
	}
	role := fmt.Sprintf("parade_reader_%d", time.Now().UnixNano())
	if _, err = target.Exec(ctx, "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS"); err != nil {
		t.Fatal(err)
	}
	defer target.Exec(ctx, "DROP ROLE "+role)
	// DROP OWNED is deliberately limited to the dedicated disposable role.
	defer target.Exec(ctx, "DROP OWNED BY "+role)
	options := ParadeBuildOptions{ReaderRole: role}
	sentinel := errors.New("resource stop")
	calls := 0
	options.CheckResources = func() error {
		calls++
		if calls == 3 {
			return sentinel
		}
		return nil
	}
	if _, err = BuildParadeIndex(ctx, source, target, keys[0], options); !errors.Is(err, sentinel) {
		t.Fatal("interrupted preparation", err)
	}
	options.CheckResources = nil
	if n, e := BuildParadeIndex(ctx, source, target, "all", options); e != nil || n != 304 {
		t.Fatal("complete visible editions", n, e)
	}
	if n, e := BuildParadeIndex(ctx, source, target, "all", options); e != nil || n != 304 {
		t.Fatal("idempotent resume", n, e)
	}
	verification, e := VerifyParadeTrial(ctx, source, target, 8)
	if e != nil || !verification.Passed || len(verification.Editions) != 7 {
		t.Fatal("all-edition verification", verification, e)
	}
	pilot, e := VerifyParadeEditionTrial(ctx, source, target, keys[0], 8)
	if e != nil || !pilot.Passed || len(pilot.Editions) != 1 {
		t.Fatal("single-edition pilot verification", e)
	}
	rawVerification, _ := json.Marshal(verification)
	for _, private := range []string{"Researchonly", "Sparseonly", "AA11AA", "June", "source_id", "generation"} {
		if bytes.Contains(rawVerification, []byte(private)) {
			t.Fatal("private value in verification report")
		}
	}
	cases, e := CreateParadeTrialCases(ctx, source)
	if e != nil || len(cases) == 0 {
		t.Fatal("private workload generation", e)
	}
	for _, c := range cases {
		if c.Path == "/api/search" {
			v, _ := url.ParseQuery(c.Query)
			if _, e := parse(v); e != nil {
				t.Fatal("invalid generated case")
			}
		}
	}
	// Text fields can match while dictionary ordering is corrupt. The independent
	// source-index comparison must reject that separate class of damage.
	originalEdition, e := searchEditions(ctx, source, keys[0])
	if e != nil {
		t.Fatal(e)
	}
	table := pgx.Identifier{"ukinfo_search", originalEdition[0].table()}.Sanitize()
	if _, e = target.Exec(ctx, "UPDATE "+table+" SET name_id=name_id+12345 WHERE id=1"); e != nil {
		t.Fatal(e)
	}
	if _, e = VerifyParadeTrial(ctx, source, target, 8); e == nil || !strings.Contains(e.Error(), "independent") {
		t.Fatal("independent ordering corruption missed", e)
	}
	var originalName int32
	if e = source.QueryRow(ctx, "SELECT name_id FROM entries WHERE dataset_id=$1 AND id=1", originalEdition[0].ID).Scan(&originalName); e != nil {
		t.Fatal(e)
	}
	if _, e = target.Exec(ctx, "UPDATE "+table+" SET name_id=$1 WHERE id=1", originalName); e != nil {
		t.Fatal(e)
	}
	rc, err := pgxpool.ParseConfig(targetDSN)
	if err != nil {
		t.Fatal(err)
	}
	rc.ConnConfig.RuntimeParams["role"] = role
	rc.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	rc.ConnConfig.RuntimeParams["statement_timeout"] = "4000"
	reader, err := pgxpool.NewWithConfig(ctx, rc)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	native, parade := NewAPI(source), NewAPI(source)
	parade.ParadeDB = reader
	metrics := httptest.NewRecorder()
	parade.Metrics().ServeHTTP(metrics, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(metrics.Body.String(), "ukinfo_parade_pool_max ") || !strings.Contains(metrics.Body.String(), "ukinfo_parade_pool_canceled_acquires_total ") {
		t.Fatal("derived pool monitoring missing")
	}
	check := func(q string) {
		t.Helper()
		want := paradeWalk(t, native, native, q)
		got := paradeWalk(t, parade, parade, q)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("different pages for invented query %s", q)
		}
		if got = paradeWalk(t, native, parade, q); !reflect.DeepEqual(got, want) {
			t.Fatalf("native cursor not portable for %s", q)
		}
		if got = paradeWalk(t, parade, native, q); !reflect.DeepEqual(got, want) {
			t.Fatalf("parade cursor not portable for %s", q)
		}
	}
	for _, q := range []string{
		"dataset=all&surname=Researchonly&limit=2",
		"dataset=all&surname=Res*&first_name=Name&first_match=given&limit=2",
		"dataset=all&surname=Researchonly&sort=name&limit=2",
		"dataset=all&postcode=AA1&limit=2",
		"dataset=all&postcode=AA1+1&sort=name&limit=2",
		"dataset=all&street=Invented+Street&house=1&limit=2",
		"dataset=all&street=Invented+Street&sort=name&limit=2",
		"dataset=all&surname=Res*&town=Invented*&first_name=Name&first_match=given&limit=2",
		"dataset=all&kind=business&sic=1234&town=Invented*&limit=2",
		"dataset=all&kind=business&industry=Invented*&town=Invented*&sort=name&limit=2",
		"dataset=all&kind=business&sic=1234&limit=2",
		"dataset=all&kind=business&industry=Invented*&sort=name&limit=2",
		"dataset=all&surname=Researchonly&group=listing&limit=2",
		"dataset=all&surname=Researchonly&sort=edition&limit=2",
		"dataset=all&surname=Absent&limit=2",
		"dataset=parade-postal&postcode=W11&limit=1",
		"dataset=parade-postal&postcode=W1+1&limit=1",
		"dataset=parade-postal&postcode=W11&sort=name&limit=1",
		"dataset=parade-postal&town=Boundary+Town&postcode=W11&limit=1",
		"dataset=parade-postal&town=Boundary+Town&postcode=W1+1&limit=1",
		"dataset=parade-postal&town=Boundary+Town&postcode=W11&sort=name&limit=1",
		"dataset=parade-sparse&surname=Sparseonly&town=Invented+Sparse+Town&house=999&limit=2",
		"dataset=parade-sparse&surname=Sparseonly&town=Invented+Sparse+Town&house=999&sort=name&limit=2",
	} {
		check(q)
	}
	for _, key := range keys {
		check("dataset=" + key + "&surname=Researchonly&limit=2")
	}
	if parade.metrics.plans["paradedb"] == 0 {
		t.Fatal("checks only exercised fallback")
	}
	// The serving SQL must retain the extension's Top-K path for both the
	// compact dictionary ordering and lexical ordering, including continuation.
	editions, e := searchEditions(ctx, source, keys[0])
	if e != nil {
		t.Fatal(e)
	}
	for _, input := range []string{"surname=Researchonly", "postcode=AA1", "street=Invented+Street", "surname=Researchonly&sort=name"} {
		q, e := parse(mustValues("dataset=" + keys[0] + "&limit=2&" + input))
		if e != nil {
			t.Fatal(e)
		}
		positions, e := parade.paradePositions(ctx, q, editions, paradePosition{})
		if e != nil || len(positions) == 0 {
			t.Fatal("plan fixture", e)
		}
		for _, after := range []paradePosition{{}, positions[0]} {
			sql, args := paradeSQL(q, editions[0], after)
			var raw []byte
			if e = reader.QueryRow(ctx, "EXPLAIN(FORMAT JSON) "+sql, args...).Scan(&raw); e != nil {
				t.Fatal(e)
			}
			if !bytes.Contains(raw, []byte("TopKScanExecState")) {
				t.Fatalf("missing Top-K for invented %s continuation=%v", input, after.ID != 0)
			}
		}
	}
	plans, e := CaptureParadeTrialPlans(ctx, source, reader, []ParadeTrialCase{{Path: "/api/search", Query: "dataset=" + keys[0] + "&postcode=AA1"}}, []int{0})
	if e != nil || len(plans) != 4 {
		t.Fatal("prepared plan diagnostics", e)
	}
	planJSON, _ := json.Marshal(plans)
	if bytes.Contains(planJSON, []byte("AA1")) || bytes.Contains(planJSON, []byte("Researchonly")) {
		t.Fatal("private predicate in plan report")
	}
	for _, plan := range plans {
		if plan.Outcome != "ok" || !slices.Contains(plan.Nodes, "Custom Scan:TopKScanExecState") {
			t.Fatal("missing prepared Top-K", plan.Outcome, plan.ErrorCode)
		}
	}
	// Unindexed explicit sample must fall back; all must never include that sample.
	check("dataset=parade-sample&surname=Researchonly&limit=2")
	var count int
	if err = target.QueryRow(ctx, "SELECT count(*) FROM ukinfo_search.editions WHERE ready").Scan(&count); err != nil || count != 7 {
		t.Fatal("sample unexpectedly indexed", count, err)
	}
	if _, err = target.Exec(ctx, "UPDATE ukinfo_search.editions SET ready=false WHERE dataset_id=(SELECT min(dataset_id) FROM ukinfo_search.editions)"); err != nil {
		t.Fatal(err)
	}
	before := parade.metrics.plans["paradedb"]
	check("dataset=all&surname=Researchonly&street=Invented+Street&limit=2")
	if parade.metrics.plans["paradedb"] != before {
		t.Fatal("partial all-edition result served")
	}
	if _, err = target.Exec(ctx, "UPDATE ukinfo_search.editions SET ready=true"); err != nil {
		t.Fatal(err)
	}
	// Same key, manifest and count but a new import generation must also fall back.
	if _, err = source.Exec(ctx, "UPDATE datasets SET imported_at=imported_at+interval '1 second' WHERE key=$1", keys[0]); err != nil {
		t.Fatal(err)
	}
	before = parade.metrics.plans["paradedb"]
	check("dataset=all&surname=Researchonly&street=Invented+Street&limit=2")
	if parade.metrics.plans["paradedb"] != before {
		t.Fatal("stale generation served")
	}
	if _, err = source.Exec(ctx, "UPDATE datasets SET imported_at=imported_at-interval '1 second' WHERE key=$1", keys[0]); err != nil {
		t.Fatal(err)
	}
	// Privileges, not just the read-only connection default, block mutations.
	tx, e := reader.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadWrite})
	if e != nil {
		t.Fatal(e)
	}
	_, e = tx.Exec(ctx, "DELETE FROM ukinfo_search.editions WHERE false")
	tx.Rollback(ctx)
	var denied *pgconn.PgError
	if !errors.As(e, &denied) || denied.Code != "42501" {
		t.Fatal("reader has write privileges", e)
	}
	// Active backend faults fail closed instead of retrying a slow native search.
	if _, err = target.Exec(ctx, "REVOKE SELECT ON ALL TABLES IN SCHEMA ukinfo_search FROM "+role); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	parade.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?dataset=all&surname=Researchonly&street=Invented+Street", nil))
	if w.Code != 503 {
		t.Fatal("reader fault did not fail closed", w.Code)
	}
	w = httptest.NewRecorder()
	parade.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?dataset=all&postcode=AA1", nil))
	if w.Code != 200 {
		t.Fatal("native postcode search touched denied target", w.Code)
	}
	w = httptest.NewRecorder()
	parade.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/health", nil))
	if w.Code != 200 {
		t.Fatal("source health depended on preview")
	}
	if _, err = reader.Exec(ctx, "DELETE FROM ukinfo_search.editions WHERE false"); err == nil {
		t.Fatal("reader can write")
	}
}
func paradeWalk(t *testing.T, first, rest *API, raw string) []result {
	t.Helper()
	v, _ := url.ParseQuery(raw)
	out := []result{}
	seen := map[string]bool{}
	for page := 0; page < 400; page++ {
		api := rest
		if page == 0 {
			api = first
		}
		w := httptest.NewRecorder()
		api.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?"+v.Encode(), nil))
		if w.Code != 200 {
			t.Fatalf("invented search failed %d: %s", w.Code, w.Body.String())
		}
		var r searchResult
		if e := json.Unmarshal(w.Body.Bytes(), &r); e != nil {
			t.Fatal(e)
		}
		out = append(out, r.Items...)
		if r.Next == "" {
			return out
		}
		if seen[r.Next] {
			t.Fatal("repeated cursor")
		}
		seen[r.Next] = true
		v.Set("cursor", r.Next)
	}
	t.Fatal("unbounded fixture pagination")
	return nil
}
