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

const groupMemberSQL = `SELECT group_key,count(*),min(id) FROM research_keys
 WHERE dataset_id=$1 AND group_key=ANY($2::bytea[]) GROUP BY group_key`

// Count every occurrence and resolve its canonical min(dataset,id) source in
// the same covered group-index pass. A scalar edition anchor prunes each probe;
// at most two reads overlap. Preserve the database's edition-key collation.
func (a *API) groupMembers(ctx context.Context, datasets []int16, groups [][]byte) (map[string]listingMember, error) {
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
			rows, err := a.searchRows(ctx, "group_members", groupMemberSQL, []any{d.id, groups}, false)
			if err != nil {
				return err
			}
			defer rows.Close()
			found := map[string]listingMember{}
			for rows.Next() {
				var key []byte
				var count int64
				var first int32
				if err = rows.Scan(&key, &count, &first); err != nil {
					return err
				}
				if len(key) != 32 || count < 1 || first < 1 {
					return recordIntegrity("invalid grouped occurrence count")
				}
				found[hex.EncodeToString(key)] = listingMember{dataset: d.id, id: first, count: count}
			}
			if err = rows.Err(); err != nil {
				return err
			}
			mu.Lock()
			for key, member := range found {
				m := totals[key]
				if m.dataset == 0 || member.dataset < m.dataset || member.dataset == m.dataset && member.id < m.id {
					m.dataset, m.id = member.dataset, member.id
				}
				m.count += member.count
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
