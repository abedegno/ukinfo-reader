package catalog

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func testParadeSICPaging(t *testing.T, native, candidate *API, records []map[string]any) {
	t.Helper()
	for _, sic := range []string{"1234", "12*", "0012", "00*", "12.3", "12.*", "c++", "c+*", "é12", "é*", "東京12", "東京*", "absent", ""} {
		expected := 0
		for _, r := range records {
			if r["entry_type"] != "business" {
				continue
			}
			value, _ := r["sic_code"].(string)
			if sic == "" || strings.HasSuffix(sic, "*") && value != "" && strings.HasPrefix(value, strings.TrimSuffix(sic, "*")) || value == sic {
				expected++
			}
		}
		for _, order := range []string{"name", "edition", "catalogue"} {
			v := url.Values{"dataset": {"street-prefix"}, "kind": {"business"}, "sic": {sic}, "sort": {order}, "limit": {"3"}}
			query := v.Encode()
			want := paradeWalk(t, native, native, query)
			if len(want) != expected {
				t.Fatalf("independent SIC fixture count: got %d want %d", len(want), expected)
			}
			for _, pair := range [][2]*API{{candidate, candidate}, {native, candidate}, {candidate, native}} {
				got := paradeWalk(t, pair[0], pair[1], query)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("SIC content/cursor mismatch order=%s", order)
				}
			}
		}
	}
	for _, tail := range []string{"town=Invented+Town", "postcode=AA1", "street=second*", "house=1", "group=listing"} {
		query := "dataset=street-prefix&kind=business&sic=12*&sort=name&limit=3&" + tail
		want := paradeWalk(t, native, native, query)
		got := paradeWalk(t, candidate, candidate, query)
		if len(want) == 0 || !reflect.DeepEqual(got, want) {
			t.Fatal("combined SIC matching/paging mismatch")
		}
	}
}
