package catalog

import "testing"

func TestParadeNamePrefixScope(t *testing.T) {
	base := query{Dataset: "all", Kind: "person", Surname: "inv*", First: "june", FirstMatch: "given", Match: "exact"}
	if !paradeNamePrefix(base) {
		t.Fatal("measured prefix shape excluded")
	}
	for _, change := range []func(*query){
		func(q *query) { q.Dataset = "invented" }, func(q *query) { q.Kind = "business" },
		func(q *query) { q.Surname = "invented" }, func(q *query) { q.First = "june adele" },
		func(q *query) { q.First = "june\tadele" }, func(q *query) { q.First = "june*" },
		func(q *query) { q.FirstMatch = "exact" }, func(q *query) { q.FirstMatch = "prefix" },
		func(q *query) { q.Sort = "name" }, func(q *query) { q.Group = "listing" },
		func(q *query) { q.Postcode = "AA1" }, func(q *query) { q.Near = "AA11AA" },
		func(q *query) { q.Street = "invented" }, func(q *query) { q.Town = "invented" },
		func(q *query) { q.Middle = "a" }, func(q *query) { q.House = "1" },
		func(q *query) { q.Building = "invented" }, func(q *query) { q.Flat = "1" },
		func(q *query) { q.Organisation = "invented" }, func(q *query) { q.SIC = "1234" },
	} {
		q := base
		change(&q)
		if paradeNamePrefix(q) {
			t.Fatal("unmeasured shape selected", q)
		}
	}
}
