package catalog

import (
	"context"
	"fmt"
	"strings"
)

// This private experiment never changes the default access path. The complete
// fallback also handles absent editions, street/locality filters and ordering.
func (a *API) placePostingsReady(ctx context.Context, f candidateFilters) (bool, error) {
	q := f.q
	if !a.ExperimentalPlacePostings || q.Sort != "" || q.Group != "" || q.Street != "" || q.Locality != "" || (q.Town == "" && q.County == "") {
		return false, nil
	}
	var available bool
	if err := a.DB.QueryRow(ctx, `SELECT CASE WHEN COALESCE(has_schema_privilege(current_user,to_regnamespace('ukinfo_place_spike'),'USAGE'),false) THEN COALESCE(has_table_privilege(current_user,to_regclass('ukinfo_place_spike.build'),'SELECT') AND has_table_privilege(current_user,to_regclass('ukinfo_place_spike.postings'),'SELECT'),false) ELSE false END`).Scan(&available); err != nil || !available {
		return false, err
	}
	var n int
	err := a.DB.QueryRow(ctx, `SELECT count(*) FROM ukinfo_place_spike.build b JOIN datasets d ON d.id=b.dataset_id AND d.entries=b.expected WHERE b.dataset_id=ANY($1::smallint[]) AND b.chunk_rows=8192 AND b.finished_at IS NOT NULL`, f.datasetIDs).Scan(&n)
	return n == len(f.datasetIDs), err
}

func placePostingSQL(value string, field, datasetArg, afterArg int, args *[]any) string {
	key := strings.TrimSuffix(value, "*")
	*args = append(*args, key)
	predicate := fmt.Sprintf("key=$%d", len(*args))
	if strings.HasSuffix(value, "*") {
		*args = append(*args, key+"\U0010ffff")
		predicate = fmt.Sprintf("key >= $%d AND key < $%d", len(*args)-1, len(*args))
	}
	return fmt.Sprintf("SELECT chunk,ids FROM ukinfo_place_spike.postings WHERE dataset_id=$%d AND field=%d AND %s AND chunk>=GREATEST(0,($%d::integer-1)/8192)", datasetArg, field, predicate, afterArg)
}

// Each ID occurs in at most one key per field. Prefix ranges therefore preserve
// uniqueness; town and county are intersected, never combined as an OR.
func placePostingIDs(q query, dataset int16, datasetArg, afterArg int, args *[]any, ctes *[]string, source string) string {
	last := source
	fields := []struct {
		value string
		field int
	}{{q.Town, 1}, {q.County, 2}}
	for _, f := range fields {
		if f.value == "" {
			continue
		}
		postings := fmt.Sprintf("place_%d_%d", dataset, f.field)
		*ctes = append(*ctes, postings+" AS MATERIALIZED ("+placePostingSQL(f.value, f.field, datasetArg, afterArg, args)+")")
		next := fmt.Sprintf("place_matches_%d_%d", dataset, f.field)
		if last == "" {
			*ctes = append(*ctes, fmt.Sprintf("%s AS MATERIALIZED (SELECT unnest(ids) AS id FROM %s)", next, postings))
		} else {
			*ctes = append(*ctes, fmt.Sprintf("%s AS MATERIALIZED (SELECT s.id FROM %s s JOIN %s p ON p.chunk=(s.id-1)/8192 AND s.id=ANY(p.ids))", next, last, postings))
		}
		last = next
	}
	return fmt.Sprintf("SELECT id AS candidate FROM %s WHERE id >= $%d", last, afterArg)
}

func placePostingBranchSQL(f candidateFilters, dataset int16, datasetArg, afterArg, limitArg int, conditions []string, args *[]any, ctes *[]string) string {
	q := f.q
	source := ""
	if q.Kind == "business" || q.Surname != "" || q.First != "" || q.Organisation != "" || q.Postcode != "" || q.Near != "" || q.hasBusinessFilters() {
		source = fmt.Sprintf("place_source_%d", dataset)
		*ctes = append(*ctes, fmt.Sprintf("%s AS MATERIALIZED (SELECT id FROM entries WHERE %s AND dataset_id=$%d)", source, strings.Join(conditions, " AND "), datasetArg))
	}
	ids := placePostingIDs(q, dataset, datasetArg, afterArg, args, ctes, source)
	innerLimit := ""
	if source != "" {
		innerLimit = fmt.Sprintf(" LIMIT $%d", limitArg)
	}
	// Without an independent anchor, kind/cursor filters must precede LIMIT;
	// interleaved businesses must never shorten a person page (or vice versa).
	return fmt.Sprintf(`(SELECT e.* FROM (%s ORDER BY candidate%s) chosen
 CROSS JOIN LATERAL (SELECT dataset_id,id,name_id,first_id,postcode_id FROM entries
 WHERE %s AND dataset_id=$%d AND id=chosen.candidate OFFSET 0) e
 ORDER BY chosen.candidate LIMIT $%d)`, ids, innerLimit, strings.Join(conditions, " AND "), datasetArg, limitArg)
}
