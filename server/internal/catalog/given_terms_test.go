package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type givenVocabularyTrace struct {
	sql  string
	args []any
}

func (g *givenVocabularyTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if len(d.Args) > 0 && d.Args[0] == "j" && strings.Contains(d.SQL, "8193") {
		g.sql = d.SQL
		g.args = append([]any(nil), d.Args...)
	}
	return ctx
}
func (*givenVocabularyTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// The vocabulary probe must not expand unrelated first words (Jane, John and
// the invented jaaa tail) just to find J and J followed by middle names.
func TestGivenNameVocabularyWorkIntegration(t *testing.T) {
	_, cfg := firstPostcodeFixture(t)
	ctx := context.Background()
	for _, mode := range []string{"force_custom_plan", "force_generic_plan"} {
		t.Run(mode, func(t *testing.T) {
			trace := &givenVocabularyTrace{}
			c := cfg.Copy()
			c.MaxConns = 1
			c.ConnConfig.Tracer = trace
			c.ConnConfig.RuntimeParams["plan_cache_mode"] = mode
			db, err := pgxpool.NewWithConfig(ctx, c)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			a := NewAPI(db)
			defer a.decoder.Close()
			q, err := parse(mustValues("dataset=first-postcode&kind=person&match=exact&surname=Invented&first_name=J&first_match=given&postcode=ZZ9"))
			if err != nil {
				t.Fatal(err)
			}
			if _, status, err := a.buildCandidateFiltersWith(ctx, q, false); err != nil || status != 200 {
				t.Fatal(status, err)
			}
			if trace.sql == "" {
				t.Fatal("given-name vocabulary query was not executed")
			}
			sql, args := trace.sql, trace.args
			var raw []byte
			if err = db.QueryRow(ctx, "EXPLAIN (ANALYZE,FORMAT JSON) "+sql, args...).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var plans []struct{ Plan map[string]any }
			if err = json.Unmarshal(raw, &plans); err != nil {
				t.Fatal(err)
			}
			examined := 0.0
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
			visit(plans[0].Plan)
			if examined > 32 {
				t.Fatalf("given-name vocabulary examined %.0f dictionary rows; want word-boundary work <=32", examined)
			}
		})
	}
}

// More than the vocabulary probe's limit of valid middle-name forms must not
// hide a later matching name, including when record-level middle refinement runs.
func TestGivenNameBroadTailPaginationIntegration(t *testing.T) {
	db, _ := firstPostcodeFixture(t)
	if _, err := db.Exec(context.Background(), `INSERT INTO terms(key) SELECT 'j aaaa'||lpad(i::text,6,'0') FROM generate_series(1,9000) i; ANALYZE;`); err != nil {
		t.Fatal(err)
	}
	a := NewAPI(db)
	defer a.decoder.Close()
	for _, tc := range []struct {
		input string
		want  []string
	}{
		{"first_name=J&first_match=given", []string{"person-0", "person-1", "person-2", "person-9"}},
		{"first_name=J&first_match=given&middle_initial=B", []string{"person-2"}},
		{"first_name=J+A&first_match=given", []string{"person-1", "person-9"}},
		{"first_name=J*&first_match=given", []string{"person-0", "person-1", "person-2", "person-3", "person-4", "person-5", "person-9"}},
	} {
		for _, order := range []string{"", "name", "edition"} {
			t.Run(tc.input+"/"+order, func(t *testing.T) {
				v, _ := url.ParseQuery("dataset=first-postcode&kind=person&match=exact&surname=Invented&postcode=ZZ9&limit=1&" + tc.input)
				v.Set("sort", order)
				got := []string{}
				seen := map[string]bool{}
				for page := 0; page < 20; page++ {
					w := httptest.NewRecorder()
					a.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?"+v.Encode(), nil))
					var out searchResult
					if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
						t.Fatalf("broad-tail search status=%d", w.Code)
					}
					for _, r := range out.Items {
						if seen[r.SourceID] {
							t.Fatal("duplicate continuation record")
						}
						seen[r.SourceID] = true
						got = append(got, r.SourceID)
					}
					if out.Next == "" {
						break
					}
					if len(out.Items) == 0 || page == 19 {
						t.Fatal("invalid continuing page")
					}
					v.Set("cursor", out.Next)
				}
				sort.Strings(got)
				if !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("identities=%v want=%v", got, tc.want)
				}
			})
		}
	}
}

// A space-prefix range includes every valid UTF-8 suffix, including the largest
// code point. Appending a sentinel rune as an exclusive upper bound loses it.
func TestGivenNameUnicodeBoundariesIntegration(t *testing.T) {
	db, _ := firstPostcodeFixture(t)
	var stream bytes.Buffer
	enc := json.NewEncoder(&stream)
	enc.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: "unicode-given", Edition: "invented", Title: "Invented Unicode boundaries", SHA256: map[string]string{"fixture": "invented"}, Expected: 2})
	for _, r := range []struct{ id, first string }{{"j-unicode", "J \U0010ffff"}, {"ja-unicode", "J A \U0010ffff"}} {
		enc.Encode(map[string]any{"type": "record", "source_id": r.id, "record": map[string]any{"surname": "Invented", "firstName": r.first, "postcode": "ZZ9 1AA"}})
	}
	enc.Encode(map[string]any{"type": "end", "count": 2})
	if _, err := Import(context.Background(), db, &stream); err != nil {
		t.Fatal(err)
	}
	a := NewAPI(db)
	defer a.decoder.Close()
	for _, tc := range []struct {
		first string
		want  []string
	}{{"J", []string{"j-unicode", "ja-unicode"}}, {"J A", []string{"ja-unicode"}}} {
		v := url.Values{"dataset": {"unicode-given"}, "surname": {"Invented"}, "first_name": {tc.first}, "first_match": {"given"}, "match": {"exact"}, "postcode": {"ZZ9"}}
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?"+v.Encode(), nil))
		var out searchResult
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
			t.Fatal("Unicode boundary request failed", w.Code)
		}
		got := []string{}
		for _, r := range out.Items {
			got = append(got, r.SourceID)
		}
		sort.Strings(got)
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("Unicode boundary identities=%v want=%v", got, tc.want)
		}
	}
}
