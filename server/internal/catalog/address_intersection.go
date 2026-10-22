package catalog

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// One maximum API page plus lookahead (101) fits in this small probe.
const addressIntersectionBatch = 128

type addressCandidate struct {
	dataset int16
	id      int32
}

// reverseAddressPage tries one bounded source-ID prefix. A complete page plus
// lookahead is exact because every unseen source follows this prefix. Exhausting
// the source is also exact. Otherwise retain the complete posting intersection;
// walking many sparse reverse batches can lose badly to a compact county range.
// The boolean indicates a proven page, never truncation of the search universe.
func (a *API) reverseAddressPage(ctx context.Context, f candidateFilters, limit int) (map[int16][]int32, bool, error) {
	sql, args := addressSourceBatchSQL(f, addressCandidate{})
	rows, err := a.searchRows(ctx, "source_probe", sql, args, true)
	if err != nil {
		return nil, false, err
	}
	candidates, count, err := readAddressCandidates(rows)
	if err != nil {
		return nil, false, err
	}
	if count == 0 {
		return map[int16][]int32{}, true, nil
	}
	// Planning checked complete immutable projections; still validate each block.
	matched, ready, err := a.filterIndexedAddressCandidates(ctx, f.q, candidates, true)
	if err != nil {
		return nil, false, err
	}
	if !ready {
		return nil, false, fmt.Errorf("address reverse lookup became unavailable")
	}
	selected := []addressCandidate{}
	for dataset, ids := range matched {
		for _, id := range ids {
			selected = append(selected, addressCandidate{dataset, id})
		}
	}
	if count == addressIntersectionBatch && len(selected) < limit {
		return nil, false, nil
	}
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].id < selected[j].id || (selected[i].id == selected[j].id && selected[i].dataset < selected[j].dataset)
	})
	selected = selected[:min(len(selected), limit)]
	out := map[int16][]int32{}
	for _, c := range selected {
		out[c.dataset] = append(out[c.dataset], c.id)
	}
	return out, true, nil
}

func addressSourceBatchSQL(f candidateFilters, after addressCandidate) (string, []any) {
	args := append([]any(nil), f.args...)
	args = append(args, after.id, after.dataset)
	seek := fmt.Sprintf("(id,dataset_id)>($%d,$%d)", len(args)-1, len(args))
	branches := []string{}
	ctes := append([]string(nil), f.termCTEs...)
	for _, dataset := range f.datasetIDs {
		args = append(args, dataset)
		name := fmt.Sprintf("address_batch_source_%d", dataset)
		// Fix the indexed source set before applying ID order. Otherwise a generic
		// plan can walk the source primary key, fetching heaps until a rare name hits.
		ctes = append(ctes, fmt.Sprintf("%s AS MATERIALIZED (SELECT dataset_id,id FROM entries WHERE %s AND dataset_id=$%d AND %s)", name, strings.Join(f.conditions, " AND "), len(args), seek))
		branches = append(branches, fmt.Sprintf("(SELECT dataset_id,id FROM %s ORDER BY id LIMIT %d)", name, addressIntersectionBatch))
	}
	sql := fmt.Sprintf("SELECT dataset_id,id FROM (%s) source_batch ORDER BY id,dataset_id LIMIT %d", strings.Join(branches, " UNION ALL "), addressIntersectionBatch)
	if len(ctes) > 0 {
		sql = "WITH " + strings.Join(ctes, ",") + " " + sql
	}
	return sql, args
}
