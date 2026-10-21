package catalog

import (
	"context"
	_ "embed"
	"fmt"
	"sort"
)

//go:embed address_lookup.sql
var addressLookupSchema string

// Match a bounded national name/postcode candidate set using compact location
// arrays. A broad town never expands into every occurrence in that town here.
// Legacy catalogues without this optional projection keep the existing fallback.
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

func (a *API) indexedAddressCandidates(ctx context.Context, q query, candidates map[int16][]int32) (map[int16][]int32, bool, error) {
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
	available, err := a.addressLookupReady(ctx, datasets)
	if err != nil || !available {
		return nil, false, err
	}
	sourceDatasets := []int16{}
	sourceIDs := []int32{}
	for _, dataset := range datasets {
		for _, id := range candidates[dataset] {
			sourceDatasets = append(sourceDatasets, dataset)
			sourceIDs = append(sourceIDs, id)
		}
	}
	args := []any{sourceDatasets, sourceIDs}
	predicate := addressPredicate(q, &args)
	sql := fmt.Sprintf(`WITH sources AS MATERIALIZED (
  SELECT * FROM unnest($1::smallint[],$2::integer[]) s(dataset_id,id)),
 locations AS MATERIALIZED (SELECT l.id FROM address_locations l WHERE %s)
 SELECT s.dataset_id,s.id FROM sources s
 CROSS JOIN LATERAL (SELECT b.location_ids[(s.id-1)%%128+1] AS location_id
  FROM address_record_blocks b WHERE b.dataset_id=s.dataset_id
   AND b.block_id=(s.id-1)/128 OFFSET 0) b
 JOIN locations l ON l.id=b.location_id`, predicate)
	matched, _, err := a.addressPlanCandidates(ctx, sql, args)
	return matched, true, err
}
