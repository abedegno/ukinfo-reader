package catalog

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestParadeHybridRouting(t *testing.T) {
	for _, tc := range []struct {
		query string
		want  bool
	}{
		{"surname=Invented", false},
		{"surname=Inv*&first_name=June", false},
		{"postcode=AA1", false},
		{"surname=Invented&postcode=AA1+1AA&sort=name", true},
		{"postcode=AA1&sort=name", true},
		{"postcode=AA1&sort=name&group=listing", false},
		{"surname=Invented&house=99", false},
		{"kind=business&sic=1234", false},
		{"kind=business&sic=1234&sort=name", true},
		{"kind=business&industry=Invented*&sort=name", true},
		{"kind=business&industry=Invented*&sort=name&group=listing", false},
		{"kind=business&industry=Invented*", false},
		{"town=Invented+Town", true},
		{"street=Invented*", true},
		{"locality=Invented", true},
		{"county=Invented", true},
		{"surname=Inv*&postcode=AA1&town=Invented&sort=name", true},
		{"kind=business&sic=1234&town=Invented", true},
		{"surname=Invented&town=Invented&house=99", true},
		{"town=Invented&group=listing", true},
		{"town=Invented&near=AA1+1AA", true},
		{"near=AA1+1AA", false},
		{"town=Invented&near=AA1+1AA&sort=nearest", false},
		{"town=Invented&near=AA1+1AA&group=listing", false},
		{"town=Invented&sort=edition", true},
		{"town=Invented&group=listing&sort=edition", false},
	} {
		t.Run(tc.query, func(t *testing.T) {
			q, err := parse(mustValues("dataset=all&" + tc.query))
			if err != nil {
				t.Fatal(err)
			}
			if paradeEligible(q) != tc.want {
				t.Fatal("incorrect backend selection")
			}
			backend, err := ParadeTrialBackend(ParadeTrialCase{Path: "/api/search", Query: "dataset=all&" + tc.query})
			if err != nil || (backend == "paradedb") != tc.want {
				t.Fatal("operator metadata disagrees with serving")
			}
			if !tc.want {
				// Neither pool is usable. Native routing must return before touching
				// either inventory or the target, regardless of target availability.
				a := NewAPI(nil)
				a.ParadeDB = &pgxpool.Pool{}
				_, _, handled, err := a.paradeResults(context.Background(), q)
				if err != nil || handled {
					t.Fatal("native query touched optional backend")
				}
			}
		})
	}
}
