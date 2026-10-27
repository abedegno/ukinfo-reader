package catalog

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testParadeNamePopulation(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	a := NewAPI(db)
	for _, tc := range []struct {
		dataset, surname string
		wide             bool
	}{
		{"given-windows", "Givenwindow", false},
		{"given-windows", "Absentinvented", false},
		{"probe-boundary", "Boundaryprobe", true}, // Exactly 4,097 occurrences.
		{"large-address", "Packedbudget", false},  // 510 occurrences.
		{"large-address", "Fillerbudget", true},
	} {
		q, err := parse(mustValues("dataset=" + tc.dataset + "&surname=" + tc.surname + "&sort=name"))
		if err != nil {
			t.Fatal(err)
		}
		editions, err := searchEditions(ctx, db, q.Dataset)
		if err != nil {
			t.Fatal(err)
		}
		wide, err := a.paradeWideName(ctx, q, editions)
		if err != nil || wide != tc.wide {
			t.Fatal("wrong bounded name selection", tc.dataset, wide, err)
		}
	}
}
