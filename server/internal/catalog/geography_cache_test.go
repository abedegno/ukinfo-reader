package catalog

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestGeographyCache(t *testing.T) {
	cache := newGeographyCache()
	var calls atomic.Int32
	load := func() (map[int32]postcodeDistance, *GeographySearch, error) {
		calls.Add(1)
		return map[int32]postcodeDistance{1: {Distance: 12}}, &GeographySearch{}, nil
	}
	key := geographyKey{"2026-08", "AA11AA", 1000, 1}
	var group sync.WaitGroup
	for i := 0; i < 20; i++ {
		group.Go(func() {
			points, _, err := cache.lookup(context.Background(), key, load)
			if err != nil || points[1].Distance != 12 {
				t.Error("cached coordinates", points, err)
			}
		})
	}
	group.Wait()
	if calls.Load() != 1 {
		t.Fatal("duplicate coordinate lookup", calls.Load())
	}
	// New imports change the generation even when the ONSPD release is unchanged.
	key.generation = 2
	if _, _, err := cache.lookup(context.Background(), key, load); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("new catalogue generation reused stale postcode links")
	}
	for i := 0; i < 9; i++ {
		key.near = fmt.Sprint(i)
		if _, _, err := cache.lookup(context.Background(), key, load); err != nil {
			t.Fatal(err)
		}
	}
	cache.mu.Lock()
	entries := len(cache.entries)
	cache.mu.Unlock()
	if entries > 8 {
		t.Fatal("unbounded coordinate cache", entries)
	}
	before := calls.Load()
	key.near = "AA11AA"
	if _, _, err := cache.lookup(context.Background(), key, load); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != before+1 {
		t.Fatal("old lookup was not evicted")
	}
}
