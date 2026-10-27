package catalog

import (
	"context"
	"encoding/hex"
	"slices"
	"sync"

	"golang.org/x/sync/errgroup"
)

type listingMember struct {
	dataset  int16
	id       int32
	count    int64
	editions []string
}

// Resolve only the representative, not every occurrence across all editions.
// A scalar dataset anchor prunes partitions even in a generic prepared plan;
// each exact group-key probe stops at its smallest source ordinal.
const groupRepresentativeSQL = `SELECT wanted.group_key,found.id
 FROM unnest($2::bytea[]) wanted(group_key)
 CROSS JOIN LATERAL (
  SELECT id FROM research_keys
  WHERE dataset_id=$1 AND group_key=wanted.group_key
  ORDER BY id LIMIT 1
 ) found`

func (a *API) groupRepresentatives(ctx context.Context, datasets []int16, groups [][]byte) (map[string]listingMember, error) {
	members := map[string]listingMember{}
	ordered := slices.Clone(datasets)
	slices.Sort(ordered)
	pending := slices.Clone(groups)
	for start := 0; start < len(ordered) && len(pending) > 0; {
		// Preserve the common one-probe early exit. If the first edition has no
		// representative, overlap subsequent edition pairs without exceeding
		// the same source pool. Merge by min(dataset,id), never completion order.
		end := min(start+2, len(ordered))
		if start == 0 {
			end = 1
		}
		group, probeCtx := errgroup.WithContext(ctx)
		var mu sync.Mutex
		for _, dataset := range ordered[start:end] {
			group.Go(func() error {
				rows, err := a.searchRows(probeCtx, "group_members", groupRepresentativeSQL, []any{dataset, pending}, false)
				if err != nil {
					return err
				}
				defer rows.Close()
				found := map[string]listingMember{}
				for rows.Next() {
					var g []byte
					var id int32
					if err = rows.Scan(&g, &id); err != nil {
						return err
					}
					found[hex.EncodeToString(g)] = listingMember{dataset: dataset, id: id}
				}
				if err = rows.Err(); err != nil {
					return err
				}
				mu.Lock()
				for key, member := range found {
					old, ok := members[key]
					if !ok || member.dataset < old.dataset || member.dataset == old.dataset && member.id < old.id {
						members[key] = member
					}
				}
				mu.Unlock()
				return nil
			})
		}
		if err := group.Wait(); err != nil {
			return nil, err
		}
		remaining := [][]byte{}
		for _, g := range pending {
			if _, ok := members[hex.EncodeToString(g)]; !ok {
				remaining = append(remaining, g)
			}
		}
		pending = remaining
		start = end
	}
	if len(pending) > 0 {
		return nil, recordIntegrity("missing grouped source")
	}
	return members, nil
}

const groupOccurrenceSQL = `SELECT group_key,count(*) FROM research_keys
 WHERE dataset_id=$1 AND group_key=ANY($2::bytea[]) GROUP BY group_key`

// Count every occurrence, including later editions than the representative. A
// scalar edition anchor prunes each probe; at most two reads overlap. Preserve
// the database's edition-key collation rather than substituting Go byte order.
func (a *API) groupOccurrences(ctx context.Context, datasets []int16, groups [][]byte) (map[string]listingMember, error) {
	type edition struct {
		id  int16
		key string
	}
	editions := []edition{}
	rows, err := a.searchRows(ctx, "group_members", "SELECT id,key FROM datasets WHERE id=ANY($1::smallint[]) ORDER BY key", []any{datasets}, false)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d edition
		if err = rows.Scan(&d.id, &d.key); err != nil {
			rows.Close()
			return nil, err
		}
		editions = append(editions, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	ranks := map[string]int{}
	for i, d := range editions {
		ranks[d.key] = i
	}
	totals := map[string]listingMember{}
	var mu sync.Mutex
	group, ctx := errgroup.WithContext(ctx)
	group.SetLimit(2)
	for _, d := range editions {
		group.Go(func() error {
			rows, err := a.searchRows(ctx, "group_members", groupOccurrenceSQL, []any{d.id, groups}, false)
			if err != nil {
				return err
			}
			defer rows.Close()
			found := map[string]int64{}
			for rows.Next() {
				var key []byte
				var count int64
				if err = rows.Scan(&key, &count); err != nil {
					return err
				}
				if len(key) != 32 || count < 1 {
					return recordIntegrity("invalid grouped occurrence count")
				}
				found[hex.EncodeToString(key)] = count
			}
			if err = rows.Err(); err != nil {
				return err
			}
			mu.Lock()
			for key, count := range found {
				m := totals[key]
				m.count += count
				m.editions = append(m.editions, d.key)
				totals[key] = m
			}
			mu.Unlock()
			return nil
		})
	}
	if err = group.Wait(); err != nil {
		return nil, err
	}
	for _, key := range groups {
		if totals[hex.EncodeToString(key)].count < 1 {
			return nil, recordIntegrity("missing grouped occurrence count")
		}
	}
	for key, m := range totals {
		slices.SortFunc(m.editions, func(a, b string) int { return ranks[a] - ranks[b] })
		totals[key] = m
	}
	return totals, nil
}
