package catalog

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParadeSICLayoutPolicy(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want bool
	}{
		{"kind=business&sic=12*&sort=name", true}, {"kind=business&sic=0012&town=Invented", true}, {"kind=business&sic=12*&town=Invented&organisation=Invented", true},
		{"kind=business&sic=12*", false}, {"kind=business&sic=12*&postcode=AA1&sort=name", false}, {"kind=business&sic=12*&town=Invented&near=AA1+1AA", false}, {"kind=business&sic=12*&town=Invented&group=listing", false}, {"kind=business&sic=12*&town=Invented&sort=edition", false}, {"kind=business&industry=Invented&sort=name", false}, {"postcode=AA1&sort=name", false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			q, err := parse(mustValues("dataset=all&" + tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			if paradeSICLayoutEligible(q) != tc.want {
				t.Fatal("wrong optional layout selection")
			}
			for _, enabled := range []bool{false, true} {
				if enabled && tc.want {
					continue
				}
				a := NewAPI(nil)
				a.ParadeDB = &pgxpool.Pool{}
				a.ExperimentalSICUnpartitioned = enabled
				editions := []paradeEdition{{ID: 7}}
				got, used, err := a.paradeSICLayout(context.Background(), q, editions)
				if err != nil || used || !reflect.DeepEqual(got, editions) {
					t.Fatal("disabled/ineligible query touched optional inventory")
				}
			}
		})
	}
}
func TestParadeSICLayoutGenerationAndSQL(t *testing.T) {
	d := paradeEdition{ID: 7, Key: "fixture", Manifest: json.RawMessage(`{}`), Entries: 42, Imported: time.Unix(0, 0).UTC()}
	const generation = "0ed7cedc20da0cca40be94bef7325d42959a2ee79cd74528573188822253169b"
	if d.generation() != generation {
		t.Fatal("pre-experiment generation format changed")
	}
	base := d.table()
	q, err := parse(mustValues("dataset=fixture&kind=business&sic=00*&town=Invented&sort=name"))
	if err != nil {
		t.Fatal(err)
	}
	before, args := paradeSQL(q, d, paradePosition{})
	d.searchTable = base + "_sic"
	after, other := paradeSQL(q, d, paradePosition{})
	if d.generation() != generation || d.table() != base+"_sic" {
		t.Fatal("request-local table selection invalidates source identity")
	}
	if after != strings.Replace(before, "ukinfo_search."+base, "ukinfo_search."+base+"_sic", 1) || !reflect.DeepEqual(args, other) {
		t.Fatal("routing changed predicates/order/bindings")
	}
	if NewAPI(nil).ExperimentalSICUnpartitioned {
		t.Fatal("experimental route defaults on")
	}
}
