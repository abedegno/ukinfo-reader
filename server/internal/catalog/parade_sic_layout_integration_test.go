package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func testParadeSICLayoutRouting(t *testing.T, native, main *API, records []map[string]any) {
	t.Helper()
	ctx := context.Background()
	target := main.ParadeDB
	editions, err := searchEditions(ctx, native.DB, "street-prefix")
	if err != nil || len(editions) != 1 {
		t.Fatal("fixture edition", err)
	}
	d := editions[0]
	hybrid := NewAPI(native.DB)
	hybrid.ParadeDB = target
	hybrid.ExperimentalSICUnpartitioned = true
	const metric = "paradedb_sic_unpartitioned"
	query := "dataset=street-prefix&kind=business&sic=12*&town=Invented+Town&sort=name&limit=2"
	fetch := func(a *API, query string) (int, []byte) {
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?"+query, nil))
		return w.Code, w.Body.Bytes()
	}
	check := func(query string, wantRoute bool) {
		t.Helper()
		_, want := fetch(main, query)
		before := hybrid.metrics.plans[metric]
		status, got := fetch(hybrid, query)
		if status != 200 || !bytes.Equal(got, want) {
			t.Fatal("optional layout response/cursor differs")
		}
		if (hybrid.metrics.plans[metric] > before) != wantRoute {
			t.Fatal("unexpected layout decision")
		}
	}
	check(query, false) // no registry: existing installations need no new schema
	if _, err = target.Exec(ctx, "CREATE TABLE ukinfo_search.sic_editions (LIKE ukinfo_search.editions INCLUDING ALL)"); err != nil {
		t.Fatal(err)
	}
	check(query, false) // empty registry
	reset := func() {
		t.Helper()
		if _, e := target.Exec(ctx, "TRUNCATE ukinfo_search.sic_editions; INSERT INTO ukinfo_search.sic_editions SELECT * FROM ukinfo_search.editions"); e != nil {
			t.Fatal(e)
		}
	}
	for _, change := range []string{"ready=false", "last_id=0", "entries=entries+1", "version=2", "generation=repeat('0',64)"} {
		reset()
		if _, err = target.Exec(ctx, "UPDATE ukinfo_search.sic_editions SET "+change); err != nil {
			t.Fatal(err)
		}
		check(query, false)
	}
	reset()
	check(query, false) // complete registry but missing alias
	alias := pgx.Identifier{"ukinfo_search", d.table() + "_sic"}.Sanitize()
	base := pgx.Identifier{"ukinfo_search", d.table()}.Sanitize()
	if _, err = target.Exec(ctx, "CREATE VIEW "+alias+" AS SELECT * FROM "+base); err != nil {
		t.Fatal(err)
	}
	check(query, true)
	for _, q := range []string{
		"dataset=street-prefix&kind=business&sic=12*&postcode=AA1&sort=name&limit=2",
		"dataset=street-prefix&kind=business&sic=12*&town=Invented+Town&sort=edition&limit=2",
		"dataset=street-prefix&kind=business&sic=12*&town=Invented+Town&group=listing&limit=2",
		"dataset=street-prefix&street=invented*&sort=name&limit=2",
	} {
		check(q, false)
	}
	t.Run("AlternateSICFullPaging", func(t *testing.T) { testParadeSICPaging(t, native, hybrid, records) })
	for _, q := range []string{query, "dataset=street-prefix&kind=business&sic=00*&sort=name&limit=1", "dataset=street-prefix&kind=business&sic=12*&town=Invented+Town&limit=1"} {
		want := paradeWalk(t, native, native, q)
		for _, pair := range [][2]*API{{main, hybrid}, {hybrid, main}, {hybrid, hybrid}} {
			if got := paradeWalk(t, pair[0], pair[1], q); !reflect.DeepEqual(got, want) {
				t.Fatal("alternate/main/native cursor exchange differs")
			}
		}
	}
	// A partially ready all-scope selection must stay entirely on the main layout.
	var input bytes.Buffer
	enc := json.NewEncoder(&input)
	enc.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: "sic-second", Edition: "invented-second", Title: "Invented second SIC edition", SHA256: map[string]string{"invented": strings.Repeat("b", 64)}, Expected: 1})
	enc.Encode(map[string]any{"type": "record", "source_id": "0", "record": map[string]any{"entry_type": "business", "organisation": "Other", "sic_code": "1234", "postcode": "AA1 1AA", "town": "Invented Town"}})
	enc.Encode(map[string]any{"type": "end", "count": 1})
	if _, err = Import(ctx, native.DB, &input); err != nil {
		t.Fatal(err)
	}
	if _, err = IndexAddresses(ctx, native.DB, "sic-second", AddressIndexOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err = IndexResearch(ctx, native.DB, "sic-second", AddressIndexOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err = BuildParadeIndex(ctx, native.DB, target, "sic-second", ParadeBuildOptions{}); err != nil {
		t.Fatal(err)
	}
	all := "dataset=all&kind=business&sic=12*&sort=name&limit=2"
	check(all, false)
	second, e := searchEditions(ctx, native.DB, "sic-second")
	if e != nil || len(second) != 1 {
		t.Fatal(e)
	}
	otherAlias := pgx.Identifier{"ukinfo_search", second[0].table() + "_sic"}.Sanitize()
	otherBase := pgx.Identifier{"ukinfo_search", second[0].table()}.Sanitize()
	if _, err = target.Exec(ctx, "CREATE VIEW "+otherAlias+" AS SELECT * FROM "+otherBase+"; INSERT INTO ukinfo_search.sic_editions SELECT * FROM ukinfo_search.editions ON CONFLICT DO NOTHING"); err != nil {
		t.Fatal(err)
	}
	check(all, true)
	want := paradeWalk(t, native, native, all)
	for _, pair := range [][2]*API{{main, hybrid}, {hybrid, main}} {
		if !reflect.DeepEqual(paradeWalk(t, pair[0], pair[1], all), want) {
			t.Fatal("multi-edition cursor exchange differs")
		}
	}
	// Readiness was checked, then the active alternate view fails: never retry main.
	if _, err = target.Exec(ctx, "DROP VIEW "+alias+"; CREATE VIEW "+alias+" AS SELECT id FROM "+base); err != nil {
		t.Fatal(err)
	}
	before := hybrid.metrics.plans["paradedb_fallback"]
	status, _ := fetch(hybrid, query)
	if status != 503 || hybrid.metrics.plans["paradedb_fallback"] != before {
		t.Fatal(fmt.Sprintf("active alternate failure retried or hidden: %d", status))
	}

	// Broken optional metadata also fails closed before any main candidate query.
	if _, err = target.Exec(ctx, "ALTER TABLE ukinfo_search.sic_editions RENAME COLUMN generation TO broken_generation"); err != nil {
		t.Fatal(err)
	}
	mainBefore, alternateBefore := hybrid.metrics.plans["paradedb"], hybrid.metrics.plans[metric]
	status, _ = fetch(hybrid, query)
	if status != 503 || hybrid.metrics.plans["paradedb"] != mainBefore || hybrid.metrics.plans[metric] != alternateBefore || hybrid.metrics.plans["paradedb_fallback"] != before {
		t.Fatal("metadata error retried main or hidden")
	}
}
