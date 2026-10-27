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
	key := geographyKey{release: "2026-08", near: "AA11AA", radius: 1000, generation: 1}
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
	// Compact W11 can mean either W11 district or W1 1 sector.
	key.near = "W11"
	key.nearLength = 6
	_, _, _ = cache.lookup(context.Background(), key, load)
	beforeBoundary := calls.Load()
	key.nearLength = 5
	_, _, _ = cache.lookup(context.Background(), key, load)
	if calls.Load() != beforeBoundary+1 {
		t.Fatal("district and sector shared a centre cache entry")
	}
	// A cached centre has no full-radius postcode map and must never satisfy a
	// native radius lookup, or vice versa.
	beforeCentre := calls.Load()
	key.centreOnly = true
	_, _, _ = cache.lookup(context.Background(), key, load)
	if calls.Load() != beforeCentre+1 {
		t.Fatal("centre-only lookup reused a full-radius cache entry")
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
