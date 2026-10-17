package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAddressParseAndProjection(t *testing.T) {
	for _, raw := range []string{"dataset=all&street=High+Street", "dataset=all&town=Lich*&county=Staffordshire", "dataset=all&kind=business&locality=Village"} {
		if _, e := parse(mustValues(raw)); e != nil {
			t.Fatal(raw, e)
		}
	}
	for _, value := range []string{"*", "a*", "road?", "a*b", "%", "a\x00", strings.Repeat("a", 257)} {
		if _, e := parse(url.Values{"dataset": {"all"}, "street": {value}}); e == nil {
			t.Fatal("accepted address", value)
		}
	}
	l, e := projectAddress(map[string]any{"street_1": "  OLD  Road ", "street_2": "High Street", "locality_1": "Village", "town": "Town", "county": "County", "street_raw": "Hidden", "building_raw": "Hidden"})
	if e != nil || l != (addressLocation{"old road", "high street", "village", "", "town", "county"}) {
		t.Fatal(l, e)
	}
	a, _ := parse(mustValues("dataset=all&street=High+Street"))
	b, _ := parse(mustValues("dataset=all&street=Other+Street"))
	if a.fingerprint() == b.fingerprint() {
		t.Fatal("address absent from fingerprint")
	}
}

func testAddresses(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	const key = "invented-addresses"
	var input bytes.Buffer
	enc := json.NewEncoder(&input)
	_ = enc.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: key, Edition: "invented-address", Title: "Invented address fixture", SHA256: map[string]string{"fake": "fake"}, Expected: 260})
	// Cross block boundaries, repeated locations, first/second street components,
	// mixed listing types, and source-only fields which must not be searchable.
	for i := 1; i <= 260; i++ {
		r := map[string]any{"surname": "Address", "firstName": "June", "street": "High Street", "town": "Invented Town", "county": "Old County", "postcode": "AA11AA", "house": fmt.Sprint(i)}
		if i%2 == 0 {
			delete(r, "street")
			r["street_2"] = "High Street"
			r["locality_1"] = "Small Village"
		}
		if i == 259 {
			r["entry_type"] = "business"
			r["organisation"] = "Address Shop"
		}
		if i == 260 {
			delete(r, "street")
			delete(r, "street_2")
			r["street_raw"] = "Hidden Road"
			r["town"] = nil
			r["town_raw"] = "Hidden Town"
		}
		_ = enc.Encode(map[string]any{"type": "record", "source_id": fmt.Sprint(i), "record": r})
	}
	_ = enc.Encode(map[string]any{"type": "end", "count": 260})
	stream := input.String()
	if _, e := Import(ctx, db, strings.NewReader(stream)); e != nil {
		t.Fatal(e)
	}
	handler := NewAPI(db).Handler()
	fetch := func(query string, status int) []byte {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", query, nil))
		if w.Code != status {
			t.Fatalf("%s: %d %s", query, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	fetch("/api/search?dataset="+key+"&street=High+Street", 503)
	pilot, e := IndexAddresses(ctx, db, key, AddressIndexOptions{SampleBlocks: 2})
	if e != nil || !pilot.Pilot || pilot.Records != 256 {
		t.Fatal("pilot", pilot, e)
	}
	var n int
	if e = db.QueryRow(ctx, "SELECT count(*) FROM address_indexes").Scan(&n); e != nil || n != 0 {
		t.Fatal("pilot published", n, e)
	}
	sentinel := errors.New("disk reserve")
	_, e = IndexAddresses(ctx, db, key, AddressIndexOptions{CheckResources: func() error { return sentinel }})
	if !errors.Is(e, sentinel) {
		t.Fatal("resource failure", e)
	}
	var lowDisk atomic.Bool
	var dictionaryBefore, dictionaryAfter int
	if e = db.QueryRow(ctx, "SELECT count(*) FROM address_locations").Scan(&dictionaryBefore); e != nil {
		t.Fatal(e)
	}
	_, e = IndexAddresses(ctx, db, key, AddressIndexOptions{
		Progress: func(p Progress) {
			if p.Phase == "grouping address postings" {
				lowDisk.Store(true)
			}
		},
		CheckResources: func() error {
			if lowDisk.Load() {
				return sentinel
			}
			return nil
		},
	})
	if !errors.Is(e, sentinel) {
		t.Fatal("disk guard during build", e)
	}
	if e = db.QueryRow(ctx, "SELECT count(*) FROM address_locations").Scan(&dictionaryAfter); e != nil || dictionaryAfter != dictionaryBefore {
		t.Fatal("failed build changed dictionary", dictionaryBefore, dictionaryAfter, e)
	}
	fetch("/api/search?dataset="+key+"&street=High+Street", 503)
	report, e := IndexAddresses(ctx, db, key, AddressIndexOptions{})
	if e != nil || report.Records != 260 || report.Locations != 3 {
		t.Fatal("complete", report, e)
	}
	if _, e = IndexAddresses(ctx, db, key, AddressIndexOptions{}); e == nil {
		t.Fatal("replaced published address index")
	}
	// Pilot over an already indexed edition must not change its dictionary or memberships.
	_, e = IndexAddresses(ctx, db, key, AddressIndexOptions{SampleBlocks: 1})
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Contains(fetch("/api/addresses", 200), []byte(`"dataset":"`+key+`"`)) {
		t.Fatal("coverage missing")
	}
	type response struct {
		Items []result `json:"items"`
		Next  string   `json:"next_cursor"`
	}
	cursor := ""
	seen := map[string]bool{}
	for page := 0; ; page++ {
		if page > 20 {
			t.Fatal("endless address paging")
		}
		var r response
		if e = json.Unmarshal(fetch("/api/search?dataset="+key+"&street=High+Street&town=Invented+Town&limit=25&cursor="+cursor, 200), &r); e != nil {
			t.Fatal(e)
		}
		for _, item := range r.Items {
			if seen[item.SourceID] {
				t.Fatal("duplicate", item.SourceID)
			}
			seen[item.SourceID] = true
			var record map[string]any
			_ = json.Unmarshal(item.Record, &record)
			l, e := projectAddress(record)
			if e != nil || (l[0] != "high street" && l[1] != "high street") || l[4] != "invented town" {
				t.Fatal("address mismatch", item, e)
			}
		}
		if cursor == "" && r.Next != "" {
			fetch("/api/search?dataset="+key+"&street=Other&cursor="+r.Next, 400)
		}
		cursor = r.Next
		if cursor == "" {
			break
		}
	}
	if len(seen) != 258 {
		t.Fatal("address paging lost records", len(seen))
	}
	for _, tc := range []struct {
		query string
		count int
	}{
		{"street=High*&surname=Addr*&match=prefix", 258},
		{"street=High*&near=AA11AA&radius_m=500", 258},
		{"kind=business&street=High+Street", 1},
		{"locality=Small+Village", 130},
		{"county=Old+County&postcode=AA1", 259},
		{"town=Hidden+Town", 0}, {"street=Hidden+Road", 0},
		{"street=" + url.QueryEscape("High' OR true --"), 0},
	} {
		// Count all pages, not just an arbitrary first page.
		count := 0
		cursor := ""
		for {
			var r response
			_ = json.Unmarshal(fetch("/api/search?dataset="+key+"&limit=100&"+tc.query+"&cursor="+cursor, 200), &r)
			count += len(r.Items)
			cursor = r.Next
			if cursor == "" {
				break
			}
		}
		if count != tc.count {
			t.Fatalf("%s: got %d want %d", tc.query, count, tc.count)
		}
	}
	fetch("/api/search?dataset=all&street=High+Street", 503) // never silently skip unindexed editions
	// All-edition address paging must include equal occurrence IDs from multiple
	// editions, while excluding a redundant sample of the same source edition.
	other := strings.ReplaceAll(stream, key, "invented-addresses-copy")
	other = strings.ReplaceAll(other, `"edition":"invented-address"`, `"edition":"invented-address-copy"`)
	if _, e = Import(ctx, db, strings.NewReader(other)); e != nil {
		t.Fatal(e)
	}
	rows, e := db.Query(ctx, "SELECT d.key FROM datasets d LEFT JOIN address_indexes i ON i.dataset_id=d.id WHERE i.dataset_id IS NULL ORDER BY d.id")
	if e != nil {
		t.Fatal(e)
	}
	keys := []string{}
	for rows.Next() {
		var k string
		if e = rows.Scan(&k); e != nil {
			t.Fatal(e)
		}
		keys = append(keys, k)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		t.Fatal(e)
	}
	for _, k := range keys {
		if _, e = IndexAddresses(ctx, db, k, AddressIndexOptions{}); e != nil {
			t.Fatal(k, e)
		}
	}
	allSeen := map[string]bool{}
	cursor = ""
	for page := 0; ; page++ {
		if page > 10 {
			t.Fatal("endless multi-edition address paging")
		}
		var r response
		_ = json.Unmarshal(fetch("/api/search?dataset=all&street=High+Street&limit=100&cursor="+cursor, 200), &r)
		for _, item := range r.Items {
			identity := item.Dataset + ":" + item.SourceID
			if allSeen[identity] {
				t.Fatal("duplicate across editions", identity)
			}
			allSeen[identity] = true
		}
		cursor = r.Next
		if cursor == "" {
			break
		}
	}
	if len(allSeen) != 516 {
		t.Fatal("all-edition address count", len(allSeen))
	}
	// Optional post-import indexing can fail after a valid source commit. The
	// caller receives a nonzero committed count and can finish indexing separately.
	committed, e := ImportWithOptions(ctx, db, strings.NewReader(fixture("address-index-failure", true)), ImportOptions{IndexAddresses: true, CheckResources: func() error { return sentinel }})
	if committed != 5 || !errors.Is(e, sentinel) {
		t.Fatal("post-commit index failure", committed, e)
	}
	if verified, e := Verify(ctx, db, "address-index-failure", nil); e != nil || verified.Entries != 5 {
		t.Fatal("source commit lost", verified, e)
	}
	if _, e = IndexAddresses(ctx, db, "address-index-failure", AddressIndexOptions{}); e != nil {
		t.Fatal("index recovery", e)
	}
	exported := fetch("/api/export?dataset="+key+"&street=High+Street&limit=1", 200)
	if !bytes.Contains(exported, []byte(`"street":"High Street"`)) {
		t.Fatal("export changed source address")
	}
}
