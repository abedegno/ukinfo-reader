package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
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
	var dense bytes.Buffer
	denseEncoder := json.NewEncoder(&dense)
	denseEncoder.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: "parade-dense", Edition: "invented-2003-01", Title: "Invented dense name-pair groups", SHA256: map[string]string{"invented": strings.Repeat("d", 64)}, Expected: 320})
	for i := 0; i < 320; i++ {
		denseEncoder.Encode(map[string]any{"type": "record", "source_id": fmt.Sprint(i), "record": map[string]any{"surname": "Denseonly", "firstName": "June", "postcode": "AA1 1AA", "street": "Invented Dense Road", "town": "Dense Town", "houseno": fmt.Sprint(i + 1)}})
	}
	denseEncoder.Encode(map[string]any{"type": "end", "count": 320})
	if _, err = Import(ctx, source, &dense); err != nil {
		t.Fatal(err)
	}
	if _, err = IndexAddresses(ctx, source, "parade-dense", AddressIndexOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err = IndexResearch(ctx, source, "parade-dense", AddressIndexOptions{}); err != nil {
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
	if n, e := BuildParadeIndex(ctx, source, target, "all", options); e != nil || n != 624 {
		t.Fatal("complete visible editions", n, e)
	}
	if n, e := BuildParadeIndex(ctx, source, target, "all", options); e != nil || n != 624 {
		t.Fatal("idempotent resume", n, e)
	}
	verification, e := VerifyParadeTrial(ctx, source, target, 8)
	if e != nil || !verification.Passed || len(verification.Editions) != 8 {
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
	rc.ConnConfig.RuntimeParams["plan_cache_mode"] = "force_generic_plan"
	reader, err := pgxpool.NewWithConfig(ctx, rc)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	// Invented coordinates exercise the real source radius resolver, including
	// retired postcodes and an empty radius, without adding source records.
	if _, err = source.Exec(ctx, `
 INSERT INTO geography_releases(release,source_url,sha256,scope,entries,located,excluded_ni,excluded_other)
 VALUES('2026-08','https://example.org/invented-onspd',repeat('a',64),'GB',10004,10004,0,0);
 CREATE TABLE postcode_locations_parade PARTITION OF postcode_locations FOR VALUES IN ('2026-08');
 INSERT INTO postcode_locations(release,postcode,introduced,terminated,country,quality,latitude,longitude)
 VALUES('2026-08','AA11AA','198001','','E92000001',1,51,-1),
 ('2026-08','BB11BB','198001','200102','E92000001',8,51.001,-1),
 ('2026-08','CC11CC','198001','','E92000001',1,52,-1),
 ('2026-08','W111AA','198001','','E92000001',1,51.3,-1);

 -- Unused but real dictionary IDs make both radius sets exceed the SQL-term
 -- budget. One set surrounds AA1; the other surrounds the empty CC1 centre.
 INSERT INTO terms(key)
 SELECT 'ZX'||area::text||(n/676)::text||chr(65+(n%676)/26)||chr(65+n%26)
 FROM generate_series(1,2) area CROSS JOIN generate_series(0,4999)n;
 INSERT INTO postcode_locations(release,postcode,introduced,terminated,country,quality,latitude,longitude)
 SELECT '2026-08','ZX'||area::text||(n/676)::text||chr(65+(n%676)/26)||chr(65+n%26),
 '198001','','E92000001',1,50.1+area,-1
 FROM generate_series(1,2) area CROSS JOIN generate_series(0,4999)n;
 INSERT INTO geography_active(singleton,release) VALUES(true,'2026-08');`); err != nil {
		t.Fatal(err)
	}
	native, parade := NewAPI(source), NewAPI(source)
	parade.ParadeDB = reader
	// A three-postcode batch must use the existing postcode primary key rather
	// than scan the 10,000 unrelated points. Dictionary C collation must not
	// silently make the database-default geography index unusable.
	if _, err = source.Exec(ctx, "ANALYZE terms; ANALYZE postcode_locations"); err != nil {
		t.Fatal(err)
	}
	var geoIDs []int32
	if err = source.QueryRow(ctx, "SELECT array_agg(id) FROM terms WHERE key=ANY($1::text[])", []string{"AA11AA", "BB11BB", "W111AA"}).Scan(&geoIDs); err != nil {
		t.Fatal(err)
	}
	var geoPlan []byte
	if err = source.QueryRow(ctx, "EXPLAIN(ANALYZE,BUFFERS,FORMAT JSON) "+candidateGeographySQL, "2026-08", geoIDs, unitPosition(51, -1), 2*math.Sin(1000/(2*earthRadius))).Scan(&geoPlan); err != nil {
		t.Fatal(err)
	}
	var geoPlans []map[string]any
	if json.Unmarshal(geoPlan, &geoPlans) != nil || len(geoPlans) != 1 {
		t.Fatal("invalid candidate geography plan")
	}
	geoRoot := geoPlans[0]["Plan"].(map[string]any)
	if geoRoot["Actual Rows"] != float64(2) || geoRoot["Shared Hit Blocks"].(float64)+geoRoot["Shared Read Blocks"].(float64) > 64 {
		t.Fatal("candidate geography scanned unrelated postcodes")
	}
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
		"dataset=all&postcode=AA1&sort=name&limit=2",
		"dataset=all&street=Invented+Street&near=AA11AA&radius_m=1000&limit=2",
		"dataset=all&street=Invented+Street&near=BB11BB&radius_m=1000&limit=2",
		"dataset=all&street=Invented+Street&near=AA1&radius_m=1000&limit=2",
		"dataset=all&street=Invented+Street&near=AA1+1&radius_m=1000&limit=2",
		"dataset=all&street=Invented+Street&near=CC11CC&radius_m=1000&limit=2",
		"dataset=all&street=Invented+Street&near=AA11AA&house=1&radius_m=1000&limit=2",
		"dataset=all&kind=business&street=Invented+Street&near=AA11AA&radius_m=1000&limit=2",
		"dataset=all&street=Invented+Street&house=1&limit=2",
		"dataset=all&street=Invented+Street&sort=name&limit=2",
		"dataset=all&surname=Res*&town=Invented*&first_name=Name&first_match=given&limit=2",
		"dataset=all&kind=business&sic=1234&town=Invented*&limit=2",
		"dataset=all&kind=business&industry=Invented*&town=Invented*&sort=name&limit=2",
		"dataset=all&kind=business&sic=1234&limit=2",
		"dataset=all&kind=business&industry=Invented*&sort=name&limit=2",
		"dataset=all&surname=Researchonly&group=listing&limit=2",
		"dataset=all&street=Invented+Street&group=listing&limit=2",
		"dataset=all&street=Invented+Street&group=listing&sort=name&house=1&limit=2",
		"dataset=all&street=Invented*&sort=edition&limit=37",
		"dataset=parade-dense&town=Dense+Town&group=listing&limit=37",
		"dataset=parade-dense&town=Dense+Town&group=listing&house=319&limit=2",
		"dataset=parade-sparse&town=Invented+Sparse+Town&group=listing&limit=1",
		"dataset=parade-sparse&town=Invented+Sparse+Town&group=listing&house=999&limit=1",
		"dataset=parade-sparse&town=Invented+Sparse+Town&sort=edition&house=999&limit=2",

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
	testParadeGroupWork(t, source, reader)
	for _, key := range keys {
		check("dataset=" + key + "&surname=Researchonly&limit=2")
	}
	// Deep pages cross the ten-page real-data boundary. Both backends and
	// cursor exchanges must also agree with the independently known total.
	for _, suffix := range []string{"sort=name", "group=listing"} {
		q := "dataset=parade-dense&town=Dense+Town&limit=7&" + suffix
		check(q)
		if rows := paradeWalk(t, parade, parade, q); len(rows) != 320 {
			t.Fatal("deep traversal lost dense-pair occurrences", len(rows))
		}
	}
	for _, q := range []string{
		"dataset=all&surname=Res*&first_name=Na*&first_match=prefix&street=Invented*&limit=1",
		"dataset=all&street=Invented*&sort=name&limit=7",
		"dataset=all&town=Invented*&group=listing&limit=1",
		"dataset=parade-postal&town=Boundary+Town&near=AA11AA&radius_m=50000&limit=1",
		"dataset=parade-postal&town=Boundary+Town&near=AA1&radius_m=50000&limit=1",
		"dataset=parade-postal&town=Boundary+Town&near=AA1+1&radius_m=50000&limit=1",
	} {
		check(q)
	}
	check("dataset=parade-dense&town=Dense+Town&near=CC11CC&radius_m=50000&limit=1")
	if rows := paradeWalk(t, parade, parade, "dataset=parade-dense&town=Dense+Town&near=CC11CC&radius_m=50000&limit=1"); len(rows) != 0 {
		t.Fatal("large radius silently discarded its postcode membership test")
	}
	far := paradeWalk(t, parade, parade, "dataset=parade-postal&town=Boundary+Town&near=AA11AA&radius_m=50000&limit=1")
	if len(far) != 1 || far[0].Distance == nil || *far[0].Distance < 30000 || *far[0].Distance > 35000 {
		t.Fatal("maximum radius did not include the known distant postcode")
	}
	if close := paradeWalk(t, parade, parade, "dataset=parade-postal&town=Boundary+Town&near=AA11AA&radius_m=1000&limit=1"); len(close) != 0 {
		t.Fatal("small radius included the distant postcode")
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
	for _, input := range []string{"surname=Researchonly", "postcode=AA1", "postcode=AA1&sort=name", "street=Invented+Street", "surname=Researchonly&sort=name", "street=Invented+Street&sort=edition", "street=Invented+Street&near=AA11AA&radius_m=1000"} {
		q, e := parse(mustValues("dataset=" + keys[0] + "&limit=2&" + input))
		if e != nil {
			t.Fatal(e)
		}
		if q.Near != "" {
			points, _, err := parade.nearby(ctx, &q)
			if err != nil {
				t.Fatal(err)
			}
			for id := range points {
				q.nearbyPostcodes = append(q.nearbyPostcodes, id)
			}
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
	if err = target.QueryRow(ctx, "SELECT count(*) FROM ukinfo_search.editions WHERE ready").Scan(&count); err != nil || count != 8 {
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
	testNativeReferencePrefix(t, source, "dataset=parade-dense&town=Dense+Town&group=listing&limit=37")
	testNativeReferencePrefix(t, source, "dataset=all&street=Invented*&sort=edition&limit=37")
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

// Count actual query work as well as comparing all pages. A dense pair extends
// beyond the lexical discovery window, but target probes must drain it in a
// bounded numeric batch without rereading other editions or decoding unused names.
func testParadeGroupWork(t *testing.T, source, target *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	a := NewAPI(source)
	a.ParadeDB = target
	raw := "dataset=parade-dense&town=Dense+Town&group=listing&limit=37"
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?"+raw, nil))
	var out searchResult
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || len(out.Items) != 37 || out.Next == "" {
		t.Fatal("dense group discovery failed", w.Code)
	}
	discoveries, drains := a.metrics.phases[phaseKey{"parade_candidates", "ok"}].Count, a.metrics.phases[phaseKey{"parade_pair", "ok"}].Count
	if discoveries != 1 || drains != 1 {
		t.Fatal("dense group pair was repeatedly rediscovered")
	}
	later := false
	for _, item := range out.Items {
		if id, e := strconv.Atoi(item.SourceID); e == nil && id >= 128 {
			later = true
		}
	}
	if !later {
		t.Fatal("fixture did not require groups beyond the discovery window")
	}
	editions, e := searchEditions(ctx, source, "parade-dense")
	if e != nil {
		t.Fatal(e)
	}
	q, e := parse(mustValues("dataset=parade-dense&town=Dense+Town&limit=100"))
	if e != nil {
		t.Fatal(e)
	}
	sql, args := paradeSQLBounds(q, editions[0], paradePosition{}, true, &paradeNamePair{Name: Normalize("Denseonly"), First: Normalize("June")})
	var rawPlan []byte
	if e = target.QueryRow(ctx, "EXPLAIN(ANALYZE,BUFFERS,FORMAT JSON) "+sql, args...).Scan(&rawPlan); e != nil {
		t.Fatal(e)
	}
	var plans []map[string]any
	if json.Unmarshal(rawPlan, &plans) != nil || len(plans) != 1 {
		t.Fatal("invalid numeric pair plan")
	}
	heap := float64(0)
	var walk func(map[string]any)
	walk = func(n map[string]any) {
		if v, ok := n["Heap Fetches"].(float64); ok {
			heap += v
		}
		if children, ok := n["Plans"].([]any); ok {
			for _, child := range children {
				walk(child.(map[string]any))
			}
		}
	}
	walk(plans[0]["Plan"].(map[string]any))
	if heap > float64(q.Limit) || !bytes.Contains(rawPlan, []byte("TopKScanExecState")) {
		t.Fatalf("numeric pair exceeded bounded Top-K work, heap fetches=%g", heap)
	}
	// A publication hole must fail explicitly rather than omit one occurrence.
	var id int32
	var sourceHash, group []byte
	var sic, industry string
	dataset := editions[0].ID
	if e = source.QueryRow(ctx, "DELETE FROM research_keys WHERE dataset_id=$1 AND id=1 RETURNING id,source_hash,group_key,sic,industry", dataset).Scan(&id, &sourceHash, &group, &sic, &industry); e != nil {
		t.Fatal(e)
	}
	defer func() {
		if _, e := source.Exec(ctx, "INSERT INTO research_keys(dataset_id,id,source_hash,group_key,sic,industry) VALUES($1,$2,$3,$4,$5,$6)", dataset, id, sourceHash, group, sic, industry); e != nil {
			t.Fatal(e)
		}
	}()
	w = httptest.NewRecorder()
	a.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?"+raw, nil))
	if w.Code != 503 || !bytes.Contains(w.Body.Bytes(), []byte("catalogue_integrity")) {
		t.Fatal("missing group key did not fail closed", w.Code)
	}
	if output := os.Getenv("UKINFO_PARADE_WORK_REPORT"); output != "" {
		if e := os.MkdirAll(filepath.Dir(output), 0755); e != nil {
			t.Fatal(e)
		}
		raw, e := json.MarshalIndent(map[string]any{"conditions": "invented dense-pair fixture; execution work only", "fixture_rows": 320, "page_size": 37, "discovery_scans": discoveries, "pair_drains": drains, "numeric_plan_heap_fetches": heap, "numeric_plan_limit": q.Limit, "missing_source_key_rejected": true, "passed": true}, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(output, append(raw, '\n'), 0644); e != nil {
			t.Fatal(e)
		}
	}
}

func testNativeReferencePrefix(t *testing.T, db *pgxpool.Pool, raw string) {
	t.Helper()
	a := NewAPI(db)
	values, _ := url.ParseQuery(raw)
	original := values.Encode()
	gold := []searchResult{}
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?"+values.Encode(), nil))
		var result searchResult
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Items) == 0 {
			t.Fatal("native oracle fixture", w.Code)
		}
		gold = append(gold, result)
		values.Set("cursor", result.Next)
	}
	values, _ = url.ParseQuery(original)
	bound := gold[2].Items[0]
	pages, e := NativeOrderedPrefixReference(context.Background(), db, values, bound.Dataset, int32(bound.ID))
	if e != nil || len(pages) != 2 {
		t.Fatal("source prefix oracle", e)
	}
	for i, body := range pages {
		var got searchResult
		if json.Unmarshal(body, &got) != nil || !reflect.DeepEqual(got.Items, gold[i].Items) || (got.Next != "") != (gold[i].Next != "") {
			t.Fatal("prefix excluded an earlier source result")
		}
	}
}
