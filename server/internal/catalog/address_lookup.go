package catalog

import (
	"context"
	_ "embed"
	"fmt"
	"sort"
)

//go:embed address_lookup.sql
var addressLookupSchema string

// addressLookupReady requires complete coverage and serving permissions. Legacy
// catalogues without this optional projection retain the complete fallback.
func (a *API) addressLookupReady(ctx context.Context, datasets []int16) (bool, error) {
	var available bool
	if err := a.DB.QueryRow(ctx, `SELECT COALESCE(
  has_table_privilege(current_user,to_regclass('address_record_indexes'),'SELECT')
  AND has_table_privilege(current_user,to_regclass('address_record_blocks'),'SELECT'),false)`).Scan(&available); err != nil || !available {
		return false, err
	}
	var ready int
	if err := a.DB.QueryRow(ctx, `SELECT count(*) FROM address_record_indexes i
  JOIN datasets d ON d.id=i.dataset_id AND d.entries=i.entries
  WHERE i.dataset_id=ANY($1::smallint[]) AND i.version=1
   AND i.blocks=(d.entries+127)/128`, datasets).Scan(&ready); err != nil {
		return false, err
	}
	return ready == len(datasets), nil
}

// indexedAddressCandidates filters bounded source candidates through compact
// address arrays, without expanding every occurrence in a broad town.
func (a *API) indexedAddressCandidates(ctx context.Context, q query, candidates map[int16][]int32) (map[int16][]int32, bool, error) {
	return a.filterIndexedAddressCandidates(ctx, q, candidates, false)
}

func (a *API) filterIndexedAddressCandidates(ctx context.Context, q query, candidates map[int16][]int32, knownReady bool) (map[int16][]int32, bool, error) {
	datasets := []int16{}
	for dataset, ids := range candidates {
		if len(ids) > 0 {
			datasets = append(datasets, dataset)
		}
	}
	sort.Slice(datasets, func(i, j int) bool { return datasets[i] < datasets[j] })
	if len(datasets) == 0 {
		return map[int16][]int32{}, true, nil
	}
	if !knownReady {
		available, err := a.addressLookupReady(ctx, datasets)
		if err != nil || !available {
			return nil, false, err
		}
	}
	// Read each edition's blocks in one bounded batch. Individual lateral index
	// probes alternate index/heap reads on HDDs while a long import prevents the
	// visibility map from marking newly built lookups all-visible.
	type sourceLocation struct {
		dataset  int16
		id       int32
		location int32
	}
	sources := []sourceLocation{}
	locationIDs := []int32{}
	for _, dataset := range datasets {
		byBlock := map[int32][]int32{}
		for _, id := range candidates[dataset] {
			byBlock[(id-1)/blockRows] = append(byBlock[(id-1)/blockRows], id)
		}
		blocks := make([]int32, 0, len(byBlock))
		for block := range byBlock {
			blocks = append(blocks, block)
		}
		sort.Slice(blocks, func(i, j int) bool { return blocks[i] < blocks[j] })
		rows, err := a.searchRows(ctx, "address_blocks", `SELECT block_id,location_ids FROM address_record_blocks
 WHERE dataset_id=$1 AND block_id=ANY($2::integer[]) ORDER BY block_id`, []any{dataset, blocks}, false)
		if err != nil {
			return nil, true, err
		}
		seen := 0
		for rows.Next() {
			var block int32
			var locations []int32
			if err = rows.Scan(&block, &locations); err != nil {
				rows.Close()
				return nil, true, err
			}
			seen++
			for _, id := range byBlock[block] {
				slot := int((id - 1) % blockRows)
				if slot >= len(locations) || locations[slot] < 1 {
					rows.Close()
					return nil, true, fmt.Errorf("incomplete address record lookup")
				}
				sources = append(sources, sourceLocation{dataset, id, locations[slot]})
				locationIDs = append(locationIDs, locations[slot])
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, true, err
		}
		if seen != len(blocks) {
			return nil, true, fmt.Errorf("incomplete address record lookup")
		}
	}
	args := []any{locationIDs}
	sql := addressLocationLookupSQL(q, &args)
	rows, err := a.searchRows(ctx, "address_dictionary", sql, args, false)
	if err != nil {
		return nil, true, err
	}
	locations := map[int32]bool{}
	for rows.Next() {
		var id int32
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, true, err
		}
		locations[id] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, true, err
	}
	matched := map[int16][]int32{}
	for _, source := range sources {
		if locations[source.location] {
			matched[source.dataset] = append(matched[source.dataset], source.id)
		}
	}
	return matched, true, nil
}

func addressLocationLookupSQL(q query, args *[]any) string {
	predicate := addressPredicate(q, args)
	sql := "SELECT l.id FROM address_locations l WHERE l.id=ANY($1::integer[]) AND " + predicate
	if q.Street == "" && q.Locality == "" {
		// Town/county covering indexes can read the complete dictionary range
		// without scattered address heap fetches. Materialize before intersecting
		// candidate IDs, otherwise the planner may prefer thousands of PK probes.
		sql = "WITH locations AS MATERIALIZED (SELECT l.id FROM address_locations l WHERE " + predicate +
			") SELECT id FROM locations WHERE id IN (SELECT unnest($1::integer[]))"
	}
	return sql
}
