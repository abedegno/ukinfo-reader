package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Invented data only. The unused dictionary tail forces the broad-prefix path;
// the Amy candidate makes a materialized membership scan exhaust that tail.
func firstPostcodeFixture(t *testing.T) (*pgxpool.Pool, *pgxpool.Config) {
	t.Helper()
	dsn := os.Getenv("UKINFO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("ukinfo_first_postcode_%d", os.Getpid())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); admin.Close(ctx) })
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err = Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	var stream bytes.Buffer
	enc := json.NewEncoder(&stream)
	enc.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: "first-postcode", Edition: "invented", Title: "Invented broad names", SHA256: map[string]string{"fixture": "invented"}, Expected: 10})
	for i, first := range []string{"J", "J A", "J B", "Jane", "John", "June", "Amy", "J A", "J A", "J A"} {
		surname, postcode := "Invented", "ZZ9 1AA"
		if i == 7 {
			postcode = "ZZ99 1AA"
		}
		if i == 8 {
			surname = "Other"
		}
		if i == 9 {
			postcode = "ZZ9 1AB"
		}
		enc.Encode(map[string]any{"type": "record", "source_id": fmt.Sprintf("person-%d", i), "record": map[string]any{"surname": surname, "firstName": first, "postcode": postcode}})
	}
	enc.Encode(map[string]any{"type": "end", "count": 10})
	if _, err = Import(ctx, db, &stream); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO terms(key) SELECT 'jaaa'||lpad(i::text,6,'0') FROM generate_series(1,200000) i; ANALYZE;`); err != nil {
		t.Fatal(err)
	}
	return db, cfg
}

// Regression: scanning the global first-name prefix costs work proportional to
// the dictionary even when surname+postcode leave only a handful of candidates.
func TestBroadFirstPostcodeDictionaryWorkIntegration(t *testing.T) {
	_, cfg := firstPostcodeFixture(t)
	ctx := context.Background()
	for _, mode := range []string{"force_custom_plan", "force_generic_plan"} {
		t.Run(mode, func(t *testing.T) {
			c := cfg.Copy()
			c.ConnConfig.RuntimeParams["plan_cache_mode"] = mode
			db, err := pgxpool.NewWithConfig(ctx, c)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			api := NewAPI(db)
			defer api.decoder.Close()
			q, err := parse(mustValues("dataset=first-postcode&kind=person&match=exact&surname=Invented&first_name=J&first_match=given&postcode=ZZ9"))
			if err != nil {
				t.Fatal(err)
			}
			f, status, err := api.buildCandidateFiltersWith(ctx, q, false)
			if err != nil || status != 200 {
				t.Fatal(status, err)
			}
			prefix := ""
			if len(f.termCTEs) > 0 {
				prefix = "WITH " + strings.Join(f.termCTEs, ",") + " "
			}
			sql := "EXPLAIN (ANALYZE,FORMAT JSON) " + prefix + "SELECT dataset_id,id FROM entries WHERE " + strings.Join(f.conditions, " AND ") + " LIMIT 51"
			var raw []byte
			if err = db.QueryRow(ctx, sql, f.args...).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var plan []struct{ Plan map[string]any }
			if err = json.Unmarshal(raw, &plan); err != nil {
				t.Fatal(err)
			}
			var examined float64
			var visit func(map[string]any)
			visit = func(n map[string]any) {
				if n["Relation Name"] == "terms" {
					rows, _ := n["Actual Rows"].(float64)
					removed, _ := n["Rows Removed by Filter"].(float64)
					loops, _ := n["Actual Loops"].(float64)
					examined += (rows + removed) * loops
				}
				if children, ok := n["Plans"].([]any); ok {
					for _, child := range children {
						visit(child.(map[string]any))
					}
				}
			}
			visit(plan[0].Plan)
			if examined > 32 {
				t.Fatalf("selective surname/postcode search examined %.0f dictionary rows; want candidate-bounded work <=32", examined)
			}
		})
	}
}

// Literal expected identities catch broad-prefix truncation, given-name boundary
// mistakes, postcode precision errors and continuation omissions/duplicates.
func TestBroadFirstPostcodePaginationIntegration(t *testing.T) {
	db, _ := firstPostcodeFixture(t)
	api := NewAPI(db)
	defer api.decoder.Close()
	for _, tc := range []struct {
		extra string
		want  []string
	}{
		{"first_name=J&first_match=given", []string{"person-0", "person-1", "person-2", "person-9"}},
		{"first_name=J&first_match=given&middle_initial=A", []string{"person-1", "person-9"}},
		{"first_name=J&first_match=prefix", []string{"person-0", "person-1", "person-2", "person-3", "person-4", "person-5", "person-9"}},
		{"first_name=J*&first_match=given", []string{"person-0", "person-1", "person-2", "person-3", "person-4", "person-5", "person-9"}},
		{"first_name=J&first_match=exact", []string{"person-0"}},
		{"first_name=J&first_match=given&postcode=ZZ9+1AA", []string{"person-0", "person-1", "person-2"}},
	} {
		for _, order := range []string{"", "name", "edition"} {
			for _, limit := range []int{1, 2, 50} {
				t.Run(fmt.Sprintf("%s/%s/%d", tc.extra, order, limit), func(t *testing.T) {
					v, _ := url.ParseQuery("dataset=first-postcode&kind=person&match=exact&surname=Invented&postcode=ZZ9&" + tc.extra)
					// ParseQuery appends duplicate keys; choose the explicit full postcode.
					if p := v["postcode"]; len(p) > 1 {
						v.Set("postcode", p[len(p)-1])
					}
					v.Set("sort", order)
					v.Set("limit", fmt.Sprint(limit))
					got := []string{}
					seen := map[string]bool{}
					for page := 0; page < 20; page++ {
						w := httptest.NewRecorder()
						api.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?"+v.Encode(), nil))
						var out searchResult
						if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
							t.Fatalf("search status=%d body=%s", w.Code, w.Body.String())
						}
						for _, item := range out.Items {
							if seen[item.SourceID] {
								t.Fatal("duplicate record across pages", item.SourceID)
							}
							seen[item.SourceID] = true
							got = append(got, item.SourceID)
						}
						if out.Next == "" {
							break
						}
						if len(out.Items) == 0 {
							t.Fatal("empty continuing page")
						}
						v.Set("cursor", out.Next)
						if page == 19 {
							t.Fatal("pagination did not terminate")
						}
					}
					sort.Strings(got)
					if !reflect.DeepEqual(got, tc.want) {
						t.Fatalf("identities=%v want=%v", got, tc.want)
					}
				})
			}
		}
	}
}
