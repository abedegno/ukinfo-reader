package catalog

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestParadeSICPrefilterSQL(t *testing.T) {
	for _, tc := range []struct{ input, operator, value string }{
		{"1234", "===", "1234"}, {"0012", "===", "0012"}, {"c++", "===", "c++"},
		{"12*", "regex", "12.*"}, {"c+*", "regex", `c\+.*`}, {"é*", "regex", "é.*"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			for _, after := range []paradePosition{{}, {Dataset: 1, ID: 17, Name: "tied", First: ""}} {
				q := query{Kind: "business", SIC: tc.input, Town: "invented town", Sort: "name", Limit: 51}
				plain, base := paradeSQLPrefix(q, paradeEdition{ID: 1}, after, false)
				got, args := paradeSQL(q, paradeEdition{ID: 1}, after)
				if len(args) != len(base)+1 {
					t.Fatal("missing bound classification prefilter")
				}
				pattern := `sic === \$([0-9]+)`
				if tc.operator == "regex" {
					pattern = `sic @@@ pdb\.regex\(\$([0-9]+)\)`
				}
				match := regexp.MustCompile(pattern).FindStringSubmatch(got)
				if len(match) != 2 {
					t.Fatal("wrong exact/prefix operator")
				}
				at, _ := strconv.Atoi(match[1])
				if args[at-1] != tc.value {
					t.Fatal("classification not bound as literal")
				}
				if strings.Contains(plain, "pdb.regex") || strings.Contains(plain, "===") {
					t.Fatal("scalar-only diagnostic altered")
				}
				if !strings.Contains(got, "sic<>''") || !strings.Contains(got, "town=") || !strings.Contains(got, "ORDER BY name,first_full,id") {
					t.Fatal("original filters/order lost")
				}
				if args[len(args)-1] != 51 || !strings.HasSuffix(got, fmt.Sprintf("LIMIT $%d", len(args))) {
					t.Fatal("lookahead binding shifted")
				}
				if after.ID > 0 && !strings.Contains(got, fmt.Sprintf("id>$%d", len(args)-1)) {
					t.Fatal("cursor binding shifted")
				}
			}
		})
	}
	q := query{Kind: "business", Industry: "chem*", Limit: 51}
	a, x := paradeSQL(q, paradeEdition{ID: 1}, paradePosition{})
	b, y := paradeSQLPrefix(q, paradeEdition{ID: 1}, paradePosition{}, false)
	if a != b || !reflect.DeepEqual(x, y) {
		t.Fatal("absent SIC changed")
	}
	q = query{Kind: "business", SIC: "12*", Street: "invented*", Postcode: "AA1", PostcodeLength: 6, Limit: 51}
	got, args := paradeSQL(q, paradeEdition{ID: 1}, paradePosition{})
	if strings.Count(got, "pdb.regex(") != 4 {
		t.Fatal("combined prefilters missing")
	}
	for _, want := range []string{"12.*", "invented.*", "AA1.*"} {
		have := false
		for _, arg := range args {
			if arg == want {
				have = true
			}
		}
		if !have {
			t.Fatal("combined literal bindings collided")
		}
	}
}
