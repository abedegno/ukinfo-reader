package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSelectiveAddressIntegration(t *testing.T) {
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
	schema := fmt.Sprintf("ukinfo_address_test_%d", os.Getpid())
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
	for _, key := range []string{"selective-one", "selective-two"} {
		var input bytes.Buffer
		enc := json.NewEncoder(&input)
		_ = enc.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: key, Edition: key, Title: "Invented address plan test", SHA256: map[string]string{"invented": "invented"}, Expected: 273})
		for i := 0; i < 273; i++ {
			name := "Smallmatch"
			if i >= 12 {
				name = "Boundarymatch"
			}
			if i >= 140 {
				name = "Broadmatch"
			}
			r := map[string]any{"surname": name, "firstName": "June A", "street_2": "Invented Road", "town": "Invented Town", "county": "Old County", "postcode": "AA1 1AA", "houseno": fmt.Sprint(i + 1)}
			if i%2 == 1 {
				r["town"] = "Other Town"
			}
			if i >= 269 {
				r["entry_type"] = "business"
				r["organisation"] = "Smallmatch Shop"
			}
			_ = enc.Encode(map[string]any{"type": "record", "source_id": fmt.Sprint(i), "record": r})
		}
		_ = enc.Encode(map[string]any{"type": "end", "count": 273})
		if _, err = Import(ctx, db, &input); err != nil {
			t.Fatal(err)
		}
		if _, err = IndexAddresses(ctx, db, key, AddressIndexOptions{}); err != nil {
			t.Fatal(err)
		}
		if _, err = IndexResearch(ctx, db, key, AddressIndexOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	var bulk bytes.Buffer
	enc := json.NewEncoder(&bulk)
	_ = enc.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: "large-address", Edition: "large-address", Title: "Invented address budgets", Expected: 22000, SHA256: map[string]string{"invented": "invented"}})
	for i := 0; i < 22000; i++ {
		name, postcode := "Widebudget", "BB1 1BB"
		if i >= 4500 {
			name = "Packedbudget"
		}
		if i >= 5010 {
			name = "Fillerbudget"
			if i%128 == 0 {
				name = "Scatteredbudget"
			}
		}
		if i < 300 {
			postcode = "CC1 1CC"
		}
		row := map[string]any{"surname": name, "firstName": "June A", "street": "Large Road", "town": "Large Town", "county": "Old County", "postcode": postcode, "houseno": fmt.Sprint(i + 1)}
		_ = enc.Encode(map[string]any{"type": "record", "source_id": fmt.Sprint(i), "record": row})
	}
	_ = enc.Encode(map[string]any{"type": "end", "count": 22000})
	if _, err = Import(ctx, db, &bulk); err != nil {
		t.Fatal(err)
	}
	if _, err = IndexAddresses(ctx, db, "large-address", AddressIndexOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err = IndexResearch(ctx, db, "large-address", AddressIndexOptions{}); err != nil {
		t.Fatal(err)
	}
	testAddressLookupIntegrity(t, db)
	testAddressLookupDictionaryPlan(t, db)
	if _, err = db.Exec(ctx, "INSERT INTO terms(key) SELECT 'jaaa'||lpad(i::text,5,'0') FROM generate_series(0,8192) i ON CONFLICT DO NOTHING"); err != nil {
		t.Fatal(err)
	}
	// Invented coordinate reference: nearby tests never depend on live geocoding.
	if _, err = db.Exec(ctx, `INSERT INTO geography_releases(release,source_url,sha256,scope,entries,located,excluded_ni,excluded_other) VALUES('2026-08','https://example.org/invented','invented','GB',3,3,0,0);
CREATE TABLE postcode_locations_matrix PARTITION OF postcode_locations FOR VALUES IN ('2026-08');
INSERT INTO postcode_locations(release,postcode,introduced,terminated,country,quality,latitude,longitude) VALUES
('2026-08','AA11AA','198001','','E92000001',1,51,-1),
('2026-08','BB11BB','198001','','E92000001',1,52,-1),
('2026-08','CC11CC','198001','','E92000001',1,53,-1);
INSERT INTO geography_active VALUES(true,'2026-08'); ANALYZE;`); err != nil {
		t.Fatal(err)
	}
	testRequestBlockReuse(t, db)
	type measurement struct {
		Case      int     `json:"case"`
		Query     string  `json:"invented_query"`
		Pages     int     `json:"pages"`
		Results   int     `json:"results"`
		FirstMS   float64 `json:"first_page_ms"`
		MaxNextMS float64 `json:"max_next_page_ms"`
	}
	observations := []measurement{}
	a := NewAPI(db)
	for caseIndex, tc := range []struct {
		raw       string
		selective bool
		count     int
	}{
		{"dataset=all&surname=Smallmatch&first_name=June&first_match=given&town=Invented+Town", true, 12},
		{"dataset=selective-one&surname=Boundarymatch&town=Invented+Town", true, 64},
		{"dataset=all&surname=Boundarymatch&town=Invented+Town", true, 128},
		{"dataset=selective-one&surname=Broadmatch&town=Invented+Town", true, 65},
		{"dataset=all&kind=business&organisation=Smallmatch+Shop&town=Invented+Town", true, 4},
		{"dataset=all&surname=Smallmatch&street=Invented*&county=Old*&town=Invented+Town&sort=name", true, 12},
		{"dataset=all&surname=Smallmatch&town=Invented+Town&sort=edition", true, 12},
		{"dataset=all&surname=Smallmatch&town=Invented+Town&group=listing", true, 6},
		{"dataset=all&surname=Smallmatch&town=Missing+Town", true, 0},
		{"dataset=all&postcode=CC11CC&town=Large+Town", true, 300},
		{"dataset=all&postcode=CC11CC&town=Large+Town&house=299", true, 1},
		{"dataset=all&surname=Packedbudget&town=Large+Town", true, 510},
		{"dataset=large-address&surname=Widebudget&town=Large+Town", false, 4500},
		{"dataset=all&street=Invented+Road&town=Invented+Town", true, 270},
		{"dataset=large-address&surname=Scatteredbudget&town=Large+Town", true, 132},
		{"dataset=all&surname=Smallmatch&first_name=J*&middle_initial=A&town=Invented+Town", true, 12},
		{"dataset=all&surname=Widebudget&first_name=June&first_match=given&postcode=BB1", false, 4200},
		{"dataset=all&surname=Wide*&postcode=CC1+1", false, 300},
		{"dataset=all&surname=Smallmatch&postcode=AA11AA", false, 24},
		{"dataset=all&surname=Smallmatch&near=AA1&radius_m=1000", false, 24},
		{"dataset=all&kind=business&organisation=Smallmatch*&postcode=AA1", false, 8},
		{"dataset=all&surname=Missingmatch&postcode=BB1", false, 0},
	} {
		t.Run(fmt.Sprintf("case-%02d", caseIndex), func(t *testing.T) {
			observation := measurement{Case: caseIndex, Query: tc.raw}
			q, err := parse(mustValues(tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			f, status, err := a.candidateFilters(ctx, q)
			if err != nil || status != 200 || f.addressMatched != tc.selective {
				t.Fatalf("plan: selective=%v status=%d err=%v", f.addressMatched, status, err)
			}
			if strings.Contains(tc.raw, "Scatteredbudget") && a.metrics.plans["indexed_source"] == 0 {
				t.Fatal("scattered source candidates did not use the compact address lookup")
			}
			pageLimit := 3
			if tc.count > 100 {
				pageLimit = 100
			}
			seen := map[string]bool{}
			cursor := ""
			total := 0
			for page := 0; ; page++ {
				if page > 1600 {
					t.Fatal("endless pagination")
				}
				w := httptest.NewRecorder()
				started := time.Now()
				a.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?"+tc.raw+fmt.Sprintf("&limit=%d&cursor=", pageLimit)+url.QueryEscape(cursor), nil))
				elapsed := float64(time.Since(started)) / float64(time.Millisecond)
				observation.Pages++
				if page == 0 {
					observation.FirstMS = elapsed
				} else if elapsed > observation.MaxNextMS {
					observation.MaxNextMS = elapsed
				}
				if w.Code != 200 {
					t.Fatal(w.Code, w.Body.String())
				}
				var out searchResult
				if err = json.Unmarshal(w.Body.Bytes(), &out); err != nil {
					t.Fatal(err)
				}
				for _, r := range out.Items {
					identity := r.Dataset + ":" + r.SourceID
					if seen[identity] {
						t.Fatal("duplicate page entry", identity)
					}
					seen[identity] = true
					if q.Town != "" && !strings.Contains(string(r.Record), "Invented Town") && !strings.Contains(string(r.Record), "Large Town") {
						t.Fatal("wrong town")
					}
				}
				total += len(out.Items)
				if out.Next == "" {
					break
				}
				cursor = out.Next
			}
			if total != tc.count {
				t.Fatalf("got %d entries, want %d", total, tc.count)
			}
			observation.Results = total
			observations = append(observations, observation)
		})
	}
	if output := os.Getenv("UKINFO_PERFORMANCE_REPORT_DIR"); output != "" {
		if err := os.MkdirAll(output, 0o755); err != nil {
			t.Fatal(err)
		}
		raw, err := json.MarshalIndent(map[string]any{"fixture_entries": 22546, "conditions": "cached invented corpus; handler and JSON included; HTTP transport excluded; race instrumentation may be enabled", "passed": !t.Failed(), "cases": observations}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(output, "search-matrix.json"), append(raw, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	testOrderedSurnameWindows(t, db)
	testSourceProbeReuse(t, db)
	testSearchPhaseRows(t, db)
	testClassificationProbeWork(t, db)
}

func testAddressLookupIntegrity(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	var wrong int
	// Compare every slot, including block boundaries and the short last block,
	// against the independently stored forward address memberships.
	if err := db.QueryRow(ctx, `SELECT count(*) FROM address_members m
 CROSS JOIN LATERAL unnest(m.ids) occurrence
 LEFT JOIN address_record_blocks b ON b.dataset_id=m.dataset_id AND b.block_id=(occurrence-1)/128
 WHERE b.location_ids[(occurrence-1)%128+1] IS DISTINCT FROM m.location_id`).Scan(&wrong); err != nil || wrong != 0 {
		t.Fatalf("reverse address slots differ: %d, %v", wrong, err)
	}
	// A duplicate in a posting must reject the entire rebuild, not publish a
	// shifted or incomplete array. Rollback also preserves the previous index.
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `DELETE FROM address_record_indexes WHERE dataset_id=(SELECT id FROM datasets WHERE key='large-address');
 DELETE FROM address_record_blocks WHERE dataset_id=(SELECT id FROM datasets WHERE key='large-address');
 UPDATE address_members SET ids=array_prepend(ids[1],ids)
 WHERE (dataset_id,location_id)=(SELECT dataset_id,location_id FROM address_members
 WHERE dataset_id=(SELECT id FROM datasets WHERE key='large-address') ORDER BY location_id LIMIT 1)`); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, "SELECT build_address_record_lookup('large-address')")
	if err == nil || !strings.Contains(err.Error(), "missing, duplicate or out-of-range") {
		t.Fatalf("corrupt lookup accepted: %v", err)
	}
	_ = tx.Rollback(ctx)
	// A catalogue upgraded before its optional lookup is backfilled must keep
	// the legacy complete intersection instead of treating missing coverage as
	// a negative match.
	if _, err = db.Exec(ctx, "DELETE FROM address_record_indexes WHERE dataset_id=(SELECT id FROM datasets WHERE key='large-address')"); err != nil {
		t.Fatal(err)
	}
	a := NewAPI(db)
	q, err := parse(mustValues("dataset=large-address&surname=Scatteredbudget&town=Large+Town"))
	if err != nil {
		t.Fatal(err)
	}
	f, status, err := a.candidateFilters(ctx, q)
	if err != nil || status != 200 || f.addressMatched {
		t.Fatalf("legacy fallback lost: matched=%v status=%d err=%v", f.addressMatched, status, err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO address_record_indexes(dataset_id,version,entries,blocks)
 SELECT id,1,entries,(entries+127)/128 FROM datasets WHERE key='large-address'`); err != nil {
		t.Fatal(err)
	}
	var dataset int16
	var original []int32
	if err = db.QueryRow(ctx, `SELECT b.dataset_id,b.location_ids FROM address_record_blocks b
 JOIN datasets d ON d.id=b.dataset_id WHERE d.key='large-address' AND b.block_id=0`).Scan(&dataset, &original); err != nil {
		t.Fatal(err)
	}
	restore := func() {
		if _, e := db.Exec(ctx, `INSERT INTO address_record_blocks(dataset_id,block_id,location_ids)
 VALUES($1,0,$2) ON CONFLICT(dataset_id,block_id) DO UPDATE SET location_ids=EXCLUDED.location_ids`, dataset, original); e != nil {
			t.Fatal(e)
		}
	}
	defer restore()
	for _, mutation := range []string{
		"DELETE FROM address_record_blocks WHERE dataset_id=$1 AND block_id=0",
		"UPDATE address_record_blocks SET location_ids=location_ids[1:1] WHERE dataset_id=$1 AND block_id=0",
		"UPDATE address_record_blocks SET location_ids[128]=0 WHERE dataset_id=$1 AND block_id=0",
	} {
		if _, err = db.Exec(ctx, mutation, dataset); err != nil {
			t.Fatal(err)
		}
		if _, available, e := a.indexedAddressCandidates(ctx, q, map[int16][]int32{dataset: {1, 128}}); !available || e == nil {
			t.Fatalf("damaged published lookup silently accepted: available=%v err=%v", available, e)
		}
		restore()
	}

}

func testOrderedSurnameWindows(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	var input bytes.Buffer
	enc := json.NewEncoder(&input)
	_ = enc.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: "surname-windows", Edition: "surname-windows", Title: "Invented ordered names", Expected: 360, SHA256: map[string]string{"invented": "invented"}})
	for i := 359; i >= 0; i-- {
		name := fmt.Sprintf("Surnamewindow%04d", 2*i)
		_ = enc.Encode(map[string]any{"type": "record", "source_id": fmt.Sprint(i), "record": map[string]any{"surname": name, "firstName": "Name A", "street": "Invented Road", "houseno": "1", "postcode": "AA1 1AA"}})
	}
	_ = enc.Encode(map[string]any{"type": "end", "count": 360})
	if _, err := Import(ctx, db, &input); err != nil {
		t.Fatal(err)
	}
	if _, err := IndexResearch(ctx, db, "surname-windows", AddressIndexOptions{}); err != nil {
		t.Fatal(err)
	}
	// Unused terms must not consume the bounded surname window.
	if _, err := db.Exec(ctx, "INSERT INTO terms(key) SELECT 'surnamewindow'||lpad((i*2+1)::text,4,'0') FROM generate_series(0,359) i ON CONFLICT DO NOTHING"); err != nil {
		t.Fatal(err)
	}
	a := NewAPI(db)
	for _, mode := range []string{"surname=Surnamewindow*&sort=name", "surname=Surnamewindow*&group=listing", "postcode=AA1&sort=name", "postcode=AA1&group=listing"} {
		probe := NewAPI(db)
		w := httptest.NewRecorder()
		probe.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?dataset=surname-windows&limit=1&"+mode, nil))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var firstPage searchResult
		if err := json.Unmarshal(w.Body.Bytes(), &firstPage); err != nil {
			t.Fatal(err)
		}
		if len(firstPage.Items) != 1 || firstPage.Next == "" {
			t.Fatal("missing ordered page/lookahead")
		}
		// One label query and one sparse two-pair batch. Remaining 126 pairs
		// must not expand sources or groups only to be discarded after sorting.
		if got := probe.metrics.phases[phaseKey{"ordered", "ok"}].Count; got != 2 || probe.metrics.orderedPairs.Load() != 2 {
			t.Fatalf("ordered pair page work: got %d queries, want 2", got)
		}
		wide := NewAPI(db)
		w = httptest.NewRecorder()
		wide.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?dataset=surname-windows&limit=100&"+mode, nil))
		if w.Code != 200 || wide.metrics.orderedPairs.Load() != 101 || wide.metrics.phases[phaseKey{"ordered", "ok"}].Count != 27 {
			t.Fatal("sparse page transaction work", w.Code)
		}
		seen := map[string]bool{}
		cursor := ""
		last := ""
		for page := 0; ; page++ {
			if page > 10 {
				t.Fatal("endless surname pages")
			}
			w := httptest.NewRecorder()
			a.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?dataset=surname-windows&first_name=Name&first_match=given&limit=100&"+mode+"&cursor="+url.QueryEscape(cursor), nil))
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			var out searchResult
			if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			for _, r := range out.Items {
				var row map[string]any
				_ = json.Unmarshal(r.Record, &row)
				name := row["surname"].(string)
				if name <= last || seen[r.SourceID] {
					t.Fatal("wrong surname order or duplicate", name)
				}
				last = name
				seen[r.SourceID] = true
			}
			if out.Next == "" {
				break
			}
			cursor = out.Next
		}
		if len(seen) != 360 {
			t.Fatal("truncated surname windows", len(seen))
		}
	}
	for _, mode := range []string{"sort=name", "group=listing"} {
		dense := NewAPI(db)
		w := httptest.NewRecorder()
		dense.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?dataset=large-address&postcode=BB1&limit=1&"+mode, nil))
		if w.Code != 200 || dense.metrics.orderedPairs.Load() != 1 {
			t.Fatal("dense pair expanded unnecessary neighbours", w.Code)
		}
	}

}

// Use a wide, mostly unrelated dictionary so a sparse ID-driven heap probe is
// observably more work than the town covering range, even on a cached CI runner.
func testAddressLookupDictionaryPlan(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.Exec(ctx, `INSERT INTO address_locations(key,street,street_2,locality,locality_2,town,county)
 SELECT 'lookup-plan:'||i,repeat('invented ',32)||i,'','','',
 CASE WHEN i%5=0 THEN 'invented plan town' ELSE 'unrelated plan town' END,'invented plan county'
 FROM generate_series(1,20000) i`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "VACUUM (ANALYZE) address_locations"); err != nil {
		t.Fatal(err)
	}
	var candidates, expected []int32
	if err := db.QueryRow(ctx, `SELECT array_agg(id ORDER BY id) FROM address_locations
 WHERE key LIKE 'lookup-plan:%' AND id%7=0`).Scan(&candidates); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT array_agg(id ORDER BY id) FROM address_locations
 WHERE town='invented plan town' AND id=ANY($1::integer[])`, candidates).Scan(&expected); err != nil {
		t.Fatal(err)
	}
	args := []any{candidates}
	sql := addressLocationLookupSQL(query{Town: "invented plan town"}, &args)
	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		t.Fatal(err)
	}
	actual := []int32{}
	for rows.Next() {
		var id int32
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		actual = append(actual, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(actual, func(i, j int) bool { return actual[i] < actual[j] })
	if !slices.Equal(actual, expected) {
		t.Fatal("dictionary intersection changed results")
	}
	var raw []byte
	if err = db.QueryRow(ctx, "EXPLAIN (ANALYZE,BUFFERS,TIMING OFF,FORMAT JSON) "+sql, args...).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var plan []map[string]any
	if err = json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	top := plan[0]["Plan"].(map[string]any)
	blocks := func(n map[string]any, k string) float64 {
		if v, ok := n[k].(float64); ok {
			return v
		}
		return 0
	}
	if blocks(top, "Shared Hit Blocks")+blocks(top, "Shared Read Blocks") > 500 {
		t.Fatal("dictionary lookup exceeded buffer budget")
	}
	var inspect func(map[string]any)
	inspect = func(n map[string]any) {
		if n["Relation Name"] == "address_locations" && (n["Node Type"] != "Index Only Scan" || blocks(n, "Heap Fetches") != 0) {
			t.Fatal("dictionary lookup fetched address heaps")
		}
		if children, ok := n["Plans"].([]any); ok {
			for _, child := range children {
				inspect(child.(map[string]any))
			}
		}
	}
	inspect(top)
	// An existing index with the right name but wrong key order must fail closed.
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `DROP INDEX address_record_block_ids; CREATE INDEX address_record_block_ids ON address_record_blocks(block_id,dataset_id)`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, addressLookupSchema); err == nil || !strings.Contains(err.Error(), "Incompatible or incomplete") {
		t.Fatalf("incorrect lookup index accepted: %v", err)
	}
}

func testSourceProbeReuse(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	a := NewAPI(db)
	q, err := parse(mustValues("dataset=large-address&surname=Widebudget&town=Large+Town"))
	if err != nil {
		t.Fatal(err)
	}
	f, status, err := a.candidateFilters(ctx, q)
	if err != nil || status != 200 || f.addressMatched {
		t.Fatalf("wrong overflow fallback: %d %v", status, err)
	}
	if got := a.metrics.phases[phaseKey{"source_probe", "ok"}].Count; got != 1 {
		t.Fatalf("identical overflow probe repeated: %d", got)
	}
	// At precisely limit+1 source records, a complete smaller address set changes
	// the source predicate. That second (now bounded) probe must not be skipped.
	var input bytes.Buffer
	enc := json.NewEncoder(&input)
	_ = enc.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: "probe-boundary", Edition: "probe-boundary", Title: "Invented probe boundary", Expected: 4097, SHA256: map[string]string{"invented": "invented"}})
	for i := 0; i < 4097; i++ {
		town := "Overflow Town"
		if i < 128 {
			town = "Tiny Boundary Town"
		}
		_ = enc.Encode(map[string]any{"type": "record", "source_id": fmt.Sprint(i), "record": map[string]any{"surname": "Boundaryprobe", "firstName": "June A", "town": town, "street": "Invented Road"}})
	}
	_ = enc.Encode(map[string]any{"type": "end", "count": 4097})
	if _, err = Import(ctx, db, &input); err != nil {
		t.Fatal(err)
	}
	if _, err = IndexAddresses(ctx, db, "probe-boundary", AddressIndexOptions{}); err != nil {
		t.Fatal(err)
	}
	b := NewAPI(db)
	q, err = parse(mustValues("dataset=probe-boundary&surname=Boundaryprobe&first_name=June&first_match=given&town=Tiny+Boundary+Town"))
	if err != nil {
		t.Fatal(err)
	}
	f, status, err = b.candidateFilters(ctx, q)
	if err != nil || status != 200 || !f.addressMatched {
		t.Fatalf("changed predicate failed: %d %v", status, err)
	}
	if got := b.metrics.phases[phaseKey{"source_probe", "ok"}].Count; got != 2 {
		t.Fatalf("changed source probe skipped: %d", got)
	}
	count := 0
	cursor := ""
	for i := 0; i < 10; i++ {
		w := httptest.NewRecorder()
		b.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?dataset=probe-boundary&surname=Boundaryprobe&first_name=June&first_match=given&town=Tiny+Boundary+Town&limit=17&cursor="+url.QueryEscape(cursor), nil))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var r searchResult
		if err = json.Unmarshal(w.Body.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		count += len(r.Items)
		cursor = r.Next
		if cursor == "" {
			break
		}
	}
	if count != 128 || cursor != "" {
		t.Fatalf("boundary lost complete pagination: count=%d", count)
	}
}

func testSearchPhaseRows(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	a := NewAPI(db)
	rows, err := a.searchRows(ctx, "candidates", "SELECT i FROM generate_series(1,3) i", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		t.Fatal("no rows")
	}
	rows.Close()
	rows.Close()
	if db.Stat().AcquiredConns() != 0 || a.metrics.phases[phaseKey{"candidates", "ok"}].Count != 1 {
		t.Fatal("early close leaked resources or double-counted")
	}
	rows, err = a.searchRows(ctx, "candidates", "SELECT 1/0", nil, false)
	if err == nil {
		for rows.Next() {
		}
		err = rows.Err()
		rows.Close()
	}
	if err == nil || db.Stat().AcquiredConns() != 0 || a.metrics.phases[phaseKey{"candidates", "error"}].Count != 1 {
		t.Fatal("query error leaked resources or disappeared")
	}
}
