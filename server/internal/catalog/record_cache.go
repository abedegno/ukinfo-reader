package catalog

import (
	"context"
	"sync"
)

const requestBlockCacheBytes = 4 << 20

type recordBlockKey struct {
	dataset int16
	block   int32
}
type recordCacheContextKey struct{}

// Private to one HTTP request: no directory records are shared between visitors
// or retained after a request. The byte budget bounds decoded blocks, not just keys.
type requestBlockCache struct {
	mu      sync.Mutex
	blocks  map[recordBlockKey][]byte
	queue   []recordBlockKey
	bytes   int
	planKey string
	plan    *candidateFilters
	observe *apiMetrics
}

func cloneCandidateFilters(f candidateFilters) candidateFilters {
	f.args = append([]any(nil), f.args...)
	f.conditions = append([]string(nil), f.conditions...)
	f.termCTEs = append([]string(nil), f.termCTEs...)
	return f
}
func (c *requestBlockCache) getPlan(key string) (candidateFilters, bool) {
	if c == nil {
		return candidateFilters{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.plan == nil || c.planKey != key {
		return candidateFilters{}, false
	}
	return cloneCandidateFilters(*c.plan), true
}
func (c *requestBlockCache) putPlan(key string, f candidateFilters) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	f = cloneCandidateFilters(f)
	c.planKey = key
	c.plan = &f
}

func withRecordCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, recordCacheContextKey{}, &requestBlockCache{blocks: map[recordBlockKey][]byte{}})
}
func recordCache(ctx context.Context) *requestBlockCache {
	c, _ := ctx.Value(recordCacheContextKey{}).(*requestBlockCache)
	return c
}
func (c *requestBlockCache) get(key recordBlockKey) []byte {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	raw := c.blocks[key]
	if c.observe != nil {
		if raw != nil {
			c.observe.cacheHits.Add(1)
		} else {
			c.observe.cacheMisses.Add(1)
		}
	}
	return raw
}
func (c *requestBlockCache) put(key recordBlockKey, raw []byte) {
	if c == nil || len(raw) > requestBlockCacheBytes {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.blocks[key]; ok {
		return
	}
	for (c.bytes+len(raw) > requestBlockCacheBytes || len(c.queue) >= 128) && len(c.queue) > 0 {
		old := c.queue[0]
		c.queue = c.queue[1:]
		c.bytes -= len(c.blocks[old])
		delete(c.blocks, old)
	}
	c.blocks[key] = raw
	c.queue = append(c.queue, key)
	c.bytes += len(raw)
}
