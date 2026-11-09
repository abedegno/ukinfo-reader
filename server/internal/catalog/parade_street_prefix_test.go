package catalog

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestParadeStreetPrefixSQL(t *testing.T) {
	for _, tc := range []struct{ street, pattern string }{
		{"invented*", "invented.*"}, {"c++ (east)*", `c\+\+ \(east\).*`},
		{`back\road*`, `back\\road.*`}, {"école*", "école.*"}, {"東京*", "東京.*"},
	} {
		t.Run(tc.street, func(t *testing.T) {
			for _, after := range []paradePosition{{}, {ID: 17, Name: "example", First: "ann", Dataset: 1}} {
				q := query{Kind: "person", Street: tc.street, Sort: "name", Limit: 51}
				base, baseArgs := paradeSQLPrefix(q, paradeEdition{ID: 1}, after, false)
				got, args := paradeSQL(q, paradeEdition{ID: 1}, after)
				if len(args) != len(baseArgs)+1 {
					t.Fatalf("missing bound street prefilter: %d vs %d", len(args), len(baseArgs))
				}
				at := -1
				for i, v := range args {
					if v == tc.pattern {
						at = i + 1
					}
				}
				if at < 1 {
					t.Fatal("literal prefix not safely escaped and bound")
				}
				want := fmt.Sprintf("(street @@@ pdb.regex($%d) OR street_2 @@@ pdb.regex($%d))", at, at)
				if !strings.Contains(got, want) || strings.Contains(base, "pdb.regex") {
					t.Fatal("both fields and scalar-only opt-out required")
				}
				if !strings.Contains(got, "street<>''") || !strings.Contains(got, "street_2<>''") || !strings.Contains(got, "ORDER BY name,first_full,id") {
					t.Fatal("scalar semantics/order lost")
				}
				if args[len(args)-1] != 51 || !strings.HasSuffix(got, fmt.Sprintf("LIMIT $%d", len(args))) {
					t.Fatal("limit binding shifted")
				}
				if after.ID > 0 && !strings.Contains(got, fmt.Sprintf("id>$%d", len(args)-1)) {
					t.Fatal("continuation ID binding shifted")
				}
			}
		})
	}
	for _, street := range []string{"", "invented street"} {
		q := query{Street: street, Limit: 51}
		a, x := paradeSQL(q, paradeEdition{ID: 1}, paradePosition{})
		b, y := paradeSQLPrefix(q, paradeEdition{ID: 1}, paradePosition{}, false)
		if a != b || !reflect.DeepEqual(x, y) {
			t.Fatal("exact/absent street changed")
		}
	}
	q := query{Street: "c++*", Postcode: "AA1", PostcodeLength: 6, Limit: 51}
	got, args := paradeSQL(q, paradeEdition{ID: 1}, paradePosition{})
	if strings.Count(got, "pdb.regex(") != 3 {
		t.Fatal("postcode and both street fields require independent prefilters")
	}
	have := map[string]bool{}
	for _, v := range args {
		if s, ok := v.(string); ok {
			have[s] = true
		}
	}
	if !have["AA1.*"] || !have[`c\+\+.*`] {
		t.Fatal("postcode/street bindings collided")
	}
}
