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

func TestPremisesIdentity(t *testing.T) {
	base := map[string]any{"houseno": "12A", "street": "High Street", "postcode": "AA1 1AA", "town": "Town", "building": "Rose House", "subbuilding": "Flat 1"}
	first, reason := premises(base)
	if reason != "" {
		t.Fatal(reason)
	}
	variants := []map[string]any{
		{"houseno": "12A", "street_2": " HIGH  STREET ", "postcode": "AA11AA", "town": "town", "building": "rose house", "subbuilding": "flat 1"},
	}
	for _, row := range variants {
		key, reason := premises(row)
		if reason != "" || key.key() != first.key() {
			t.Fatal("equivalent display parts", key, reason)
		}
	}
	for _, field := range []string{"houseno", "building", "subbuilding", "town", "postcode"} {
		row := map[string]any{}
		for k, v := range base {
			row[k] = v
		}
		row[field] = "different"
		key, _ := premises(row)
		if key.key() == first.key() {
			t.Fatal("merged distinct premises", field)
		}
	}
	for _, row := range []map[string]any{
		{"houseno": "0", "street": "High Street", "postcode": "AA11AA"},
		{"houseno": json.Number("0.0"), "street": "High Street", "postcode": "AA11AA"},
		{"houseno": true, "building": "Rose House", "street": "High Street", "postcode": "AA11AA"},
		{"houseno": "12", "street": "High Street", "postcode": "AA11AA", "source_warning": []string{"unknown"}},
		{"houseno": "12", "street": "High Street", "postcode": "AA1", "postcode_raw": "AA11AA"},
		{"houseno": "12", "street": "High Street", "postcode": "AA11AA", "postcode_precision": 7},
		{"houseno": "12", "street": "High Street", "postcode": "AA11AA", "source_warning": "building_reference_inside_string", "building_raw": "Hidden"},
	} {
		if _, reason := premises(row); reason == "" {
			t.Fatal("accepted ambiguous premises", row)
		}
	}
	numeric, _ := premises(map[string]any{"houseno": json.Number("12"), "street": "High Street", "postcode": "AA11AA"})
	text, _ := premises(map[string]any{"houseno": "12", "street": "High Street", "postcode": "AA11AA"})
	if numeric.key() != text.key() {
		t.Fatal("numeric house mismatch")
	}
}
func testPremises(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	rows := []map[string]any{}
	for i := 0; i < 300; i++ {
		rows = append(rows, map[string]any{"surname": "Neighbour", "firstName": fmt.Sprintf("Person %03d", i), "houseno": "12", "street": "Invented Street", "subbuilding": "Flat 1", "town": "Invented Town", "postcode": "AA1 1AA"})
	}
	rows[0]["large_source_value"] = json.Number("9007199254740993")
	// Distinct premises, unresolved buildings and raw postcode values never join.
	rows = append(rows, map[string]any{"surname": "Other flat", "houseno": "12", "street": "Invented Street", "subbuilding": "Flat 2", "town": "Invented Town", "postcode": "AA11AA"}, map[string]any{"surname": "Whole building", "houseno": "12", "street": "Invented Street", "town": "Invented Town", "postcode": "AA11AA"}, map[string]any{"surname": "Other house", "houseno": "14", "street": "Invented Street", "subbuilding": "Flat 1", "town": "Invented Town", "postcode": "AA11AA"}, map[string]any{"surname": "Raw only", "houseno": "12", "street": "Invented Street", "subbuilding": "Flat 1", "town": "Invented Town", "postcode": "AA1", "postcode_raw": "AA11AA"}, map[string]any{"surname": "Warning", "houseno": "12", "street": "Invented Street", "subbuilding": "Flat 1", "town": "Invented Town", "postcode": "AA11AA", "source_warning": "building_reference_inside_string"})
	// Business co-listings are retained alongside electoral/telephone occurrences.
	rows[299]["entry_type"] = "business"
	rows[299]["organisation"] = "Invented shop"
	importRows := func(key, edition string, sample bool, records []map[string]any) {
		t.Helper()
		var buffer bytes.Buffer
		enc := json.NewEncoder(&buffer)
		_ = enc.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: key, Edition: edition, Title: edition, Sample: sample, SHA256: map[string]string{"invented": "fake"}, Expected: int64(len(records))})
		for i, r := range records {
			_ = enc.Encode(map[string]any{"type": "record", "source_id": fmt.Sprint(i), "record": r})
		}
		_ = enc.Encode(map[string]any{"type": "end", "count": len(records)})
		if _, err := Import(ctx, db, &buffer); err != nil {
			t.Fatal(err)
		}
	}
	importRows("premises-2005", "invented-2005-06", false, rows[:3])
	importRows("premises-1999", "invented-1999-04", false, rows)
	importRows("premises-sample", "invented-2005-06", true, rows[:2])
	handler := NewAPI(db).Handler()
	fetch := func(path string, status int) []byte {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != status {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	for _, query := range []string{"dataset=all&id=1", "dataset=premises-1999&id=0", "dataset=premises-1999&id=2147483648", "dataset=premises-1999&id=1&format=invalid"} {
		fetch("/api/record?"+query, 400)
	}
	fetch("/api/record?dataset=absent&id=1", 404)
	fetch("/api/record?dataset=premises-1999&id=99999", 404)
	fetch("/api/record?dataset=premises-1999&id=1&source_id=changed", 409)
	var record struct {
		Item      result `json:"item"`
		Available bool   `json:"address_available"`
		Reason    string `json:"address_reason"`
	}
	if err := json.Unmarshal(fetch("/api/record?dataset=premises-1999&id=1&source_id=0", 200), &record); err != nil || !record.Available || record.Item.SourceID != "0" {
		t.Fatal(record, err)
	}
	if err := json.Unmarshal(fetch("/api/record?dataset=premises-1999&id=304", 200), &record); err != nil || record.Available || record.Reason == "" {
		t.Fatal("partial postcode", record, err)
	}
	fetch("/api/address?dataset=premises-1999&id=304", 422)
	fetch("/api/address?dataset=premises-1999&id=305", 422)
	fetch("/api/address?dataset=premises-1999&id=1&scope=unknown", 400)
	fetch("/api/address?dataset=premises-1999&id=1&limit=1000", 400)
	type response struct {
		Items  []result `json:"items"`
		Next   string   `json:"next_cursor"`
		Anchor result   `json:"anchor"`
	}
	for _, tc := range []struct {
		scope string
		count int
	}{{"edition", 300}, {"all", 303}} {
		seen := map[string]bool{}
		cursor := ""
		business := false
		for page := 0; ; page++ {
			if page > 20 {
				t.Fatal("endless address paging")
			}
			var data response
			if err := json.Unmarshal(fetch("/api/address?dataset=premises-1999&id=1&source_id=0&scope="+tc.scope+"&limit=25&cursor="+url.QueryEscape(cursor), 200), &data); err != nil {
				t.Fatal(err)
			}
			for _, item := range data.Items {
				identity := item.Dataset + ":" + item.SourceID
				if seen[identity] {
					t.Fatal("duplicate", identity)
				}
				if tc.scope == "all" && len(seen) == 0 && item.Dataset != "premises-1999" {
					t.Fatal("edition chronology does not match dates")
				}
				seen[identity] = true
				if item.Dataset == "premises-sample" {
					t.Fatal("redundant sample")
				}
				var row map[string]any
				_ = json.Unmarshal(item.Record, &row)
				if row["subbuilding"] != "Flat 1" {
					t.Fatal("flat merged")
				}
				if row["entry_type"] == "business" {
					business = true
				}
			}
			cursor = data.Next
			if cursor == "" {
				break
			}
			if page == 0 {
				fetch("/api/address?dataset=premises-1999&id=1&scope="+tc.scope+"&cursor="+url.QueryEscape(cursor), 400)
			} // source guard is part of cursor
		}
		if len(seen) != tc.count || !business {
			t.Fatal("premises scope", tc, len(seen), business)
		}
	}
	for _, endpoint := range []string{"/api/record?dataset=premises-1999&id=1&format=jsonl", "/api/address?dataset=premises-1999&id=1&limit=1&format=jsonl"} {
		body := fetch(endpoint, 200)
		if !bytes.Contains(body, []byte("9007199254740993")) {
			t.Fatal("source numeric value changed in export")
		}
	}
	var first response
	_ = json.Unmarshal(fetch("/api/address?dataset=premises-1999&id=1&source_id=0&limit=1", 200), &first)
	fetch("/api/address?dataset=premises-1999&id=1&source_id=0&scope=all&cursor="+url.QueryEscape(first.Next), 400)
	fetch("/api/address?dataset=premises-1999&id=301&source_id=300&cursor="+url.QueryEscape(first.Next), 400)
	// The first two postcode batches can contain no matching premises. Continue
	// scanning rather than treating a filtered first batch as the complete result.
	gaps := make([]map[string]any, 0, 515)
	for i := 0; i < 515; i++ {
		house := "14"
		if i >= 512 {
			house = "12"
		}
		gaps = append(gaps, map[string]any{"surname": "Gap", "houseno": house, "subbuilding": "Flat 1", "street": "Invented Street", "town": "Invented Town", "postcode": "AA11AA"})
	}
	importRows("premises-gaps", "invented-2026-10", false, gaps)
	gapCursor := ""
	gapCount := 0
	for page := 0; ; page++ {
		if page > 3 {
			t.Fatal("gap paging never ended")
		}
		var data response
		_ = json.Unmarshal(fetch("/api/address?dataset=premises-gaps&id=513&limit=1&cursor="+url.QueryEscape(gapCursor), 200), &data)
		gapCount += len(data.Items)
		gapCursor = data.Next
		if gapCursor == "" {
			break
		}
	}
	if gapCount != 3 {
		t.Fatal("postcode batches skipped late matches", gapCount)
	}
	// Same-edition sample exploration remains explicitly scoped to that sample.
	var sample response
	_ = json.Unmarshal(fetch("/api/address?dataset=premises-sample&id=1", 200), &sample)
	if len(sample.Items) != 2 {
		t.Fatal("sample scope", len(sample.Items))
	}
	// These endpoints need the base postcode projection, not the optional address index.
	if _, err := db.Exec(ctx, "DELETE FROM address_indexes WHERE dataset_id IN (SELECT id FROM datasets WHERE key LIKE 'premises-%')"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(fetch("/api/address?dataset=premises-1999&id=1&limit=1", 200)), `"next_cursor"`) {
		t.Fatal("missing page")
	}
}
