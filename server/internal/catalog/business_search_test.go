package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func testBusinessSearch(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	rows, err := db.Query(ctx, "SELECT d.key FROM datasets d JOIN address_indexes a ON a.dataset_id=d.id AND a.entries=d.entries JOIN research_indexes r ON r.dataset_id=d.id AND r.entries=d.entries ORDER BY d.id")
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	for rows.Next() {
		var k string
		if err = rows.Scan(&k); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, k)
	}
	rows.Close()
	for _, k := range keys {
		if _, err = IndexBusinessSearch(ctx, db, k, nil); err != nil {
			t.Fatal(k, err)
		}
	}
	if _, err = IndexBusinessSearch(ctx, db, keys[0], nil); err != nil {
		t.Fatal("idempotent build", err)
	}
	before, after := NewAPI(db), NewAPI(db)
	before.DisableBusinessSearch = true
	defer before.decoder.Close()
	defer after.decoder.Close()
	for _, raw := range []string{
		"sic=9999", "sic=12*&town=Other+Business+Town", "industry=Other+widgets", "sic=missing", "industry=Other*&county=Old*",
		"sic=12*&street=Business*", "sic=12*&postcode=BB1", "sic=12*&organisation=Business*", "sic=12*&building=missing",
		"sic=12*&sort=name", "sic=12*&group=listing", "industry=Other*&town=Other*",
	} {
		t.Run("business-projection/"+raw, func(t *testing.T) {
			values, _ := url.ParseQuery(raw)
			values.Set("kind", "business")
			values.Set("dataset", "all")
			values.Set("limit", "23")
			cursor := ""
			for page := 0; page < 4; page++ {
				values.Set("cursor", cursor)
				b, c := httptest.NewRecorder(), httptest.NewRecorder()
				path := "/api/search?" + values.Encode()
				before.Handler().ServeHTTP(b, httptest.NewRequest("GET", path, nil))
				after.Handler().ServeHTTP(c, httptest.NewRequest("GET", path, nil))
				if b.Code != 200 || c.Code != 200 || b.Body.String() != c.Body.String() {
					t.Fatalf("page %d status=%d/%d same=%v", page, b.Code, c.Code, b.Body.String() == c.Body.String())
				}
				var response struct {
					Next string `json:"next_cursor"`
				}
				if err = json.Unmarshal(c.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				cursor = response.Next
				if cursor == "" {
					break
				}
			}
		})
	}
	if after.metrics.plans["business_projection"] == 0 || before.metrics.plans["business_projection"] != 0 {
		t.Fatal("business path not exercised")
	}
	var sourceCount, indexCount int64
	if err = db.QueryRow(ctx, "SELECT count(*) FROM entries WHERE kind=1").Scan(&sourceCount); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, "SELECT sum(businesses) FROM business_search_indexes").Scan(&indexCount); err != nil || indexCount != sourceCount {
		t.Fatal("incomplete business coverage", err)
	}
	testBusinessSearchPlans(t, db, after)
	// Unfinished metadata on just one business-bearing edition must retain the
	// complete legacy path for a mixed-edition search.
	if _, err = db.Exec(ctx, "UPDATE business_search_indexes SET entries=entries+1 WHERE businesses>0"); err != nil {
		t.Fatal(err)
	}
	q, err := parse(mustValues("dataset=all&kind=business&sic=12*"))
	if err != nil {
		t.Fatal(err)
	}
	f, status, err := after.buildCandidateFilters(ctx, q)
	if err != nil || status != 200 || f.businessSearch {
		t.Fatal("mixed-edition fallback", err)
	}
	if _, err = db.Exec(ctx, "UPDATE business_search_indexes SET entries=entries-1 WHERE businesses>0"); err != nil {
		t.Fatal(err)
	}
	var detached int16
	if err = db.QueryRow(ctx, "SELECT dataset_id FROM business_search_indexes WHERE businesses>0 ORDER BY dataset_id LIMIT 1").Scan(&detached); err != nil {
		t.Fatal(err)
	}
	partition := pgx.Identifier{fmt.Sprintf("business_d%d", detached)}.Sanitize()
	if _, err = db.Exec(ctx, "ALTER TABLE business_search DETACH PARTITION "+partition); err != nil {
		t.Fatal(err)
	}
	f, status, err = after.buildCandidateFilters(ctx, q)
	if err != nil || status != 200 || f.businessSearch {
		t.Fatal("detached partition must fall back", err)
	}
	if _, err = db.Exec(ctx, fmt.Sprintf("ALTER TABLE business_search ATTACH PARTITION %s FOR VALUES IN (%d)", partition, detached)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "DELETE FROM business_search_indexes"); err != nil {
		t.Fatal(err)
	}
	// Do not leave added fixtures affecting the remaining matrix.
	for _, k := range keys {
		var id int16
		if err = db.QueryRow(ctx, "SELECT id FROM datasets WHERE key=$1", k).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, "DROP TABLE "+pgx.Identifier{fmt.Sprintf("business_d%d", id)}.Sanitize()); err != nil {
			t.Fatal(err)
		}
	}
	var damaged int16
	if err = db.QueryRow(ctx, "SELECT id FROM datasets WHERE key='business-address-work'").Scan(&damaged); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `CREATE TABLE missing_business_key AS SELECT * FROM research_keys WHERE dataset_id=$1 AND id=1`, damaged); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "DELETE FROM research_keys WHERE dataset_id=$1 AND id=1", damaged); err != nil {
		t.Fatal(err)
	}
	if _, err = IndexBusinessSearch(ctx, db, "business-address-work", nil); err == nil {
		t.Fatal("missing research row accepted")
	}
	var rolledBack bool
	if err = db.QueryRow(ctx, "SELECT to_regclass($1) IS NULL AND NOT EXISTS(SELECT FROM business_search_indexes WHERE dataset_id=$2)", fmt.Sprintf("business_d%d", damaged), damaged).Scan(&rolledBack); err != nil || !rolledBack {
		t.Fatal("failed build was not atomic", err)
	}
	if _, err = db.Exec(ctx, "INSERT INTO research_keys SELECT * FROM missing_business_key; DROP TABLE missing_business_key"); err != nil {
		t.Fatal(err)
	}

}

