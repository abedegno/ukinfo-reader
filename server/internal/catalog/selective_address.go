package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// This is a plan-selection threshold, never a limit on matching results. If the
// complete name candidate set exceeds it, retain the indexed address join.
const selectiveAddressLimit = 128

// matchSelectiveAddresses avoids expanding a whole town's address postings for
// a handful of name matches. Use the same displayed-field projection as the
// address index, then let SQL perform ordering, grouping and cursor pagination.
func (a *API) matchSelectiveAddresses(ctx context.Context, f *candidateFilters) error {
	q := f.q
	if f.missing || !q.hasAddress() || (q.Surname == "" && q.Organisation == "") {
		return nil
	}
	sql := "SELECT dataset_id,id FROM entries WHERE " + strings.Join(f.conditions, " AND ") + fmt.Sprintf(" LIMIT %d", selectiveAddressLimit+1)
	if len(f.termCTEs) != 0 {
		sql = "WITH " + strings.Join(f.termCTEs, ", ") + " " + sql
	}
	rows, err := a.DB.Query(ctx, sql, f.args...)
	if err != nil {
		return err
	}
	candidates := map[int16][]int32{}
	count := 0
	for rows.Next() {
		var dataset int16
		var id int32
		if err = rows.Scan(&dataset, &id); err != nil {
			break
		}
		candidates[dataset] = append(candidates[dataset], id)
		count++
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil || count > selectiveAddressLimit {
		return err
	}
	conditions := []string{}
	for _, dataset := range f.datasetIDs {
		records, err := a.loadRecords(ctx, dataset, candidates[dataset])
		if err != nil {
			return err
		}
		ids := []int32{}
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
				ids = append(ids, int32(record.ID))
			}
		}
		if len(ids) != 0 {
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
	return nil
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
