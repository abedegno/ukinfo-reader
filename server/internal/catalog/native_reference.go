package catalog

// Private operator oracle. This is not a serving access path or a request flag.
// A candidate's third-page source row supplies only an inclusive ordering bound.
// Every source occurrence at or before that bound is independently enumerated,
// address-filtered and passed through the original native grouping/sort. Missing
// earlier candidates therefore change the oracle output; no candidate IDs are
// used as an allow-list. The bounded prefix includes lookahead for two full pages.
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NativeOrderedPrefixReference(ctx context.Context, db *pgxpool.Pool, values url.Values, boundDataset string, boundID int32) ([][]byte, error) {
	q, e := parse(values)
	if e != nil {
		return nil, errors.New("invalid reference query")
	}
	if !q.hasAddress() || q.Near != "" || q.hasRefinements() || !(q.Group != "" && (q.Sort == "" || q.Sort == "name") || q.Group == "" && q.Sort == "edition") {
		return nil, errors.New("unsupported native reference ordering")
	}
	ctx = withRecordCache(ctx)
	a := NewAPI(db)
	f, status, e := a.candidateFilters(ctx, q)
	if e != nil || status != 200 || f.missing {
		return nil, errors.New("reference inventory unavailable")
	}
	q = f.q
	var dataset int16
	for id, key := range f.datasetKeys {
		if key == boundDataset {
			dataset = id
		}
	}
	if dataset == 0 || boundID < 1 {
		return nil, errors.New("reference boundary outside catalogue")
	}
	var name, first, date string
	if e = db.QueryRow(ctx, `SELECT n.key,t.key,COALESCE(substring(d.manifest->>'edition' FROM '([12][0-9]{3}-[01][0-9])'),'')
 FROM entries e JOIN terms n ON n.id=e.name_id JOIN terms t ON t.id=e.first_id JOIN datasets d ON d.id=e.dataset_id
 WHERE e.dataset_id=$1 AND e.id=$2 AND e.kind=$3`, dataset, boundID, f.args[1]).Scan(&name, &first, &date); e != nil {
		return nil, errors.New("reference boundary source unavailable")
	}
	var matches map[int16][]int32
	if q.Sort == "edition" {
		matches, e = a.referenceAddressPrefix(ctx, q, f, name, first, date)
		if e != nil {
			return nil, e
		}
	} else {
		before, e := referenceTermIDs(ctx, db, name, false)
		if e != nil {
			return nil, e
		}
		equal, e := referenceTermIDs(ctx, db, name, true)
		if e != nil {
			return nil, e
		}
		firstIDs, e := referenceTermIDs(ctx, db, first, false)
		if e != nil {
			return nil, e
		}
		equalFirst, e := referenceTermIDs(ctx, db, first, true)
		if e != nil {
			return nil, e
		}
		firstIDs = append(firstIDs, equalFirst...)
		args := append([]any(nil), f.args...)
		args = append(args, before, equal, firstIDs)
		bound := fmt.Sprintf("(name_id=ANY($%d::integer[]) OR (name_id=ANY($%d::integer[]) AND first_id=ANY($%d::integer[])))", len(args)-2, len(args)-1, len(args))
		dates, e := db.Query(ctx, `SELECT id,COALESCE(substring(manifest->>'edition' FROM '([12][0-9]{3}-[01][0-9])'),'') FROM datasets WHERE id=ANY($1::smallint[]) ORDER BY id`, f.datasetIDs)
		if e != nil {
			return nil, errors.New("reference editions unavailable")
		}
		type editionDate struct {
			ID   int16
			Date string
		}
		editions := []editionDate{}
		for dates.Next() {
			var d editionDate
			if e = dates.Scan(&d.ID, &d.Date); e != nil {
				break
			}
			editions = append(editions, d)
		}
		if e == nil {
			e = dates.Err()
		}
		dates.Close()
		if e != nil {
			return nil, errors.New("reference editions unavailable")
		}
		branches := []string{}
		for _, d := range editions {
			if q.Sort == "edition" && d.Date < date {
				continue
			}
			conditions := append([]string(nil), f.conditions...)
			if q.Sort != "edition" || d.Date == date {
				conditions = append(conditions, bound)
			}
			args = append(args, d.ID)
			conditions = append(conditions, fmt.Sprintf("dataset_id=$%d", len(args)))
			branches = append(branches, "(SELECT dataset_id,id FROM entries WHERE "+strings.Join(conditions, " AND ")+")")
		}
		if len(branches) == 0 {
			return nil, errors.New("reference boundary has no editions")
		}
		sql := strings.Join(branches, " UNION ALL ") + " ORDER BY dataset_id,id LIMIT 2000001"
		if len(f.termCTEs) > 0 {
			sql = "WITH " + strings.Join(f.termCTEs, ",") + " " + sql
		}
		rows, e := db.Query(ctx, sql, args...)
		if e != nil {
			return nil, errors.New("reference prefix unavailable")
		}
		matches = map[int16][]int32{}
		batch := map[int16][]int32{}
		count := 0
		filter := func() error {
			matched, ready, e := a.indexedAddressCandidates(ctx, q, batch)
			if e != nil || !ready {
				return errors.New("complete reverse-address reference required")
			}
			for d, ids := range matched {
				matches[d] = append(matches[d], ids...)
			}
			clear(batch)
			return nil
		}
		for rows.Next() {
			var d int16
			var id int32
			if e = rows.Scan(&d, &id); e != nil {
				break
			}
			count++
			if count > 2000000 {
				e = errors.New("reference prefix exceeded safety bound")
				break
			}
			batch[d] = append(batch[d], id)
			if count%4096 == 0 {
				if e = filter(); e != nil {
					break
				}
			}
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			return nil, errors.New("reference prefix did not complete")
		}
		if len(batch) > 0 {
			if e = filter(); e != nil {
				return nil, e
			}
		}
	}
	f.useAddressCandidates(matches)
	recordCache(ctx).putPlan(q.fingerprint(), f)
	pages := [][]byte{}
	for page := 0; page < 2; page++ {
		out, status, e := a.orderedResults(ctx, q)
		if e != nil || status != 200 {
			return nil, errors.New("native prefix ordering failed")
		}
		raw, e := json.Marshal(out)
		if e != nil {
			return nil, e
		}
		pages = append(pages, raw)
		if out.Next == "" {
			break
		}
		q.Cursor = out.Next
	}
	return pages, nil
}
func referenceTermIDs(ctx context.Context, db *pgxpool.Pool, value string, equal bool) ([]int32, error) {
	op := "<"
	if equal {
		op = "="
	}
	rows, e := db.Query(ctx, "SELECT id FROM terms WHERE key"+op+"$1 ORDER BY id LIMIT 65537", value)
	if e != nil {
		return nil, errors.New("reference vocabulary unavailable")
	}
	defer rows.Close()
	ids := []int32{}
	for rows.Next() {
		var id int32
		if e = rows.Scan(&id); e != nil {
			return nil, errors.New("reference vocabulary unreadable")
		}
		ids = append(ids, id)
	}
	if rows.Err() != nil || len(ids) > 65536 {
		return nil, errors.New("reference vocabulary prefix exceeded safety bound")
	}
	return ids, nil
}

