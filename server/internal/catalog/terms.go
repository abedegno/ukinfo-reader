package catalog

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// A bounded FIFO avoids an importer-sized Go map of the entire postcode/name
// vocabulary. Each COPY batch holds its own resolved IDs until it is written.
type termCache struct {
	values   map[string]int32
	ring     []string
	next     int
	capacity int
}

func newTermCache(capacity int) *termCache {
	return &termCache{values: map[string]int32{"": 0}, capacity: capacity}
}
func (c *termCache) put(key string, id int32) {
	if key == "" {
		return
	}
	if _, ok := c.values[key]; ok {
		return
	}
	if len(c.ring) < c.capacity {
		c.ring = append(c.ring, key)
	} else {
		delete(c.values, c.ring[c.next])
		c.ring[c.next] = key
		c.next = (c.next + 1) % c.capacity
	}
	c.values[key] = id
}
func (c *termCache) resolve(ctx context.Context, tx pgx.Tx, keys []string) (map[string]int32, error) {
	result := make(map[string]int32, len(keys))
	missing := []string{}
	seen := map[string]bool{}
	for _, key := range keys {
		if id, ok := c.values[key]; ok {
			result[key] = id
		} else if !seen[key] {
			seen[key] = true
			missing = append(missing, key)
		}
	}
	if len(missing) == 0 {
		return result, nil
	}
	// No no-op UPDATE: repeated terms must not generate dead tuples or needless WAL.
	if _, e := tx.Exec(ctx, "INSERT INTO terms(key) SELECT unnest($1::text[]) ON CONFLICT(key) DO NOTHING", missing); e != nil {
		return nil, e
	}
	rows, e := tx.Query(ctx, "SELECT key,id FROM terms WHERE key=ANY($1::text[])", missing)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var id int32
		if e = rows.Scan(&key, &id); e != nil {
			return nil, e
		}
		result[key] = id
		c.put(key, id)
	}
	return result, rows.Err()
}
