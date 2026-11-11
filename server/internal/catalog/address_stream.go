package catalog

import (
	"context"
	"fmt"
	"sort"
)

const addressStreamBatch = 1024

// About 512 KiB of source IDs per request, plus one sentinel. This is an access
// path budget, never a result cap: an unproven prefix keeps the full fallback.
const addressSourcePrefixLimit = 65536

// streamAddressPage filters a bounded ordered prefix through the published
// reverse lookup. Release source rows before querying locations, so even one
// connection or multiple APIs sharing a pool cannot deadlock on nested borrows.
func (a *API) streamAddressPage(ctx context.Context, f candidateFilters, limit int) (map[int16][]int32, bool, error) {
	sql, args := addressSourceSQL(f, addressCandidate{}, addressSourcePrefixLimit+1)
	rows, err := a.searchRows(ctx, "source_probe", sql, args, true)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	sources := make([]addressCandidate, 0, addressSourcePrefixLimit+1)
	var readErr error
	for len(sources) < addressSourcePrefixLimit+1 && rows.Next() {
		var c addressCandidate
		if readErr = rows.Scan(&c.dataset, &c.id); readErr != nil {
			break
		}
		sources = append(sources, c)
	}
	if readErr == nil {
		readErr = rows.Err()
	}
	rows.Close()
	if readErr == nil {
		readErr = rows.Err()
	}
	overflow := len(sources) > addressSourcePrefixLimit
	if overflow {
		sources = sources[:addressSourcePrefixLimit]
	}
	selected := make([]addressCandidate, 0, limit)
	page := func() map[int16][]int32 {
		out := map[int16][]int32{}
		for _, c := range selected {
			out[c.dataset] = append(out[c.dataset], c.id)
		}
		return out
	}
	for start := 0; start < len(sources); start += addressStreamBatch {
		candidates := map[int16][]int32{}
		for _, c := range sources[start:min(start+addressStreamBatch, len(sources))] {
			candidates[c.dataset] = append(candidates[c.dataset], c.id)
		}
		matched, ready, err := a.filterIndexedAddressCandidates(ctx, f.q, candidates, true)
		if err != nil {
			return nil, false, err
		}
		if !ready {
			return nil, false, fmt.Errorf("address reverse lookup became unavailable")
		}
		batch := make([]addressCandidate, 0, addressStreamBatch)
		for dataset, ids := range matched {
			for _, id := range ids {
				batch = append(batch, addressCandidate{dataset, id})
			}
		}
		sort.Slice(batch, func(i, j int) bool {
			return batch[i].id < batch[j].id || (batch[i].id == batch[j].id && batch[i].dataset < batch[j].dataset)
		})
		selected = append(selected, batch[:min(len(batch), limit-len(selected))]...)
		// A later source error cannot invalidate an already complete ordered prefix.
		if len(selected) == limit {
			return page(), true, nil
		}
	}
	if readErr != nil {
		return nil, false, readErr
	}
	return page(), !overflow, nil
}
