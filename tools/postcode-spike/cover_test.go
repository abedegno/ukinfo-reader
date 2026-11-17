package main

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestCoveringVariantKeepsAllScalarPredicatesAndBindings(t *testing.T) {
	s := twoStatements()[0]
	got, err := coveringVariant(s.SQL, s.Args)
	want := strings.ReplaceAll(s.SQL, "postcode @@@ pdb.regex($6) AND id @@@ pdb.all()", "($6::text IS NOT NULL)")
	if err != nil || got != want || strings.Contains(got, "@@@") {
		t.Fatal("covering SQL changed authoritative predicates")
	}
	for _, sql := range []string{s.SQL + " OFFSET 1", strings.Replace(s.SQL, "ORDER BY name,first_full,id", "ORDER BY id", 1), strings.Replace(s.SQL, "kind=$1", "kind=$1 OR true", 1)} {
		if _, err := coveringVariant(sql, s.Args); err == nil {
			t.Fatal("unknown shape accepted")
		}
	}
	full := candidatePrefix + "d1_aaaaaaaaaaaaaaaaaaaaaaaa" + fullCandidateTail
	if _, err := coveringVariant(full, []any{int16(0), "AA11AA", 51}); err == nil {
		t.Fatal("full control substituted for district")
	}
}

func TestCoverComparisonSelectsDistrictAndCounterbalancesTwice(t *testing.T) {
	cases := twoStatements()
	cases[1] = statement{candidatePrefix + "d1_aaaaaaaaaaaaaaaaaaaaaaaa" + fullCandidateTail, []any{int16(0), "AA11AA", 51}}
	seen := []string{}
	report, err := compareCovering(context.Background(), cases, func(_ context.Context, s statement) reading {
		label := "original_bm25"
		if !strings.Contains(s.SQL, "@@@") {
			label = "covering_btree"
		}
		seen = append(seen, label)
		return reading{Rows: 51, Digest: [32]byte{1}, Milliseconds: 1}
	})
	want := []string{"original_bm25", "covering_btree", "covering_btree", "original_bm25", "covering_btree", "original_bm25", "original_bm25", "covering_btree"}
	if err != nil || !report.Complete || !report.SQLRowsEqual || !reflect.DeepEqual(seen, want) {
		t.Fatal("invalid counterbalanced covering comparison")
	}
	for i, o := range report.Observations {
		if o.Strategy != seen[i] {
			t.Fatal("wrong strategy label")
		}
	}
	for _, mode := range []string{"mismatch", "error"} {
		n := 0
		r, _ := compareCovering(context.Background(), cases, func(context.Context, statement) reading {
			n++
			r := reading{Rows: 1, Digest: [32]byte{1}, Milliseconds: 1}
			if n == 5 {
				if mode == "mismatch" {
					r.Digest = [32]byte{2}
				} else {
					r.Err = errors.New("PRIVATE")
				}
			}
			return r
		})
		if r.SQLRowsEqual {
			t.Fatal("incomplete row equivalence accepted")
		}
	}
	if _, err := compareCovering(context.Background(), twoStatements(), func(context.Context, statement) reading { return reading{} }); err == nil {
		t.Fatal("two district cases accepted")
	}
}

func TestCoverIndexAdmissionRejectsEveryMetadataDrift(t *testing.T) {
	table := "d1_aaaaaaaaaaaaaaaaaaaaaaaa"
	good := coverIndex{Generation: strings.Repeat("a", 64), Entries: 99, LastID: 99, Version: 1, Ready: true, Index: "p8_pc_" + table + "_cover", Table: table, Valid: true, IndexReady: true, Method: "btree", NKeys: 5, NAttrs: 9, Columns: []string{"kind", "postcode", "name", "first_full", "id", "name_id", "first_id", "postcode_id", "postcode_length"}, Collations: []string{"C", "C", "C"}, Options: []int{0, 0, 0, 0, 0}, Owner: "postgres", TableOwner: "postgres", Database: "ukinfo_search", Bytes: 4096, Server: 180007, Search: "0.26.3", Ordinary: true}
	if validateCoverIndex(good, table) != nil {
		t.Fatal("valid index rejected")
	}
	changes := []func(*coverIndex){
		func(x *coverIndex) { x.Generation = strings.Repeat("b", 64) }, func(x *coverIndex) { x.Entries = 0 }, func(x *coverIndex) { x.LastID-- }, func(x *coverIndex) { x.Version++ }, func(x *coverIndex) { x.Ready = false }, func(x *coverIndex) { x.Valid = false }, func(x *coverIndex) { x.IndexReady = false }, func(x *coverIndex) { x.Method = "bm25" }, func(x *coverIndex) { x.NKeys-- }, func(x *coverIndex) { x.NAttrs-- }, func(x *coverIndex) { x.Partial = true }, func(x *coverIndex) { x.Expressions = true }, func(x *coverIndex) { x.Unique = true }, func(x *coverIndex) { x.Tablespace = 1 }, func(x *coverIndex) { x.Index += "x" }, func(x *coverIndex) { x.Table += "x" }, func(x *coverIndex) { x.Columns = []string{"id"} }, func(x *coverIndex) { x.Collations = []string{"default", "C", "C"} }, func(x *coverIndex) { x.Options = []int{0, 1, 0, 0, 0} }, func(x *coverIndex) { x.Owner = "foreign" }, func(x *coverIndex) { x.TableOwner = "foreign" }, func(x *coverIndex) { x.Database = "foreign" }, func(x *coverIndex) { x.Bytes = 17 << 30 }, func(x *coverIndex) { x.Server-- }, func(x *coverIndex) { x.Search = "foreign" }, func(x *coverIndex) { x.Ordinary = false },
	}
	for i, change := range changes {
		bad := good
		change(&bad)
		if validateCoverIndex(bad, table) == nil {
			t.Fatalf("metadata drift %d admitted", i)
		}
	}
}
