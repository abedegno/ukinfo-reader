package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/abedegno/ukinfo-reader/server/internal/telemetry"
	"github.com/jackc/pgx/v5"
	"sort"
	"strings"
)

// Budgets select an access path; an overflow always retains the complete SQL
// intersection. Many adjacent occurrences can share one compressed source block.
const selectiveAddressLimit = 4096
const selectiveAddressBlocks = 128
const selectivePostingsLimit = 1024

func (a *API) matchSelectiveAddresses(ctx context.Context, f *candidateFilters) (err error) {
	ctx, span := telemetry.Operation(ctx, "address.select")
	defer func() { telemetry.End(span, err) }()
	q := f.q
	if f.missing || !q.hasAddress() {
		return nil
	}
	ready, readyErr := a.placePostingsReady(ctx, *f)
	if readyErr != nil {
		return readyErr
	}
	if ready {
		f.placePostings = true
		a.metrics.plan("place_postings")
		return nil
	}
	plan := "fallback"
	sourceOverflow := false
	defer func() {
		if err != nil {
			a.metrics.plan("error")
		} else {
			if plan == "fallback" && f.addressLookup && !f.addressMatched && q.Group == "" && q.Sort == "" {
				plan = "reverse_intersection"
			}
			a.metrics.plan(plan)
		}
	}()
	// With a complete reverse lookup, try the independent indexed anchor before
	// walking a broad city's street postings. Overflow retains the full fallback.
	if q.Surname != "" || q.Organisation != "" || q.Postcode != "" || q.Near != "" || q.hasBusinessFilters() {
		ready, err := a.addressLookupReady(ctx, f.datasetIDs)
		if err != nil {
			return err
		}
		if ready {
			f.addressLookup = a.ExperimentalAddressProbe
			sourceSQL := "SELECT dataset_id,id FROM entries WHERE " + strings.Join(f.conditions, " AND ") + fmt.Sprintf(" LIMIT %d", selectiveAddressLimit+1)
			if len(f.termCTEs) > 0 {
				sourceSQL = "WITH " + strings.Join(f.termCTEs, ",") + " " + sourceSQL
			}
			candidates, count, err := a.addressPlanCandidates(ctx, "source_probe", sourceSQL, f.args)
			if err != nil {
				return err
			}
			sourceOverflow = count > selectiveAddressLimit
			if !sourceOverflow {
				matched, available, err := a.indexedAddressCandidates(ctx, q, candidates)
				if err != nil {
					return err
				}
				if available {
					plan = "indexed_source"
					f.useAddressCandidates(matched)
					return nil
				}
			}
		}
	}
	// First try the compact address side. LIMIT bounds the probe, never the result:
	// only a completely exhausted probe may replace the address intersection.
	args := []any{f.datasetIDs}
	predicate := addressPredicate(q, &args)
	sql := fmt.Sprintf("SELECT m.dataset_id,candidate FROM address_locations l JOIN address_members m ON m.location_id=l.id CROSS JOIN LATERAL unnest(m.ids) candidate WHERE m.dataset_id=ANY($1::smallint[]) AND %s LIMIT %d", predicate, selectivePostingsLimit+1)
	candidates, count, err := a.addressPlanCandidates(ctx, "address_probe", sql, args)
	if err != nil {
		return err
	}
	if count <= selectivePostingsLimit {
		f.useAddressCandidates(candidates)
		plan = "address"
		if !q.hasRefinements() || f.missing {
			return nil
		}
	}
	// A source-side probe is useful only with an independent indexed anchor.
	if !f.addressMatched && q.Surname == "" && q.Organisation == "" && q.Postcode == "" && q.Near == "" && !q.hasBusinessFilters() {
		return nil
	}
	// The first source probe already proved overflow. Only repeat it if a
	// complete address set has changed its predicate; otherwise it is identical.
	if sourceOverflow && !f.addressMatched {
		return nil
	}
	sql = "SELECT dataset_id,id FROM entries WHERE " + strings.Join(f.conditions, " AND ") + fmt.Sprintf(" LIMIT %d", selectiveAddressLimit+1)
	if len(f.termCTEs) > 0 {
		sql = "WITH " + strings.Join(f.termCTEs, ",") + " " + sql
	}
	candidates, count, err = a.addressPlanCandidates(ctx, "source_probe", sql, f.args)
	if err != nil || count > selectiveAddressLimit {
		return err
	}
	blocks := map[recordBlockKey]bool{}
	for dataset, ids := range candidates {
		for _, id := range ids {
			blocks[recordBlockKey{dataset, (id - 1) / blockRows}] = true
		}
	}
	if len(blocks) > selectiveAddressBlocks {
		matched, available, err := a.indexedAddressCandidates(ctx, q, candidates)
		if err != nil {
			return err
		}
		if available {
			plan = "indexed_source"
			f.useAddressCandidates(matched)
		}
		return nil
	}
	matched := map[int16][]int32{}
	for _, dataset := range f.datasetIDs {
		ids := candidates[dataset]
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		// Keep record allocations bounded even for the maximum 64-KiB source row.
		for start := 0; start < len(ids); start += 100 {
			records, err := a.loadRecords(ctx, dataset, ids[start:min(start+100, len(ids))])
			if err != nil {
				return err
			}
			for _, record := range records {
				var source addressSource
				if err = json.Unmarshal(record.Record, &source); err != nil {
					return err
				}
				location, err := source.project()
				if err != nil {
					return err
				}
				if location.matches(q) {
					accepted := true
					if q.hasRefinements() {
						accepted, err = q.accepts(record.Record)
						if err != nil {
							return err
						}
					}
					if accepted {
						matched[dataset] = append(matched[dataset], int32(record.ID))
					}
				}
			}
		}
	}
	plan = "source"
	f.useAddressCandidates(matched)
	return nil
}

func (a *API) addressPlanCandidates(ctx context.Context, phase, sql string, args []any) (map[int16][]int32, int, error) {
	rows, err := a.searchRows(ctx, phase, sql, args, false)
	if err != nil {
		return nil, 0, err
	}
	return readAddressCandidates(rows)
}

func readAddressCandidates(rows pgx.Rows) (map[int16][]int32, int, error) {
	defer rows.Close()
	candidates := map[int16][]int32{}
	count := 0
	for rows.Next() {
		var dataset int16
		var id int32
		if err := rows.Scan(&dataset, &id); err != nil {
			return nil, 0, err
		}
		candidates[dataset] = append(candidates[dataset], id)
		count++
	}
	return candidates, count, rows.Err()
}

func (f *candidateFilters) useAddressCandidates(candidates map[int16][]int32) {
	conditions := []string{}
	for _, dataset := range f.datasetIDs {
		if ids := candidates[dataset]; len(ids) > 0 {
			f.args = append(f.args, dataset, ids)
			conditions = append(conditions, fmt.Sprintf("(dataset_id=$%d AND id=ANY($%d::integer[]))", len(f.args)-1, len(f.args)))
		}
	}
	f.addressMatched = true
	if len(conditions) == 0 {
		f.missing = true
	} else {
		f.conditions = append(f.conditions, "("+strings.Join(conditions, " OR ")+")")
	}
}

func (l addressLocation) matches(q query) bool {
	for _, filter := range []struct {
		value  string
		fields []int
	}{{q.Street, []int{0, 1}}, {q.Locality, []int{2, 3}}, {q.Town, []int{4}}, {q.County, []int{5}}} {
		if filter.value == "" {
			continue
		}
		matched := false
		for _, field := range filter.fields {
			matched = matched || textMatches(l[field], filter.value)
		}
		if !matched {
			return false
		}
	}
	return true
}