func testBusinessSearchPlans(t *testing.T, db *pgxpool.Pool, a *API) {
	t.Helper()
	ctx := context.Background()
	q, err := parse(mustValues("dataset=business-address-work&kind=business&sic=9999"))
	if err != nil {
		t.Fatal(err)
	}
	f, status, err := a.buildCandidateFilters(ctx, q)
	if err != nil || status != 200 || !f.businessSearch {
		t.Fatal("projection plan not selected", err)
	}
	args := append([]any(nil), f.args...)
	args = append(args, 51, f.datasetIDs[0])
	ctes := append([]string(nil), f.termCTEs...)
	branch := businessSearchBranch(f, f.datasetIDs[0], len(args), len(args)-1, f.conditions, &args, &ctes, "name_id,first_id,id,dataset_id")
	sql := "WITH " + strings.Join(ctes, ",") + " SELECT * FROM " + branch + " matches ORDER BY name_id,first_id,id,dataset_id"
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
			if _, err = conn.Exec(ctx, "PREPARE business_work(smallint[],smallint,text,integer,smallint) AS "+sql); err != nil {
				t.Fatal(err)
			}
			defer conn.Exec(ctx, "DEALLOCATE business_work")
			execute := fmt.Sprintf("EXECUTE business_work(ARRAY[%d]::smallint[],1,'9999',51,%d)", f.datasetIDs[0], f.datasetIDs[0])
			var raw []byte
			if err = conn.QueryRow(ctx, "EXPLAIN(ANALYZE,BUFFERS,FORMAT JSON) "+execute).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var plans []map[string]any
			if err = json.Unmarshal(raw, &plans); err != nil {
				t.Fatal(err)
			}
			root := plans[0]["Plan"].(map[string]any)
			if root["Actual Rows"].(float64) != 51 || root["Shared Hit Blocks"].(float64)+root["Shared Read Blocks"].(float64) > 1000 {
				t.Fatal("projection work budget exceeded", mode)
			}
			scans := 0
			var walk func(map[string]any)
			walk = func(n map[string]any) {
				if name, ok := n["Relation Name"].(string); ok && strings.HasPrefix(name, "business_d") {
					scans++
					if n["Node Type"] != "Index Only Scan" || n["Heap Fetches"] != float64(0) {
						t.Fatal("business projection needs covering index-only reads", mode, n["Node Type"])
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
				t.Fatal("expected exactly one projection partition", mode, scans)
			}
		}()
	}
}
