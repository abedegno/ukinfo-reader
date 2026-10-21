package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Exercise the actual API predicate against a wide classification dictionary.
// These are invented SQL-only projections; no source-record reads are involved.
func testClassificationProbeWork(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	var dataset int16
	if err := db.QueryRow(ctx, `INSERT INTO datasets(key,manifest,entries) VALUES('classification-work','{}',50000) RETURNING id`).Scan(&dataset); err != nil {
		t.Fatal(err)
	}
	entryTable := fmt.Sprintf("entries_d%d", dataset)
	researchTable := fmt.Sprintf("research_d%d", dataset)
	sql := fmt.Sprintf(`CREATE TABLE %s PARTITION OF entries FOR VALUES IN (%d);
 CREATE TABLE %s PARTITION OF research_keys FOR VALUES IN (%d);
 INSERT INTO terms(key) VALUES('ZZ99ZZ') ON CONFLICT DO NOTHING;
 INSERT INTO entries SELECT i,%d,1,0,0,(SELECT id FROM terms WHERE key=CASE WHEN i<=50 THEN 'AA11AA' ELSE 'ZZ99ZZ' END) FROM generate_series(1,50000)i;
 INSERT INTO research_keys SELECT %d,i,decode(repeat('00',32),'hex'),sha256(convert_to('invented group '||i,'UTF8')),'','wide invented industry '||(i%%1000) FROM generate_series(1,50000)i;
 INSERT INTO research_indexes(dataset_id,version,entries) VALUES(%d,1,50000);
 CREATE INDEX classification_work_cover ON %s(dataset_id,id) INCLUDE(industry,sic) WHERE industry<>'' OR sic<>'';`, entryTable, dataset, researchTable, dataset, dataset, dataset, dataset, researchTable)
	if _, err := db.Exec(ctx, sql); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{entryTable, researchTable, "terms"} {
		if _, err := db.Exec(ctx, "VACUUM(ANALYZE) "+table); err != nil {
			t.Fatal(err)
		}
	}
	a := NewAPI(db)
	q, err := parse(mustValues("dataset=classification-work&kind=business&postcode=AA1+1AA&industry=wide+invented+industry*"))
	if err != nil {
		t.Fatal(err)
	}
	f, status, err := a.buildCandidateFilters(ctx, q)
	if err != nil || status != 200 || f.missing {
		t.Fatal("classification filters", status, err)
	}
	query := "SELECT id FROM entries WHERE " + strings.Join(f.conditions, " AND ") + " ORDER BY id LIMIT 51"
	if len(f.args) != 5 {
		t.Fatal("unexpected fixture bindings")
	}
	postcode := f.args[2].([]int32)[0]
	observations := []map[string]any{}
	for _, mode := range []string{"force_custom_plan", "force_generic_plan"} {
		conn, err := db.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		func() {
			defer conn.Release()
			if _, err = conn.Exec(ctx, "SET plan_cache_mode="+mode); err != nil {
				t.Fatal(err)
			}
			defer conn.Exec(ctx, "RESET plan_cache_mode")
			if _, err = conn.Exec(ctx, "PREPARE classification_work(smallint[],smallint,integer[],text,text) AS "+query); err != nil {
				t.Fatal(err)
			}
			defer conn.Exec(ctx, "DEALLOCATE classification_work")
			execute := fmt.Sprintf("EXECUTE classification_work(ARRAY[%d]::smallint[],1,ARRAY[%d],'wide invented industry','wide invented industry'||chr(1114111))", dataset, postcode)
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
					t.Fatal("classified source order changed")
				}
			}
			err = rows.Err()
			rows.Close()
			if err != nil || count != 50 {
				t.Fatal("classified results", count, err)
			}
			var raw []byte
			if err = conn.QueryRow(ctx, "EXPLAIN(ANALYZE,BUFFERS,FORMAT JSON) "+execute).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var plans []map[string]any
			if err = json.Unmarshal(raw, &plans); err != nil {
				t.Fatal(err)
			}
			root := plans[0]["Plan"].(map[string]any)
			blocks := root["Shared Hit Blocks"].(float64) + root["Shared Read Blocks"].(float64)
			if blocks >= 800 {
				t.Fatalf("classification work budget exceeded: %g blocks", blocks)
			}
			scans := 0
			idCondition := regexp.MustCompile(`\bid\s*=`)
			var walk func(map[string]any)
			walk = func(n map[string]any) {
				if n["Relation Name"] == researchTable {
					scans++
					condition, _ := n["Index Cond"].(string)
					filter, _ := n["Filter"].(string)
					if n["Node Type"] != "Index Only Scan" || n["Heap Fetches"] != float64(0) || !idCondition.MatchString(condition) || strings.Contains(condition, "industry") || strings.Contains(condition, "sic") || strings.Contains(filter, "industry") || strings.Contains(filter, "sic") {
						t.Fatal("classification probe must seek a covered source ID before testing its classification", mode)
					}
				}
				if children, ok := n["Plans"].([]any); ok {
					for _, child := range children {
						walk(child.(map[string]any))
					}
				}
			}
			walk(root)
			observations = append(observations, map[string]any{"mode": mode, "execution_ms": plans[0]["Execution Time"], "shared_blocks": blocks, "results": count, "heap_fetches": 0})
			if scans != 1 {
				t.Fatal("expected one pruned classification partition", scans)
			}
		}()
	}
	if dir := os.Getenv("UKINFO_PERFORMANCE_REPORT_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		data, err := json.MarshalIndent(map[string]any{"fixture_entries": 50000, "conditions": "Cached invented PostgreSQL fixture; not HDD capacity", "plans": observations}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "classification-plans.json"), append(data, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
	testGroupRepresentativePlans(t, db, dataset)

}
