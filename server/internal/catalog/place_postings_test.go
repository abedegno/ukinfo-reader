package catalog

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testPlacePostings(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	// This schema exists only in the disposable integration database. Build from
	// the independent forward street index, rather than the candidate's reverse
	// lookup, so the expected HTTP results do not share the new access path.
	_, err := db.Exec(ctx, `CREATE SCHEMA ukinfo_place_spike;
 CREATE TABLE ukinfo_place_spike.build AS SELECT id AS dataset_id,entries AS expected,8192 AS chunk_rows,now() AS finished_at FROM datasets;
 CREATE TABLE ukinfo_place_spike.postings(dataset_id smallint,field smallint,key text COLLATE "C",chunk integer,ids integer[],PRIMARY KEY(dataset_id,field,key,chunk));
 INSERT INTO ukinfo_place_spike.postings
 SELECT dataset_id,field,f.key,(entry_id-1)/8192,array_agg(entry_id ORDER BY entry_id)
 FROM address_members m JOIN address_locations l ON l.id=m.location_id CROSS JOIN LATERAL unnest(m.ids) entry_id
 CROSS JOIN LATERAL(VALUES(1,l.town),(2,l.county)) f(field,key) WHERE f.key<>''
 GROUP BY dataset_id,field,f.key,(entry_id-1)/8192;
 ANALYZE ukinfo_place_spike.postings;`)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, "DROP SCHEMA ukinfo_place_spike CASCADE")
	before, after := NewAPI(db), NewAPI(db)
	defer before.decoder.Close()
	defer after.decoder.Close()
	after.ExperimentalPlacePostings = true
	for _, query := range []string{
		"dataset=large-address&surname=Widebudget&town=Large+Town",
		"dataset=large-address&surname=Scatteredbudget&town=Large*",
		"dataset=all&town=Invented*&county=Old*",
		"dataset=all&town=Other+Town&county=Missing",
		"dataset=all&town=Other+Town&county=Old+County&kind=business",
		"dataset=all&county=Old*&kind=business",
		"dataset=all&town=Invented*&first_name=June&first_match=given",
		"dataset=all&town=Invented*&street=Invented*",
		"dataset=all&county=Old*&sort=name",
		"dataset=all&town=Invented*&group=listing",
		"dataset=large-address&town=Large+Town&postcode=CC1",
		"dataset=all&town=Missing",
	} {
		t.Run(query, func(t *testing.T) {
			cursor := ""
			for page := 0; page < 4; page++ {
				values, parseErr := url.ParseQuery(query)
				if parseErr != nil {
					t.Fatal(parseErr)
				}
				if values.Get("kind") == "" {
					values.Set("kind", "person")
				}
				values.Set("match", "exact")
				values.Set("limit", "7")
				path := "/api/search?" + values.Encode() + cursor
				b, c := httptest.NewRecorder(), httptest.NewRecorder()
				before.Handler().ServeHTTP(b, httptest.NewRequest("GET", path, nil))
				after.Handler().ServeHTTP(c, httptest.NewRequest("GET", path, nil))
				if b.Code != 200 || c.Code != 200 || b.Body.String() != c.Body.String() {
					t.Fatalf("page %d before=%d after=%d bodies_equal=%v", page, b.Code, c.Code, b.Body.String() == c.Body.String())
				}
				var body struct {
					Next string `json:"next_cursor"`
				}
				if err := json.Unmarshal(b.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body.Next == "" {
					break
				}
				cursor = "&cursor=" + url.QueryEscape(body.Next)
			}
		})
	}
	if before.metrics.plans["place_postings"] != 0 || after.metrics.plans["place_postings"] == 0 {
		t.Fatal("private posting path was not exercised")
	}
	// An unfinished edition must retain the complete legacy path, even when the
	// private flag is on. Mixed-edition searches cannot silently omit a catalogue.
	if _, err = db.Exec(ctx, "UPDATE ukinfo_place_spike.build SET finished_at=NULL WHERE dataset_id=(SELECT min(dataset_id) FROM ukinfo_place_spike.build)"); err != nil {
		t.Fatal(err)
	}
	f := candidateFilters{q: query{Town: "invented town"}}
	rows, err := db.Query(ctx, "SELECT id FROM datasets ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var d int16
		if err = rows.Scan(&d); err != nil {
			t.Fatal(err)
		}
		f.datasetIDs = append(f.datasetIDs, d)
	}
	rows.Close()
	ready, err := after.placePostingsReady(ctx, f)
	if err != nil || ready {
		t.Fatalf("unfinished edition ready=%v err=%v", ready, err)
	}
}
