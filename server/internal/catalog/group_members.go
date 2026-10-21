package catalog

import (
	"context"
	"encoding/hex"
	"slices"
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
	for _, dataset := range ordered {
		if len(pending) == 0 {
			break
		}
		rows, err := a.searchRows(ctx, "group_members", groupRepresentativeSQL, []any{dataset, pending}, false)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var g []byte
			var id int32
			if err = rows.Scan(&g, &id); err != nil {
				rows.Close()
				return nil, err
			}
			members[hex.EncodeToString(g)] = listingMember{dataset: dataset, id: id}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		remaining := [][]byte{}
		for _, g := range pending {
			if _, ok := members[hex.EncodeToString(g)]; !ok {
				remaining = append(remaining, g)
			}
		}
		pending = remaining
	}
	if len(pending) > 0 {
		return nil, recordIntegrity("missing grouped source")
	}
	return members, nil
}