func (a *API) referenceAddressPrefix(ctx context.Context, q query, f candidateFilters, name, first, date string) (map[int16][]int32, error) {
	db := a.DB
	rows, e := db.Query(ctx, `SELECT id,COALESCE(substring(manifest->>'edition' FROM '([12][0-9]{3}-[01][0-9])'),'') FROM datasets WHERE id=ANY($1::smallint[]) ORDER BY id`, f.datasetIDs)
	if e != nil {
		return nil, errors.New("reference editions unavailable")
	}
	dates := map[int16]string{}
	selected := []int16{}
	for rows.Next() {
		var id int16
		var d string
		if e = rows.Scan(&id, &d); e != nil {
			break
		}
		if d >= date {
			dates[id] = d
			selected = append(selected, id)
		}
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return nil, errors.New("reference editions unavailable")
	}
	args := []any{selected}
	predicate := addressPredicate(q, &args)
	rows, e = db.Query(ctx, "SELECT m.dataset_id,m.ids FROM address_members m JOIN address_locations l ON l.id=m.location_id WHERE m.dataset_id=ANY($1::smallint[]) AND "+predicate, args...)
	if e != nil {
		return nil, errors.New("reference address population unavailable")
	}
	area := map[int16][]int32{}
	count := 0
	for rows.Next() {
		var d int16
		var ids []int32
		if e = rows.Scan(&d, &ids); e != nil {
			break
		}
		count += len(ids)
		if count > 8000000 {
			e = errors.New("reference address population exceeded safety bound")
			break
		}
		area[d] = append(area[d], ids...)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return nil, errors.New("reference address population did not complete")
	}
	matches := map[int16][]int32{}
	labels := map[int32]string{}
	for _, d := range selected {
		ids := area[d]
		slices.Sort(ids)
		ids = slices.Compact(ids)
		for start := 0; start < len(ids); start += 4096 {
			bound := append([]any(nil), f.args...)
			bound = append(bound, d, ids[start:min(start+4096, len(ids))])
			sql := "SELECT id,name_id,first_id FROM entries WHERE " + strings.Join(f.conditions, " AND ") + fmt.Sprintf(" AND dataset_id=$%d AND id=ANY($%d::integer[])", len(bound)-1, len(bound))
			if len(f.termCTEs) > 0 {
				sql = "WITH " + strings.Join(f.termCTEs, ",") + " " + sql
			}
			rows, e = db.Query(ctx, sql, bound...)
			if e != nil {
				return nil, errors.New("reference source batch unavailable")
			}
			type entry struct{ id, name, first int32 }
			entries := []entry{}
			needed := map[int32]bool{}
			for rows.Next() {
				var item entry
				if e = rows.Scan(&item.id, &item.name, &item.first); e != nil {
					break
				}
				entries = append(entries, item)
				for _, id := range []int32{item.name, item.first} {
					if _, ok := labels[id]; !ok {
						needed[id] = true
					}
				}
			}
			if e == nil {
				e = rows.Err()
			}
			rows.Close()
			if e != nil {
				return nil, errors.New("reference source batch unreadable")
			}
			if len(needed) > 0 {
				wanted := make([]int32, 0, len(needed))
				for id := range needed {
					wanted = append(wanted, id)
				}
				slices.Sort(wanted)
				rows, e = db.Query(ctx, "SELECT id,key FROM terms WHERE id=ANY($1::integer[]) ORDER BY id", wanted)
				if e != nil {
					return nil, errors.New("reference vocabulary unavailable")
				}
				found := 0
				for rows.Next() {
					var id int32
					var key string
					if e = rows.Scan(&id, &key); e != nil {
						break
					}
					labels[id] = key
					found++
				}
				if e == nil {
					e = rows.Err()
				}
				rows.Close()
				if e != nil || found != len(needed) {
					return nil, errors.New("reference vocabulary incomplete")
				}
			}
			for _, item := range entries {
				n, t := labels[item.name], labels[item.first]
				if dates[d] > date || n < name || n == name && t <= first {
					matches[d] = append(matches[d], item.id)
				}
			}
			if len(labels) > 65536 {
				clear(labels)
			}
		}
	}
	return matches, nil
}
