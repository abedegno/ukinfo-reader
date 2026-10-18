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

func researchFixture(key, edition string, reverse bool) string {
	var output bytes.Buffer
	enc := json.NewEncoder(&output)
	enc.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: key, Edition: edition, Title: "Invented research " + edition, SHA256: map[string]string{"invented.data": strings.Repeat("a", 64)}, Expected: 8})
	records := []map[string]any{}
	for i := 0; i < 6; i++ {
		records = append(records, map[string]any{"entry_id": fmt.Sprint(i), "surname": "Researchonly", "firstName": fmt.Sprintf("Name %d", i), "postcode": "AA1 1AA", "street": "Invented Street", "houseno": "1"})
	}
	records[4]["postcode"] = "BB1 1BB"
	records[5] = map[string]any{"entry_id": "5", "surname": "Researchonly", "firstName": "Name 0", "postcode": "AA1 1AA", "street": "Invented Street", "houseno": "1"}
	records = append(records, map[string]any{"entry_id": "business", "entry_type": "business", "organisation": "Researchonly business", "postcode": "AA1 1AA", "industry": "Invented widgets", "sic_code": "1234", "street": "Invented Street", "houseno": "1"}, map[string]any{"entry_id": "warning", "surname": "Researchonly", "firstName": "Name 0", "postcode": "AA1 1AA", "street": "Invented Street", "houseno": "1", "source_warning": "ambiguous_address"})
	if reverse {
		for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
			records[i], records[j] = records[j], records[i]
		}
	}
	for _, record := range records {
		enc.Encode(map[string]any{"type": "record", "source_id": record["entry_id"], "record": record})
	}
	enc.Encode(map[string]any{"type": "end", "count": len(records)})
	return output.String()
}
func testResearchIndex(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	for _, d := range []struct {
		key, edition string
		reverse      bool
	}{{"research-old", "research-1999-04", false}, {"research-new", "research-2005-06", true}} {
		if _, err := Import(ctx, db, strings.NewReader(researchFixture(d.key, d.edition, d.reverse))); err != nil {
			t.Fatal(err)
		}
		report, err := IndexResearch(ctx, db, d.key, AddressIndexOptions{})
		if err != nil || report.Records != 8 || report.Businesses != 1 {
			t.Fatal(report, err)
		}
	}
	for _, key := range []string{"research-old", "research-new"} {
		if _, err := IndexAddresses(ctx, db, key, AddressIndexOptions{}); err != nil {
			t.Fatal(err)
		}
	}

	api := NewAPI(db)
	get := func(path string, status int) map[string]json.RawMessage {
		t.Helper()
		response := httptest.NewRecorder()
		api.Handler().ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != status {
			t.Fatalf("%s: %d %s", path, response.Code, response.Body.String())
		}
		var body map[string]json.RawMessage
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	base := "/api/search?dataset=all&surname=Researchonly&group=listing&limit=2"
	// Other test editions have no research index; selecting all must refuse to imply
	// complete grouping until every source with person listings is indexed.
	get(base, 503)
	// Complete the remaining small synthetic sources so all-edition grouping can
	// demonstrate completeness without changing sample-selection semantics.
	rows, err := db.Query(ctx, "SELECT key FROM datasets WHERE id NOT IN (SELECT dataset_id FROM research_indexes)")
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	for rows.Next() {
		var key string
		rows.Scan(&key)
		keys = append(keys, key)
	}
	rows.Close()
	for _, key := range keys {
		if _, err := IndexResearch(ctx, db, key, AddressIndexOptions{}); err != nil {
			t.Fatal(key, err)
		}
	}
	seen := map[string]bool{}
	total := 0
	groupKey := ""
	for path := base; ; {
		body := get(path, 200)
		var items []result
		var next string
		json.Unmarshal(body["items"], &items)
		json.Unmarshal(body["next_cursor"], &next)
		for _, item := range items {
			if seen[item.Group] {
				t.Fatal("group reappeared on another page")
			}
			seen[item.Group] = true
			total++
			if item.Occurrences == 4 {
				groupKey = item.Group
			}
		}
		if next == "" {
			break
		}
		path = base + "&cursor=" + url.QueryEscape(next)
	}
	// Five complete distinct listing values plus two warning occurrences.
	if total != 7 || groupKey == "" {
		t.Fatal("group count", total, groupKey)
	}
	body := get("/api/group?dataset=all&group_key="+groupKey, 200)
	var members []result
	json.Unmarshal(body["items"], &members)
	if len(members) != 4 {
		t.Fatal("source occurrences", len(members))
	}

	for _, mode := range []string{"name", "edition", "nearest"} {
		base := "/api/search?dataset=all&surname=Researchonly&sort=" + mode + "&limit=2"
		if mode == "nearest" {
			base += "&near=AA11AA&radius_m=1000"
		}
		seen := map[string]bool{}
		previousName, previousEdition := "", ""
		previousDistance := -1.0
		count := 0
		for path := base; ; {
			body := get(path, 200)
			var items []result
			var next string
			json.Unmarshal(body["items"], &items)
			json.Unmarshal(body["next_cursor"], &next)
			for _, item := range items {
				identity := item.Dataset + ":" + item.SourceID
				if seen[identity] {
					t.Fatal("ordered occurrence repeated", mode)
				}
				seen[identity] = true
				count++
				var row map[string]any
				json.Unmarshal(item.Record, &row)
				name := Normalize(Text(row, "firstName"))
				if mode == "name" && name < previousName {
					t.Fatal("name order crossed a page boundary incorrectly")
				}
				previousName = name
				if mode == "edition" {
					if previousEdition != "" && item.Dataset < previousEdition {
						t.Fatal("edition order crossed a page boundary incorrectly")
					}
					previousEdition = item.Dataset
				}
				if mode == "nearest" {
					if item.Distance == nil || *item.Distance < previousDistance {
						t.Fatal("distance order crossed a page boundary incorrectly")
					}
					previousDistance = *item.Distance
				}
			}
			if next == "" {
				break
			}
			path = base + "&cursor=" + url.QueryEscape(next)
		}
		if count != 14 || (mode == "nearest" && previousDistance <= 0) {
			t.Fatal("ordered results omitted occurrences", mode, count, previousDistance)
		}
	}

	body = get("/api/search?dataset=all&kind=business&industry=invented*&sic=1234", 200)
	var businesses []result
	json.Unmarshal(body["items"], &businesses)
	if len(businesses) != 2 {
		t.Fatal("business classification", len(businesses))
	}
	for _, extra := range []string{"&street=Invented+Street&sort=name", "&first_name=Name&first_match=given&house=1"} {
		query := "/api/search?dataset=research-old&surname=Researchonly&group=listing&limit=2" + extra
		count := 0
		for path := query; ; {
			body := get(path, 200)
			var items []result
			var next string
			json.Unmarshal(body["items"], &items)
			json.Unmarshal(body["next_cursor"], &next)
			count += len(items)
			if next == "" {
				break
			}
			path = query + "&cursor=" + url.QueryEscape(next)
		}
		if count != 6 {
			t.Fatal("group address/refinement pagination", count)
		}
	}

	old := get("/api/record?dataset=research-old&id=1&source_id=0", 200)
	var wrapped struct {
		Item result `json:"item"`
	}
	json.Unmarshal(mustJSON(old), &wrapped)
	archive := wrapped.Item.Archive
	if len(archive) != 64 {
		t.Fatal("missing archival identity")
	}
	// Same original source, reversed ingestion order and changed dataset key. The
	// stale ID hint must resolve by original source identity, never open another row.
	resolved := get("/api/record?dataset=renamed-or-sample&id=1&source_id=0&archive_id="+archive, 200)
	json.Unmarshal(mustJSON(resolved), &wrapped)
	if wrapped.Item.SourceID != "0" || wrapped.Item.Archive != archive {
		t.Fatal("archival resolution", wrapped)
	}
	get("/api/record?dataset=research-new&id=1&source_id=not-present&archive_id="+archive, 404)
	var id int16
	db.QueryRow(ctx, "SELECT id FROM datasets WHERE key='research-new'").Scan(&id)
	if err := RemoveDataset(ctx, db, "research-new", id); err != nil {
		t.Fatal(err)
	}
	testResearchPairWindows(t, db, get)
}
func mustJSON(value any) []byte { raw, _ := json.Marshal(value); return raw }
func TestListingKeyContract(t *testing.T) {
	base := map[string]any{"surname": "Invented", "firstName": "Ann", "street": "Made Up Street", "houseno": "1", "postcode": "AA1 1AA"}
	key := func(row map[string]any, dataset, source string) []byte {
		k, e := listingKey(row, "original-components", source, dataset)
		if e != nil {
			t.Fatal(e)
		}
		return k
	}
	first := key(base, "old", "a")
	same := map[string]any{}
	for name, value := range base {
		same[name] = value
	}
	same["surname"] = "  INVENTED "
	same["street"] = "made  up street"
	if !bytes.Equal(first, key(same, "new", "b")) {
		t.Fatal("canonical complete listings failed to group")
	}
	for _, change := range []map[string]any{{"firstName": ""}, {"street": ""}, {"houseno": "0"}, {"postcode_precision": json.Number("12")}, {"source_warning": "ambiguous_address"}, {"subbuilding": map[string]any{"bad": "type"}}} {
		row := map[string]any{}
		for name, value := range base {
			row[name] = value
		}
		for name, value := range change {
			row[name] = value
		}
		if bytes.Equal(key(row, "one", "a"), key(row, "two", "b")) {
			t.Fatal("unsafe/incomplete occurrences were grouped", change)
		}
	}
	other := map[string]any{}
	for name, value := range base {
		other[name] = value
	}
	other["entry_type"] = "telephone"
	if bytes.Equal(first, key(other, "new", "b")) {
		t.Fatal("distinct listing types grouped")
	}
}
func TestSearchFieldErrors(t *testing.T) {
	api := NewAPI(nil)
	for _, tc := range []struct{ query, field string }{{"dataset=demo&surname=S*", "surname"}, {"dataset=demo&postcode=invalid", "postcode"}, {"dataset=demo&surname=Invented&middle_initial=AB", "middle_initial"}, {"dataset=demo&street=I*", "street"}} {
		response := httptest.NewRecorder()
		api.Handler().ServeHTTP(response, httptest.NewRequest("GET", "/api/search?"+tc.query, nil))
		var body map[string]string
		json.Unmarshal(response.Body.Bytes(), &body)
		if response.Code != 400 || body["field"] != tc.field {
			t.Fatal(tc, response.Code, body)
		}
	}
}

func testResearchPairWindows(t *testing.T, db *pgxpool.Pool, get func(string, int) map[string]json.RawMessage) {
	t.Helper()
	ctx := context.Background()
	var stream bytes.Buffer
	encoder := json.NewEncoder(&stream)
	encoder.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: "research-window", Edition: "invented-2005-06", Title: "Invented pair windows", SHA256: map[string]string{"invented": strings.Repeat("b", 64)}, Expected: 360})
	for i := 0; i < 360; i++ {
		name := i
		if i >= 300 {
			name = 0
		}
		encoder.Encode(map[string]any{"type": "record", "source_id": fmt.Sprint(i), "record": map[string]any{"entry_id": fmt.Sprint(i), "surname": "Pairwindowonly", "firstName": fmt.Sprintf("Name%03d", name), "street": "Invented Street", "houseno": "1", "postcode": "AA11AA"}})
	}
	encoder.Encode(map[string]any{"type": "end", "count": 360})
	if _, err := Import(ctx, db, &stream); err != nil {
		t.Fatal(err)
	}
	if _, err := IndexResearch(ctx, db, "research-window", AddressIndexOptions{}); err != nil {
		t.Fatal(err)
	}
	query := "/api/search?dataset=research-window&surname=Pairwindowonly&group=listing&limit=100"
	seen := map[string]bool{}
	group := ""
	for path := query; ; {
		body := get(path, 200)
		var items []result
		var next string
		json.Unmarshal(body["items"], &items)
		json.Unmarshal(body["next_cursor"], &next)
		for _, item := range items {
			if seen[item.Group] {
				t.Fatal("group repeated across pair window")
			}
			seen[item.Group] = true
			if item.Occurrences == 61 {
				group = item.Group
			}
		}
		if next == "" {
			break
		}
		path = query + "&cursor=" + url.QueryEscape(next)
	}
	if len(seen) != 300 || group == "" {
		t.Fatal("pair window truncated", len(seen))
	}
	sourceIDs := map[string]bool{}
	query = "/api/group?dataset=research-window&group_key=" + group
	for path := query; ; {
		body := get(path, 200)
		var items []result
		var next string
		json.Unmarshal(body["items"], &items)
		json.Unmarshal(body["next_cursor"], &next)
		for _, item := range items {
			if sourceIDs[item.SourceID] {
				t.Fatal("source repeated across pages")
			}
			sourceIDs[item.SourceID] = true
		}
		if next == "" {
			break
		}
		path = query + "&cursor=" + url.QueryEscape(next)
	}
	if len(sourceIDs) != 61 {
		t.Fatal("group source paging omitted occurrences", len(sourceIDs))
	}
}
