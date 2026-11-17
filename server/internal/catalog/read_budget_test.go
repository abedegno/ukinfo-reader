package catalog

import (
	"context"
	"testing"
	"time"
)

func TestSearchRequestBudgets(t *testing.T) {
	for _, tc := range []struct {
		name string
		q    query
		want time.Duration
	}{
		{"basic", query{Postcode: "AA1"}, 3 * time.Second},
		{"address", query{Town: "Invented"}, 5 * time.Second},
		{"grouped", query{Group: "listing"}, 5 * time.Second},
		{"ordered", query{Sort: "name"}, 5 * time.Second},
		{"nearby", query{Near: "AA11AA"}, 5 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := time.Now()
			ctx, cancel := searchRequestContext(context.Background(), tc.q)
			defer cancel()
			deadline, ok := ctx.Deadline()
			if !ok || deadline.Sub(before) < tc.want-100*time.Millisecond || deadline.Sub(before) > tc.want+100*time.Millisecond {
				t.Fatalf("unexpected request deadline: %v", deadline.Sub(before))
			}
		})
	}
}
func TestSearchRequestPreservesEarlierCallerDeadline(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	child, stop := searchRequestContext(parent, query{})
	defer stop()
	a, _ := parent.Deadline()
	b, _ := child.Deadline()
	if !a.Equal(b) {
		t.Fatal("caller deadline extended")
	}
	cancel()
	select {
	case <-child.Done():
	default:
		t.Fatal("caller cancellation not propagated")
	}
}
