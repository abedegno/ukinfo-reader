package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSearchEngineSpikeSemantics(t *testing.T) {
	dsn := os.Getenv("UKINFO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set UKINFO_TEST_DATABASE_URL to a disposable PostgreSQL database")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	schema := fmt.Sprintf("ukinfo_engine_spike_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := BuildSearchEngineSpike(ctx, db, db, "se2003-dpcs0104", nil, nil); err == nil || !strings.Contains(err.Error(), "separate PostgreSQL servers") {
		t.Fatalf("same-server guard: %v", err)
	}
	var untouched bool
	if err := db.QueryRow(ctx, "SELECT to_regclass('spike_build') IS NULL").Scan(&untouched); err != nil || !untouched {
		t.Fatal("same-server guard wrote experiment metadata")
	}
	_, err = db.Exec(ctx, `CREATE TABLE native(id integer,kind smallint,name text COLLATE "C",first_full text COLLATE "C",given text COLLATE "C",postcode text COLLATE "C",street text COLLATE "C",street_2 text COLLATE "C",locality text COLLATE "C",locality_2 text COLLATE "C",town text COLLATE "C",county text COLLATE "C",sic text COLLATE "C",industry text COLLATE "C",postcode_length smallint GENERATED ALWAYS AS(length(postcode)) STORED)`)
	if err != nil {
		t.Fatal(err)
	}
	documents := []map[string]any{
		{"surname": "O'Neill", "firstName": "Ann A", "postcode": "W1 1AA", "street_1": "Fallback Street", "street_2": "Second Street", "locality_2": "Second Locality", "town": "Test Town", "county": "Test County"},
		{"surname": "O'Neill", "firstName": "Anna", "postcode": "W11 1AA", "town": "Test Town"},
		{"surname": "O'Neill", "firstName": "Ann B", "postcode": "W11", "town": "Test Town"},
		{"surname": "Other", "firstName": "Ann", "postcode": "W1 1AA", "town": "Other Town"},
		{"entry_type": "business", "organisation": "O'Neill Supplies", "firstName": "Hidden First", "sic_code": "0042", "town": "Test Town"},
		{"surname": "O'Neill", "firstName": "Ann", "postcode": "", "postcode_raw": "W1 1AA", "street": "Public Street", "street_1": "Hidden Street", "town": "Test Town"},
	}
	for i, doc := range documents {
		raw, _ := json.Marshal(doc)
		v, e := spikeFlat(int32(i+1), raw)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.CopyFrom(ctx, pgx.Identifier{"native"}, spikeColumns, pgx.CopyFromRows([][]any{v})); e != nil {
			t.Fatal(e)
		}
	}
	// A two-digit outward code must not be shortened to a different district.
	sampled, _ := spikeFlat(1201, []byte(`{"surname":"Sampled","firstName":"June","postcode":"ZZ12 3ZZ","town":"Sampled Town"}`))
	if _, e := db.CopyFrom(ctx, pgx.Identifier{"native"}, spikeColumns, pgx.CopyFromRows([][]any{sampled})); e != nil {
		t.Fatal(e)
	}
	generated, e := CreateSearchEngineCases(ctx, db, nil)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, c := range generated {
		v, e := url.ParseQuery(c.Query)
		if e != nil {
			t.Fatal(e)
		}
		if v.Get("postcode") == "" {
			continue
		}
		if v.Get("postcode") != "ZZ12" {
			t.Fatal("sampled postcode prefix changed its district")
		}
		q, e := parse(v)
		if e != nil {
			t.Fatal(e)
		}
		sql, args := spikeQuery("native", q, spikePosition{})
		var id int32
		var name, first string
		if e = db.QueryRow(ctx, sql, args...).Scan(&id, &name, &first); e != nil || id != 1201 {
			t.Fatal("sampled prefix failed to match its source")
		}
		found = true
	}
	if !found {
		t.Fatal("missing positive sampled postcode case")
	}
	engines := []string{"native"}
	var hasSearch bool
	if e := db.QueryRow(ctx, "SELECT EXISTS(SELECT FROM pg_extension WHERE extname='pg_search')").Scan(&hasSearch); e != nil {
		t.Fatal(e)
	}
	if hasSearch {
		_, e := db.Exec(ctx, `CREATE TABLE parade (LIKE native INCLUDING DEFAULTS INCLUDING CONSTRAINTS); INSERT INTO parade SELECT * FROM native; CREATE INDEX probe_search ON parade USING paradedb(id,kind,(name::pdb.literal),(first_full::pdb.literal),(given::pdb.literal),(postcode::pdb.literal),(street::pdb.literal),(street_2::pdb.literal),(locality::pdb.literal),(locality_2::pdb.literal),(town::pdb.literal),(county::pdb.literal),(sic::pdb.literal),(industry::pdb.literal),postcode_length)`)
		if e != nil {
			t.Fatal(e)
		}
		engines = append(engines, "parade")
	}
	for _, tc := range []struct {
		query string
		want  []int32
	}{
		{"surname=O%27Neill&first_name=Ann&first_match=given", []int32{1, 3, 6}},
		{"surname=O%27Neill&first_name=Ann&first_match=exact", []int32{6}},
		{"surname=O%27Neill&first_name=Ann*&first_match=prefix", []int32{1, 2, 3, 6}},
		{"postcode=W1+1", []int32{1, 3, 4}},
		{"postcode=W11", []int32{2, 3}},
		{"street=Second+Street&county=Test+County", []int32{1}},
		{"street=Second+Street&county=Wrong+County", nil},
		{"locality=Second+Locality", []int32{1}},
		{"street=Hidden+Street", nil},
		{"kind=business&sic=0042&town=Test+Town", []int32{5}},
		{"surname=O%27Neill&first_name=Ann+B&first_match=given", []int32{3}},
	} {
		for _, engine := range engines {
			t.Run(engine+"/"+tc.query, func(t *testing.T) {
				q, e := parse(mustValues("dataset=test&limit=50&" + tc.query))
				if e != nil {
					t.Fatal(e)
				}
				sql, args := spikeQuery(engine, q, spikePosition{})
				rows, e := db.Query(ctx, sql, args...)
				if e != nil {
					t.Fatal(e)
				}
				defer rows.Close()
				var ids []int32
				for rows.Next() {
					var p spikePosition
					if e = rows.Scan(&p.id, &p.name, &p.first); e != nil {
						t.Fatal(e)
					}
					ids = append(ids, p.id)
				}
				if e = rows.Err(); e != nil {
					t.Fatal(e)
				}
				if !slices.Equal(ids, tc.want) {
					t.Fatalf("got %v want %v", ids, tc.want)
				}
			})
		}
	}
	q, err := parse(mustValues("dataset=test&surname=O%27Neill&sort=name&limit=1"))
	if err != nil {
		t.Fatal(err)
	}
	for _, engine := range engines {
		var got []int32
		var after spikePosition
		for page := 0; page < 5; page++ {
			sql, args := spikeQuery(engine, q, after)
			rows, e := db.Query(ctx, sql, args...)
			if e != nil {
				t.Fatal(e)
			}
			var ps []spikePosition
			for rows.Next() {
				var p spikePosition
				if e = rows.Scan(&p.id, &p.name, &p.first); e != nil {
					t.Fatal(e)
				}
				ps = append(ps, p)
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				t.Fatal(e)
			}
			if len(ps) == 0 {
				break
			}
			got = append(got, ps[0].id)
			if len(ps) == 1 {
				break
			}
			after = ps[0]
		}
		if !slices.Equal(got, []int32{6, 1, 3, 2}) {
			t.Fatalf("%s lexical pages %v", engine, got)
		}
	}
}
