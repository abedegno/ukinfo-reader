package catalog

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// Only a complete optional premises projection uses this path. Fetch a bounded
// ordered page of candidate IDs first, then share record reads across editions.
// Original displayed address fields still decide every match, including lookahead.
func (a *API) indexedPremisesItems(ctx context.Context, datasets []premisesDataset, start int, after premisesCursor, identity string, hash []byte, limit int, fingerprint string) ([]result, string, error) {
	items := []result{}
	last := after
	type candidate struct {
		rank    int
		dataset int16
		id      int32
	}
	for start < len(datasets) {
		needed := limit + 1 - len(items)
		args := []any{hash, needed}
		branches := []string{}
		for rank := start; rank < len(datasets); rank++ {
			id := int32(0)
			if datasets[rank].ID == after.Dataset {
				id = after.ID
			}
			args = append(args, datasets[rank].ID, id)
			branches = append(branches, fmt.Sprintf("(SELECT %d AS edition_rank,dataset_id,id FROM premises_keys WHERE dataset_id=$%d AND premises_key=$1 AND id>$%d ORDER BY id LIMIT $2)", rank, len(args)-1, len(args)))
		}
		rows, err := a.DB.Query(ctx, "SELECT edition_rank,dataset_id,id FROM ("+strings.Join(branches, " UNION ALL ")+") candidates ORDER BY edition_rank,id LIMIT $2", args...)
		if err != nil {
			return nil, "", err
		}
		positions := []candidate{}
		requested := map[int16][]int32{}
		for rows.Next() {
			var p candidate
			if err = rows.Scan(&p.rank, &p.dataset, &p.id); err != nil {
				break
			}
			if p.rank < start || p.rank >= len(datasets) || p.dataset != datasets[p.rank].ID || p.id < 1 {
				err = recordIntegrity("invalid premises candidate")
				break
			}
			positions = append(positions, p)
			requested[p.dataset] = append(requested[p.dataset], p.id)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return nil, "", err
		}
		if len(positions) == 0 {
			break
		}
		records, err := a.loadPageRecords(ctx, requested)
		if err != nil {
			return nil, "", err
		}
		for _, p := range positions {
			item := records[sourceRecordKey{p.dataset, p.id}]
			original, reason, err := premisesRecord(item.Record)
			if err != nil {
				return nil, "", err
			}
			if reason != "" || original.key() != identity {
				continue
			}
			if len(items) == limit {
				raw, _ := json.Marshal(last)
				return items, base64.RawURLEncoding.EncodeToString(raw), nil
			}
			item.Dataset = datasets[p.rank].Key
			items = append(items, item)
			last = premisesCursor{fingerprint, p.dataset, p.id}
		}
		if len(positions) < needed {
			break
		}
		// Advance over rejected keys too. The returned cursor stays on the last
		// accepted source, so a matching lookahead is never skipped on continuation.
		consumed := positions[len(positions)-1]
		start = consumed.rank
		after = premisesCursor{fingerprint, consumed.dataset, consumed.id}
	}
	return items, "", nil
}
