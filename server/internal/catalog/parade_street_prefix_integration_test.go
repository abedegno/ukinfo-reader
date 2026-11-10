package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Exercise the serving parser, both street positions and opaque HTTP cursor
// exchange against the independent native implementation, not another SQL copy.
func TestParadeStreetPrefixIntegration(t *testing.T) {
	sourceDSN, targetDSN := os.Getenv("UKINFO_TEST_DATABASE_URL"), os.Getenv("UKINFO_PARADE_TEST_DATABASE_URL")
	if sourceDSN == "" || targetDSN == "" {
		t.Skip("requires disposable source and separate pg_search 0.26.3 target")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, sourceDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("street_source_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, err := pgxpool.ParseConfig(sourceDSN)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	source, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	cfg, err = pgxpool.ParseConfig(targetDSN)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["plan_cache_mode"] = "force_generic_plan"
	target, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	var occupied bool
	if err = target.QueryRow(ctx, "SELECT to_regnamespace('ukinfo_search') IS NOT NULL").Scan(&occupied); err != nil || occupied {
		t.Fatal("target must be disposable", err)
	}
	defer target.Exec(ctx, "DROP SCHEMA IF EXISTS ukinfo_search CASCADE")
	if err = Migrate(ctx, source); err != nil {
		t.Fatal(err)
	}
	prefixes := []string{"invented", "second", "a.b", "c++ (east)", "bracket[", "caret^", `back\road`, "quote'", "école", "東京", "both", strings.Repeat("z", 200)}
	records := []map[string]any{}
	for _, kind := range []string{"person", "business"} {
		for _, prefix := range prefixes {
			for i := 0; i < 5; i++ {
				value := prefix + " road"
				r := map[string]any{"surname": "Tied", "firstName": "Ann", "postcode": "AA1 1AA", "street": value, "town": "Invented Town", "houseno": "1"}
				if kind == "business" {
					r["entry_type"] = "business"
					r["organisation"] = "Tied"
					r["sic_code"] = []string{"1234", "1235", "0012", "12.3", "c++"}[i]
					if i == 0 && prefix == "école" {
						r["sic_code"] = "é12"
					}
					if i == 0 && prefix == "東京" {
						r["sic_code"] = "東京12"
					}
				}
				if i == 1 {
					r["street"] = "Unrelated"
					r["street_2"] = value
				}
				if i == 2 {
					r["street_2"] = value
				}
				if i >= 3 {
					r["surname"] = "Later"
					r["organisation"] = "Later"
				}
				records = append(records, r)
			}
		}
		r := map[string]any{"surname": "Empty", "firstName": "Ann", "postcode": "AA1 1AA"}
		if kind == "business" {
			r["entry_type"] = "business"
			r["organisation"] = "Empty"
		}
		records = append(records, r)
	}
	var input bytes.Buffer
	enc := json.NewEncoder(&input)
	enc.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: "street-prefix", Edition: "invented-2003-01", Title: "Invented street prefixes", SHA256: map[string]string{"invented": strings.Repeat("a", 64)}, Expected: int64(len(records))})
	for i, r := range records {
		enc.Encode(map[string]any{"type": "record", "source_id": fmt.Sprint(i), "record": r})
	}
	enc.Encode(map[string]any{"type": "end", "count": len(records)})
	if _, err = Import(ctx, source, &input); err != nil {
		t.Fatal(err)
	}
	if _, err = IndexAddresses(ctx, source, "street-prefix", AddressIndexOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err = IndexResearch(ctx, source, "street-prefix", AddressIndexOptions{}); err != nil {
		t.Fatal(err)
	}
	if n, err := BuildParadeIndex(ctx, source, target, "street-prefix", ParadeBuildOptions{}); err != nil || n != int64(len(records)) {
		t.Fatal("fixture build", n, err)
	}
	native, candidate := NewAPI(source), NewAPI(source)
	candidate.ParadeDB = target
	for _, kind := range []string{"person", "business"} {
		for _, prefix := range append(prefixes, "absent") {
			for _, order := range []string{"name", "edition", "catalogue"} {
				v := url.Values{"dataset": {"street-prefix"}, "kind": {kind}, "street": {prefix + "*"}, "sort": {order}, "limit": {"2"}}
				query := v.Encode()
				want := paradeWalk(t, native, native, query)
				expected := 5
				if prefix == "absent" {
					expected = 0
				}
				if len(want) != expected {
					t.Fatalf("independent fixture count: got %d want %d", len(want), expected)
				}
				for _, pair := range [][2]*API{{candidate, candidate}, {native, candidate}, {candidate, native}} {
					got := paradeWalk(t, pair[0], pair[1], query)
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("prefix paging differs for kind=%s order=%s", kind, order)
					}
				}
			}
		}
	}
	for _, tail := range []string{"postcode=AA1", "postcode=AA1+1AA", "town=Invented+Town", "house=1"} {
		query := "dataset=street-prefix&street=" + url.QueryEscape("c++ (east)*") + "&sort=name&limit=2&" + tail
		if got, want := paradeWalk(t, candidate, candidate, query), paradeWalk(t, native, native, query); len(got) != 5 || !reflect.DeepEqual(got, want) {
			t.Fatal("combined/refinement paging differs")
		}
	}
	t.Run("SICCursorEquality", func(t *testing.T) { testParadeSICPaging(t, native, candidate, records) })
	t.Run("SICLayoutRouting", func(t *testing.T) { testParadeSICLayoutRouting(t, native, candidate, records) })
	if candidate.metrics.plans["paradedb"] == 0 {
		t.Fatal("only exercised fallback")
	}
}
