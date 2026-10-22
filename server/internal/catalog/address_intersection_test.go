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
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testReverseIntersectionPlans(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	var dataset int16
	var name int32
	if err := db.QueryRow(ctx, "SELECT id FROM datasets WHERE key='large-address'").Scan(&dataset); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, "SELECT id FROM terms WHERE key='widebudget'").Scan(&name); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"address_record_blocks", "address_locations", fmt.Sprintf("entries_d%d", dataset)} {
		if _, err := db.Exec(ctx, "VACUUM(ANALYZE) "+table); err != nil {
			t.Fatal(err)
		}
	}
	// Both probes overflow. Source pages stay bounded and selective even under
	// a generic plan; full result pagination is checked by the surrounding fixture.
	f := candidateFilters{datasetIDs: []int16{dataset}, args: []any{[]int16{dataset}, int16(0), []int32{name}}, conditions: []string{"dataset_id=ANY($1::smallint[])", "kind=$2", "name_id=ANY($3::integer[])"}}
	query, _ := addressSourceBatchSQL(f, addressCandidate{})
	observations := []map[string]any{}
	conn, err := db.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	// Give both cached-plan modes realistic name cardinality and visibility.
	// These invented SQL-only projections exist only inside this disposable fixture.
	// Remove them before record/pagination checks; no dictionary or records needed.
	defer func() {
		if _, e := conn.Exec(ctx, fmt.Sprintf("DELETE FROM entries WHERE dataset_id=%d AND id>22000", dataset)); e != nil {
			t.Error(e)
		}
		if _, e := conn.Exec(ctx, fmt.Sprintf("VACUUM(ANALYZE) entries_d%d", dataset)); e != nil {
			t.Error(e)
		}
	}()
	if _, err = conn.Exec(ctx, fmt.Sprintf("INSERT INTO entries SELECT 22000+i,%d,0,1000000+i,0,0 FROM generate_series(1,100000)i", dataset)); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, fmt.Sprintf("VACUUM(ANALYZE) entries_d%d", dataset)); err != nil {
		t.Fatal(err)
	}

	for _, mode := range []string{"force_custom_plan", "force_generic_plan"} {
		func() {
			if _, err = conn.Exec(ctx, "SET plan_cache_mode="+mode); err != nil {
				t.Fatal(err)
			}
			defer conn.Exec(ctx, "RESET plan_cache_mode")
			if _, err = conn.Exec(ctx, "PREPARE reverse_work(smallint[],smallint,integer[],integer,smallint,smallint) AS SELECT id FROM ("+query+") chosen ORDER BY id"); err != nil {
				t.Fatal(err)
			}
			defer conn.Exec(ctx, "DEALLOCATE reverse_work")
			execute := fmt.Sprintf("EXECUTE reverse_work(ARRAY[%d]::smallint[],0,ARRAY[%d],0,0,%d)", dataset, name, dataset)
			rows, err := conn.Query(ctx, execute)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for rows.Next() {
				var id int
				if err = rows.Scan(&id); err != nil {
					t.Fatal(err)
				}
				count++
				if id != count {
					t.Fatal("source identity/order changed")
				}
			}
			err = rows.Err()
			rows.Close()
			if err != nil || count != addressIntersectionBatch {
				t.Fatal("intersection incomplete", count, err)
			}
			var raw []byte
			if err = conn.QueryRow(ctx, "EXPLAIN(ANALYZE,BUFFERS,TIMING OFF,FORMAT JSON) "+execute).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var plans []map[string]any
			if err = json.Unmarshal(raw, &plans); err != nil {
				t.Fatal(err)
			}
			root := plans[0]["Plan"].(map[string]any)
			buffers := root["Shared Hit Blocks"].(float64) + root["Shared Read Blocks"].(float64)
			if buffers >= 1000 {
				t.Fatal("reverse intersection exceeded buffer budget", buffers)
			}
			scans := 0
			sourceAccess := ""
			heapBlocks := float64(0)
			var walk func(map[string]any)
			walk = func(n map[string]any) {
				if n["Relation Name"] == "address_members" || n["Relation Name"] == "address_record_blocks" {
					t.Fatal("source page must not expand address tables")
				}
				if n["Relation Name"] == fmt.Sprintf("entries_d%d", dataset) {
					scans++
					sourceAccess, _ = n["Node Type"].(string)
					switch sourceAccess {
					case "Index Only Scan":
						if n["Heap Fetches"] != float64(0) {
							t.Fatal("source page fetched heaps through covering index", mode)
						}
					case "Bitmap Heap Scan":
						heapBlocks, _ = n["Exact Heap Blocks"].(float64)
						if heapBlocks > 64 || n["Lossy Heap Blocks"] != float64(0) {
							t.Fatal("source bitmap exceeded selective heap budget", mode, heapBlocks)
						}
					default:
						t.Fatal("source page must use selective index access", mode, sourceAccess)
					}
				}
				if children, ok := n["Plans"].([]any); ok {
					for _, child := range children {
						walk(child.(map[string]any))
					}
				}
			}
			walk(root)
			if scans != 1 {
				t.Fatal("expected one pruned source partition", scans)
			}

			observations = append(observations, map[string]any{"mode": mode, "source_rows": addressIntersectionBatch, "lookup_blocks_upper_bound": addressIntersectionBatch, "shared_blocks": buffers, "source_access": sourceAccess, "heap_blocks": heapBlocks, "execution_ms": plans[0]["Execution Time"]})
		}()
	}
	if dir := os.Getenv("UKINFO_PERFORMANCE_REPORT_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		raw, err := json.MarshalIndent(map[string]any{"conditions": "Invented cached fixture; not HDD capacity", "plans": observations}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "reverse-address-plans.json"), append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

// Separate fixture: both business-classification and town probes overflow, and
// the address predicate has a distant matching range after an empty source prefix.
func testBusinessAddressIntersection(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	var input bytes.Buffer
	enc := json.NewEncoder(&input)
	_ = enc.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: "business-address-work", Edition: "business-address-work", Title: "Invented business intersection", Expected: 5000, SHA256: map[string]string{"invented": "invented"}})
	for i := 0; i < 5000; i++ {
		town := "Business Town"
		if i >= 2500 && i < 3750 {
			town = "Other Business Town"
		}
		industry, sic := "Invented widgets", "1234"
		if i < 100 {
			industry, sic = "Other widgets", "9999"
		}
		_ = enc.Encode(map[string]any{"type": "record", "source_id": fmt.Sprint(i), "record": map[string]any{"entry_type": "business", "organisation": "Wide widgets", "industry": industry, "sic_code": sic, "town": town, "street": "Invented Road", "houseno": fmt.Sprint(i)}})
	}
	_ = enc.Encode(map[string]any{"type": "end", "count": 5000})
	if _, err := Import(ctx, db, &input); err != nil {
		t.Fatal(err)
	}
	if _, err := IndexAddresses(ctx, db, "business-address-work", AddressIndexOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := IndexResearch(ctx, db, "business-address-work", AddressIndexOptions{}); err != nil {
		t.Fatal(err)
	}
	a := NewAPI(db)
	a.ExperimentalAddressProbe = true
	// Normal serving keeps the experiment off. A sparse query must have exactly
	// the same body/cursor in both modes; only the opt-in mode adds a source probe.
	var baselineBody []byte
	for _, enabled := range []bool{false, true} {
		api := NewAPI(db)
		api.ExperimentalAddressProbe = enabled
		w := httptest.NewRecorder()
		api.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?dataset=business-address-work&kind=business&organisation=Wide*&industry=invented*&sic=12*&town=Other+Business+Town&limit=100", nil))
		if w.Code != 200 {
			t.Fatal("probe mode failed", w.Code)
		}
		if !enabled {
			baselineBody = append([]byte(nil), w.Body.Bytes()...)
		} else if !bytes.Equal(baselineBody, w.Body.Bytes()) {
			t.Fatal("probe changed sparse results or cursor")
		}
		want := uint64(1)
		if enabled {
			want = 2
		}
		if got := api.metrics.phases[phaseKey{"source_probe", "ok"}].Count; got != want {
			t.Fatal("unexpected probe default", enabled, got, want)
		}
	}
	q, err := parse(mustValues("dataset=business-address-work&kind=business&organisation=Wide*&industry=invented*&sic=12*&town=Other+Business+Town"))
	if err != nil {
		t.Fatal(err)
	}
	f, status, err := a.candidateFilters(ctx, q)
	if err != nil || status != 200 || f.addressMatched {
		t.Fatal("expected two overflowing probes", err)
	}
	if _, proven, err := a.reverseAddressPage(ctx, f, 101); err != nil || proven {
		t.Fatal("sparse prefix must retain the complete fallback", err)
	}
	for _, tc := range []struct {
		town, order string
		count       int
		other       bool
	}{{"Business+Town", "", 3650, false}, {"Business+Town", "&sort=name", 3650, false}, {"Business+Town", "&group=listing", 3650, false}, {"Other+Business+Town", "", 1250, true}} {
		cursor := ""
		seen := map[string]bool{}
		for page := 0; page < 42; page++ {
			w := httptest.NewRecorder()
			a.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?dataset=business-address-work&kind=business&organisation=Wide*&industry=invented*&sic=12*&town="+tc.town+"&limit=100"+tc.order+"&cursor="+url.QueryEscape(cursor), nil))
			if w.Code != 200 {
				t.Fatal("business intersection failed", w.Code, w.Body.String())
			}
			var r searchResult
			if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
				t.Fatal(err)
			}
			for _, item := range r.Items {
				id, err := strconv.Atoi(item.SourceID)
				if err != nil || id < 100 || ((id >= 2500 && id < 3750) != tc.other) || seen[item.SourceID] {
					t.Fatal("wrong/repeated business occurrence")
				}
				seen[item.SourceID] = true
			}
			cursor = r.Next
			if cursor == "" {
				break
			}
		}
		if len(seen) != tc.count || cursor != "" {
			t.Fatal("incomplete business pagination", len(seen))
		}
	}
	testPositiveProjectionBenchmark(t, db)
	testBusinessSearch(t, db)
}
