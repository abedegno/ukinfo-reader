package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSelectiveAddressIntegration(t *testing.T) {
	dsn := os.Getenv("UKINFO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set UKINFO_TEST_DATABASE_URL to a disposable PostgreSQL 18 database")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	schema := fmt.Sprintf("ukinfo_address_test_%d", os.Getpid())
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
	if err = Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"selective-one", "selective-two"} {
		var input bytes.Buffer
		enc := json.NewEncoder(&input)
		_ = enc.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: key, Edition: key, Title: "Invented address plan test", SHA256: map[string]string{"invented": "invented"}, Expected: 273})
		for i := 0; i < 273; i++ {
			name := "Smallmatch"
			if i >= 12 {
				name = "Boundarymatch"
			}
			if i >= 140 {
				name = "Broadmatch"
			}
			r := map[string]any{"surname": name, "firstName": "June A", "street_2": "Invented Road", "town": "Invented Town", "county": "Old County", "postcode": "AA1 1AA", "houseno": fmt.Sprint(i + 1)}
			if i%2 == 1 {
				r["town"] = "Other Town"
			}
			if i >= 269 {
				r["entry_type"] = "business"
				r["organisation"] = "Smallmatch Shop"
			}
			_ = enc.Encode(map[string]any{"type": "record", "source_id": fmt.Sprint(i), "record": r})
		}
		_ = enc.Encode(map[string]any{"type": "end", "count": 273})
		if _, err = Import(ctx, db, &input); err != nil {
			t.Fatal(err)
		}
		if _, err = IndexAddresses(ctx, db, key, AddressIndexOptions{}); err != nil {
			t.Fatal(err)
		}
		if _, err = IndexResearch(ctx, db, key, AddressIndexOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	var bulk bytes.Buffer
	enc := json.NewEncoder(&bulk)
	_ = enc.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: "large-address", Edition: "large-address", Title: "Invented address budgets", Expected: 22000, SHA256: map[string]string{"invented": "invented"}})
	for i := 0; i < 22000; i++ {
		name, postcode := "Widebudget", "BB1 1BB"
		if i >= 4500 {
			name = "Packedbudget"
		}
		if i >= 5010 {
			name = "Fillerbudget"
			if i%128 == 0 {
				name = "Scatteredbudget"
			}
		}
		if i < 300 {
			postcode = "CC1 1CC"
		}
		row := map[string]any{"surname": name, "firstName": "June A", "street": "Large Road", "town": "Large Town", "county": "Old County", "postcode": postcode, "houseno": fmt.Sprint(i + 1)}
		_ = enc.Encode(map[string]any{"type": "record", "source_id": fmt.Sprint(i), "record": row})
	}
	_ = enc.Encode(map[string]any{"type": "end", "count": 22000})
	if _, err = Import(ctx, db, &bulk); err != nil {
		t.Fatal(err)
	}
	if _, err = IndexAddresses(ctx, db, "large-address", AddressIndexOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err = IndexResearch(ctx, db, "large-address", AddressIndexOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "INSERT INTO terms(key) SELECT 'jaaa'||lpad(i::text,5,'0') FROM generate_series(0,8192) i ON CONFLICT DO NOTHING"); err != nil {
		t.Fatal(err)
	}
	testRequestBlockReuse(t, db)
	a := NewAPI(db)
	for _, tc := range []struct {
		raw       string
		selective bool
		count     int
	}{
		{"dataset=all&surname=Smallmatch&first_name=June&first_match=given&town=Invented+Town", true, 12},
		{"dataset=selective-one&surname=Boundarymatch&town=Invented+Town", true, 64},
		{"dataset=all&surname=Boundarymatch&town=Invented+Town", true, 128},
		{"dataset=selective-one&surname=Broadmatch&town=Invented+Town", true, 65},
		{"dataset=all&kind=business&organisation=Smallmatch+Shop&town=Invented+Town", true, 4},
		{"dataset=all&surname=Smallmatch&street=Invented*&county=Old*&town=Invented+Town&sort=name", true, 12},
		{"dataset=all&surname=Smallmatch&town=Invented+Town&sort=edition", true, 12},
		{"dataset=all&surname=Smallmatch&town=Invented+Town&group=listing", true, 6},
		{"dataset=all&surname=Smallmatch&town=Missing+Town", true, 0},
		{"dataset=all&postcode=CC11CC&town=Large+Town", true, 300},
		{"dataset=all&postcode=CC11CC&town=Large+Town&house=299", true, 1},
		{"dataset=all&surname=Packedbudget&town=Large+Town", true, 510},
		{"dataset=large-address&surname=Widebudget&town=Large+Town", false, 4500},
		{"dataset=all&street=Invented+Road&town=Invented+Town", true, 270},
		{"dataset=large-address&surname=Scatteredbudget&town=Large+Town", false, 132},
		{"dataset=all&surname=Smallmatch&first_name=J*&middle_initial=A&town=Invented+Town", true, 12},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			q, err := parse(mustValues(tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			f, status, err := a.candidateFilters(ctx, q)
			if err != nil || status != 200 || f.addressMatched != tc.selective {
				t.Fatalf("plan: selective=%v status=%d err=%v", f.addressMatched, status, err)
			}
			pageLimit := 3
			if tc.count > 100 {
				pageLimit = 100
			}
			seen := map[string]bool{}
			cursor := ""
			total := 0
			for page := 0; ; page++ {
				if page > 1600 {
					t.Fatal("endless pagination")
				}
				w := httptest.NewRecorder()
				a.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?"+tc.raw+fmt.Sprintf("&limit=%d&cursor=", pageLimit)+url.QueryEscape(cursor), nil))
				if w.Code != 200 {
					t.Fatal(w.Code, w.Body.String())
				}
				var out searchResult
				if err = json.Unmarshal(w.Body.Bytes(), &out); err != nil {
					t.Fatal(err)
				}
				for _, r := range out.Items {
					identity := r.Dataset + ":" + r.SourceID
					if seen[identity] {
						t.Fatal("duplicate page entry", identity)
					}
					seen[identity] = true
					if !strings.Contains(string(r.Record), "Invented Town") && !strings.Contains(string(r.Record), "Large Town") {
						t.Fatal("wrong town")
					}
				}
				total += len(out.Items)
				if out.Next == "" {
					break
				}
				cursor = out.Next
			}
			if total != tc.count {
				t.Fatalf("got %d entries, want %d", total, tc.count)
			}
		})
	}
	testOrderedSurnameWindows(t, db)
}

