package catalog

import (
	"context"
	"errors"

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
	consume := func(rows pgx.Rows, err error) error {
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			var id int32
			if err = rows.Scan(&key, &id); err != nil {
				return err
			}
			result[key] = id
			c.put(key, id)
		}
		return rows.Err()
	}
	lookup := func(keys []string) error {
		return consume(tx.Query(ctx, "SELECT key,id FROM terms WHERE key=ANY($1::text[])", keys))
	}
	if e := lookup(missing); e != nil {
		return nil, e
	}
	unresolved := func(keys []string) []string {
		remaining := []string{}
		for _, key := range keys {
			if _, ok := result[key]; !ok {
				remaining = append(remaining, key)
			}
		}
		return remaining
	}
	fresh := unresolved(missing)
	if len(fresh) > 0 {
		// Check existing terms before INSERT: ON CONFLICT still consumes sequence
		// values. A bounded cache miss must not spend another identity for a term
		// already present from an earlier batch or edition. No no-op UPDATE/WAL.
		if e := consume(tx.Query(ctx, "INSERT INTO terms(key) SELECT unnest($1::text[]) ON CONFLICT(key) DO NOTHING RETURNING key,id", fresh)); e != nil {
			return nil, e
		}
		// The import advisory lock serializes normal writers. Resolve a conflict
		// from a separate manual writer safely under READ COMMITTED as well.
		if remaining := unresolved(fresh); len(remaining) > 0 {
			if e := lookup(remaining); e != nil {
				return nil, e
			}
			if len(unresolved(remaining)) > 0 {
				return nil, errors.New("unresolved search vocabulary")
			}
		}
	}
	return result, nil
}
