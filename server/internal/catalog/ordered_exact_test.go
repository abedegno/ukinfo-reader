package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// A common surname can have many distinct full first names. Unused vocabulary
// and insertion order must not affect lexical pages or exhaust their work budget.
func testOrderedExactNames(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	var input bytes.Buffer
	enc := json.NewEncoder(&input)
	_ = enc.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: "given-windows", Edition: "given-windows", Title: "Invented full first names", Expected: 360, SHA256: map[string]string{"invented": "invented"}})
	for i := 359; i >= 0; i-- {
		_ = enc.Encode(map[string]any{"type": "record", "source_id": fmt.Sprint(i), "record": map[string]any{
			"surname": "Givenwindow", "firstName": fmt.Sprintf("June %04d", 2*i),
			"street": "Invented Road", "houseno": fmt.Sprint(i + 1), "postcode": "AA1 1AA",
		}})
	}
	_ = enc.Encode(map[string]any{"type": "end", "count": 360})
	if _, err := Import(ctx, db, &input); err != nil {
		t.Fatal(err)
	}
	if _, err := IndexResearch(ctx, db, "given-windows", AddressIndexOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO terms(key) SELECT 'june '||lpad((i*2+1)::text,4,'0') FROM generate_series(0,359) i ON CONFLICT DO NOTHING"); err != nil {
		t.Fatal(err)
	}
	probe := NewAPI(db)
	w := httptest.NewRecorder()
	probe.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?dataset=given-windows&surname=Givenwindow&sort=name&limit=1", nil))
	if w.Code != 200 || probe.metrics.phases[phaseKey{"vocabulary", "ok"}].Count != 2 || probe.metrics.orderedPairs.Load() != 2 {
		t.Fatal("exact surname must fetch labels in one batch before bounded pair expansion", w.Code)
	}
	for _, suffix := range []string{"sort=name&limit=31", "sort=name&first_name=June&first_match=given&limit=99", "group=listing&limit=47"} {
		a := NewAPI(db)
		rows := paradeWalk(t, a, a, "dataset=given-windows&surname=Givenwindow&"+suffix)
		if len(rows) != 360 {
			t.Fatal("incomplete exact-name traversal", len(rows))
		}
		for i, r := range rows {
			var record struct {
				First string `json:"firstName"`
			}
			if err := json.Unmarshal(r.Record, &record); err != nil {
				t.Fatal(err)
			}
			if record.First != fmt.Sprintf("June %04d", 2*i) {
				t.Fatal("numeric dictionary order or unused labels changed the lexical page", i)
			}
		}
	}
}