func testOrderedSurnameWindows(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	var input bytes.Buffer
	enc := json.NewEncoder(&input)
	_ = enc.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: "surname-windows", Edition: "surname-windows", Title: "Invented ordered names", Expected: 360, SHA256: map[string]string{"invented": "invented"}})
	for i := 359; i >= 0; i-- {
		name := fmt.Sprintf("Surnamewindow%04d", 2*i)
		_ = enc.Encode(map[string]any{"type": "record", "source_id": fmt.Sprint(i), "record": map[string]any{"surname": name, "firstName": "Name A", "street": "Invented Road", "houseno": "1", "postcode": "AA1 1AA"}})
	}
	_ = enc.Encode(map[string]any{"type": "end", "count": 360})
	if _, err := Import(ctx, db, &input); err != nil {
		t.Fatal(err)
	}
	if _, err := IndexResearch(ctx, db, "surname-windows", AddressIndexOptions{}); err != nil {
		t.Fatal(err)
	}
	// Unused terms must not consume the bounded surname window.
	if _, err := db.Exec(ctx, "INSERT INTO terms(key) SELECT 'surnamewindow'||lpad((i*2+1)::text,4,'0') FROM generate_series(0,359) i ON CONFLICT DO NOTHING"); err != nil {
		t.Fatal(err)
	}
	a := NewAPI(db)
	for _, mode := range []string{"sort=name", "group=listing"} {
		seen := map[string]bool{}
		cursor := ""
		last := ""
		for page := 0; ; page++ {
			if page > 10 {
				t.Fatal("endless surname pages")
			}
			w := httptest.NewRecorder()
			a.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?dataset=surname-windows&surname=Surnamewindow*&first_name=Name&first_match=given&limit=100&"+mode+"&cursor="+url.QueryEscape(cursor), nil))
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			var out searchResult
			if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			for _, r := range out.Items {
				var row map[string]any
				_ = json.Unmarshal(r.Record, &row)
				name := row["surname"].(string)
				if name <= last || seen[r.SourceID] {
					t.Fatal("wrong surname order or duplicate", name)
				}
				last = name
				seen[r.SourceID] = true
			}
			if out.Next == "" {
				break
			}
			cursor = out.Next
		}
		if len(seen) != 360 {
			t.Fatal("truncated surname windows", len(seen))
		}
	}
}
