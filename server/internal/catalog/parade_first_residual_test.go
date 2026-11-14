package catalog

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestParadeFirstResidualScope(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  bool
	}{
		{"surname=Invented&town=Imaginary&first_name=Ma*&first_match=given", true},
		{"surname=Invented&town=Imaginary&first_name=Mary+Jane&first_match=given", true},
		{"surname=Invented&town=Imaginary&first_name=Mary&first_match=prefix&sort=name", true},
		{"surname=Invented&town=Imaginary&first_name=Mary&first_match=given", false},
		{"surname=Invented&town=Imaginary&first_name=Mary&first_match=exact", false},
		{"surname=Inv*&town=Imaginary&first_name=Ma*", false},
		{"surname=Invented&town=Imaginary*&first_name=Ma*", false},
		{"surname=Invented&town=Imaginary&first_name=Mary&match=prefix", false},
		{"surname=Invented&first_name=Ma*", false},
		{"town=Imaginary&first_name=Ma*", false},
		{"surname=Invented&town=Imaginary", false},
		{"surname=Invented&town=Imaginary&first_name=Ma*&near=AA1+1AA", false},
		{"kind=business&organisation=Invented&town=Imaginary", false},
	} {
		t.Run(tc.input, func(t *testing.T) {
			q, err := parse(mustValues("dataset=all&" + tc.input))
			if err != nil {
				t.Fatal(err)
			}
			sql, _ := paradeSQL(q, paradeEdition{ID: 1, Key: "invented"}, paradePosition{})
			if strings.HasPrefix(sql, "WITH first_candidates AS MATERIALIZED (") != tc.want {
				t.Fatal("unexpected first-name plan scope")
			}
		})
	}
}

func TestParadeFirstResidualKeepsAllCandidatesBeforeFilteringAndPaging(t *testing.T) {
	for _, sort := range []string{"", "name", "edition"} {
		t.Run(sort, func(t *testing.T) {
			q, err := parse(mustValues("dataset=all&match=exact&surname=Invented&town=Imaginary&first_name=Mary+Jane&first_match=given&limit=2&sort=" + sort))
			if err != nil {
				t.Fatal(err)
			}
			after := paradePosition{ID: 17, Dataset: 1, Name: "invented", First: "mary jane", Edition: "2008-03"}
			d := paradeEdition{ID: 1, Key: "invented", Manifest: []byte(`{"edition":"invented-2008-03"}`)}
			sql, args := paradeSQL(q, d, after)
			parts := strings.Split(sql, ") SELECT ")
			if len(parts) != 2 {
				t.Fatal("missing first-name materialization")
			}
			if strings.Contains(parts[0], " LIMIT ") || strings.Contains(parts[0], " ORDER BY ") || !strings.Contains(parts[0], "town=$") || !strings.Contains(parts[0], "name=$") {
				t.Fatal("inner population capped or anchors lost")
			}
			if !strings.Contains(parts[1], "first_full >= $") || !strings.Contains(parts[1], "first_full < $") || !strings.Contains(parts[1], " OR ") || !strings.Contains(parts[1], " ORDER BY ") || !strings.Contains(parts[1], " LIMIT $") {
				t.Fatal("outer precision/order/limit lost")
			}
			if args[len(args)-1] != 2 {
				t.Fatal("wrong page size")
			}
			used := map[int]bool{}
			for _, m := range regexp.MustCompile(`\$(\d+)`).FindAllStringSubmatch(sql, -1) {
				i, _ := strconv.Atoi(m[1])
				used[i] = true
			}
			for i := 1; i <= len(args); i++ {
				if !used[i] {
					t.Fatal(fmt.Sprintf("unbound or unused parameter %d", i))
				}
			}
			if len(used) != len(args) {
				t.Fatal("parameter count mismatch")
			}
		})
	}
}
