package catalog

import (
	"regexp"
	"strings"
	"testing"
)

func TestParadePostcodePrefilterKeepsDistrictSectorAndStoredPartial(t *testing.T) {
	for _, tc := range []struct {
		input   string
		yes, no []string
	}{
		{"W11", []string{"W11", "W111AA", "W11ÅßA"}, []string{"W11AA", "W1111AA"}},
		{"W1+1", []string{"W11", "W11AA", "W11Åß"}, []string{"W111AA", "W1111AA"}},
	} {
		t.Run(tc.input, func(t *testing.T) {
			q, err := parse(mustValues("dataset=invented&postcode=" + tc.input + "&sort=name"))
			if err != nil {
				t.Fatal(err)
			}
			sql, args := paradeSQL(q, paradeEdition{ID: 1, Key: "invented"}, paradePosition{})
			if !strings.Contains(sql, "postcode_length=$") || !strings.Contains(sql, " OR postcode=$") || !strings.Contains(sql, "postcode >= $") {
				t.Fatal("authoritative scalar precision lost")
			}
			pattern := args[len(args)-2].(string)
			re, err := regexp.Compile("^(?:" + pattern + ")$")
			if err != nil {
				t.Fatal(err)
			}
			for _, text := range tc.yes {
				if !re.MatchString(text) {
					t.Fatal("valid stored partial/precision omitted")
				}
			}
			for _, text := range tc.no {
				if re.MatchString(text) {
					t.Fatal("wrong postcode precision admitted by prefilter")
				}
			}
		})
	}
}
