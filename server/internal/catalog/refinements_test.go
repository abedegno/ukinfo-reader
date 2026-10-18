package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRefinementFields(t *testing.T) {
	raw := json.RawMessage(`{"houseno":12,"building":"Rose House","subbuilding":"Flat 1","firstName":"John A."}`)
	for _, tc := range []struct {
		q    query
		want bool
	}{
		{query{House: "12", Building: "rose*", Flat: "flat 1", Middle: "a", First: "john", FirstMatch: "given"}, true},
		{query{House: "1"}, false}, {query{House: "12a"}, false}, {query{Flat: "flat 2"}, false},
		{query{Middle: "b"}, false}, {query{First: "jo", FirstMatch: "given"}, false},
	} {
		got, err := tc.q.accepts(raw)
		if err != nil || got != tc.want {
			t.Fatal(tc, got, err)
		}
	}
	for _, raw := range []string{`{"houseno":0}`, `{"houseno":null}`, `{"houseno":true}`, `{"houseno_raw":"12"}`} {
		if got, _ := (query{House: "0*"}).accepts(json.RawMessage(raw)); got {
			t.Fatal("matched absent house", raw)
		}
	}
	for _, s := range []string{"dataset=a&house=12", "dataset=a&building=Rose", "dataset=a&surname=Li&house=1?", "dataset=a&surname=Li&middle_initial=AB", "dataset=a&surname=Li&middle_initial=1", "dataset=a&surname=Li&first_match=contains", "dataset=a&kind=business&postcode=AA11AA&middle_initial=A"} {
		if _, err := parse(mustValues(s)); err == nil {
			t.Fatal("accepted", s)
		}
	}
	for _, s := range []string{"dataset=a&postcode=AA1&house=12", "dataset=a&surname=Li&middle_initial=A.", "dataset=a&street=High&building=Rose*&flat=Flat+1", "dataset=a&surname=Li&first_name=Ann&first_match=given"} {
		if _, err := parse(mustValues(s)); err != nil {
			t.Fatal(s, err)
		}
	}
	a, _ := parse(mustValues("dataset=a&surname=Li&house=12"))
	b := a
	b.House = "14"
	if a.fingerprint() == b.fingerprint() {
		t.Fatal("refinement cursor overlap")
	}
}

func testRefinements(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	records := []map[string]any{}
	for i := 0; i < 310; i++ {
		house := "14"
		if i >= 305 {
			house = "12"
		}
		records = append(records, map[string]any{"surname": "Refine", "firstName": "John A", "houseno": house, "building": "Rose House", "subbuilding": "Flat 1", "street": "Invented Road", "town": "Invented Town", "postcode": "ZZ1 1ZZ", "large_value": json.Number("9007199254740993")})
	}
	records = append(records, map[string]any{"surname": "Refine", "firstName": "Johnny A", "houseno": "12A", "building": "Rose House", "subbuilding": "Flat 2", "street": "Invented Road", "postcode": "ZZ11ZZ"}, map[string]any{"surname": "Refine", "firstName": "John B", "houseno": 12, "building": "Rose House", "subbuilding": "Flat 1", "street": "Invented Road", "postcode": "ZZ11ZZ"}, map[string]any{"surname": "Refine", "firstName": "John", "houseno": "0", "street": "Invented Road", "postcode": "ZZ11ZZ"}, map[string]any{"entry_type": "business", "organisation": "Invented shop", "houseno": 12, "building": "Rose House", "subbuilding": "Flat 1", "street": "Invented Road", "postcode": "ZZ11ZZ"})
	importRows := func(key string, rows []map[string]any) {
		t.Helper()
		var buffer bytes.Buffer
		enc := json.NewEncoder(&buffer)
		enc.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: key, Edition: key, Title: "Invented refinements", Expected: int64(len(rows)), SHA256: map[string]string{"invented": "fake"}})
		for i, row := range rows {
			enc.Encode(map[string]any{"type": "record", "source_id": fmt.Sprint(i), "record": row})
		}
		enc.Encode(map[string]any{"type": "end", "count": len(rows)})
		if _, err := Import(context.Background(), db, &buffer); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range records {
		row["large_value"] = json.Number("9007199254740993")
	}
	importRows("refinements", records)
	importRows("refinements-other", records[305:307])
	handler := NewAPI(db).Handler()
	fetch := func(params url.Values, export bool, status int) []byte {
		t.Helper()
		path := "/api/search?"
		if export {
			path = "/api/export?"
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path+params.Encode(), nil))
		if w.Code != status {
			t.Fatalf("%s: %d %s", params, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	walk := func(s string, want int) {
		t.Helper()
		params := mustValues(s)
		params.Set("limit", "2")
		seen := map[string]bool{}
		for n := 0; ; n++ {
			if n > 10 {
				t.Fatal("endless paging")
			}
			var data searchResult
			if err := json.Unmarshal(fetch(params, false, 200), &data); err != nil {
				t.Fatal(err)
			}
			for _, item := range data.Items {
				key := item.Dataset + ":" + item.SourceID
				if seen[key] {
					t.Fatal("duplicate", key)
				}
				seen[key] = true
			}
			if len(data.Items) > 0 {
				raw := string(fetch(params, true, 200))
				if !strings.Contains(raw, "9007199254740993") {
					t.Fatal("export rounded original integer")
				}
			}
			if data.Next == "" {
				break
			}
			if n == 0 {
				changed := mustValues(params.Encode())
				changed.Set("house", "99")
				changed.Set("cursor", data.Next)
				fetch(changed, false, 400)
			}
			params.Set("cursor", data.Next)
		}
		if len(seen) != want {
			t.Fatalf("%s: got %d want %d", s, len(seen), want)
		}
	}
	// More than three full candidate batches precede the first match.
	walk("dataset=refinements&surname=Refine&house=12&building=Rose*&flat=Flat+1&middle_initial=A&first_name=John&first_match=given", 5)
	walk("dataset=all&surname=Refine&house=12&middle_initial=A&first_name=John&first_match=given", 7)
	walk("dataset=refinements&postcode=ZZ1&house=12&middle_initial=B", 1)
	walk("dataset=refinements&postcode=ZZ11ZZ&first_name=Jo&first_match=prefix&house=12*", 7)
	walk("dataset=refinements&surname=Refine&house=12*&first_name=John&first_match=given", 6)
	walk("dataset=refinements&surname=Refine&house=12*&first_name=John&first_match=prefix", 7)
	var exact searchResult
	if err := json.Unmarshal(fetch(mustValues("dataset=refinements&surname=Ref*&match=prefix&first_name=John&first_match=exact"), false, 200), &exact); err != nil || len(exact.Items) != 1 {
		t.Fatal("independent first-name exact matching", exact, err)
	}
	walk("dataset=refinements&surname=Ref*&match=prefix&first_name=John&first_match=exact&house=0", 0)
	walk("dataset=refinements&surname=Refine&first_name=Johnny+A&first_match=exact&house=12A", 1)
	walk("dataset=refinements&kind=business&postcode=ZZ11ZZ&house=12&building=Rose+House", 1)
	if _, err := IndexAddresses(context.Background(), db, "refinements", AddressIndexOptions{}); err != nil {
		t.Fatal(err)
	}
	walk("dataset=refinements&street=Invented+Road&house=12&middle_initial=A", 5)
}
