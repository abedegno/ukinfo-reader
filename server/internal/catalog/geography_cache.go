package catalog

import (
	"context"
	"sync"
)

// Cache only postcode coordinates/term IDs, never directory records or names.
// Releases are immutable; new catalogue imports invalidate links through generation.
type geographyKey struct {
	release, near string
	radius        int
	generation    int16
}
type geographyEntry struct {
	ready  chan struct{}
	points map[int32]postcodeDistance
	info   *GeographySearch
	err    error
}
type geographyCache struct {
	mu      sync.Mutex
	entries map[geographyKey]*geographyEntry
	queue   []geographyKey
	points  int
}

func newGeographyCache() *geographyCache {
	return &geographyCache{entries: map[geographyKey]*geographyEntry{}}
}
func (c *geographyCache) lookup(ctx context.Context, key geographyKey, load func() (map[int32]postcodeDistance, *GeographySearch, error)) (map[int32]postcodeDistance, *GeographySearch, error) {
	c.mu.Lock()
	if entry, ok := c.entries[key]; ok {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-entry.ready:
			return entry.points, entry.info, entry.err
		}
	}
	entry := &geographyEntry{ready: make(chan struct{})}
	c.entries[key] = entry
	c.mu.Unlock()
	points, info, err := load()
	c.mu.Lock()
	entry.points, entry.info, entry.err = points, info, err
	if err != nil {
		delete(c.entries, key)
	} else {
		c.queue = append(c.queue, key)
		c.points += len(points)
		// Eviction affects reuse, never query completeness. In-flight lookups remain
		// shareable; their waiting callers hold the entry even after eviction.
		for len(c.queue) > 8 || c.points > 500000 {
			old := c.queue[0]
			c.queue = c.queue[1:]
			c.points -= len(c.entries[old].points)
			delete(c.entries, old)
		}
	}
	close(entry.ready)
	c.mu.Unlock()
	return points, info, err
}
