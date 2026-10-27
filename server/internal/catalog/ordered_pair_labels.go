package catalog

import (
	"context"
	"errors"
	"slices"
	"strings"
)

// Exact-name, postcode and classified-business populations can share many labels.
// Fetch their compact numeric pairs, then each distinct vocabulary ID once in
// storage order. The ceiling selects an access path, never a result limit: an
// overflowing probe retains the original complete SQL ordering.
func (a *API) batchedNamePairs(ctx context.Context, q query, ctes []string, args []any) ([]orderedNamePair, bool, error) {
	name := q.Surname
	if q.Kind == "business" {
		name = q.Organisation
	}
	exactName := name != "" && q.Match != "prefix" && !strings.HasSuffix(name, "*")
	if (!exactName && q.Postcode == "" && !(q.Kind == "business" && q.hasBusinessFilters())) || !(q.Sort == "name" || q.Group != "" && q.Sort == "") {
		return nil, false, nil
	}
	count := len(args) - 1
	if q.Cursor != "" {
		count -= 5
	}
	if count < 2 {
		return nil, false, errors.New("invalid ordered parameters")
	}
	rows, e := a.searchRows(ctx, "ordered", "WITH "+strings.Join(ctes, ",")+" SELECT name_id,first_id,occurrences FROM ordered_candidates LIMIT 65537", args[:count], false)
	if e != nil {
		return nil, true, e
	}
	pairs := []orderedNamePair{}
	ids := map[int32]bool{}
	for rows.Next() {
		var p orderedNamePair
		if e = rows.Scan(&p.nameID, &p.firstID, &p.count); e != nil {
			break
		}
		pairs = append(pairs, p)
		ids[p.nameID] = true
		ids[p.firstID] = true
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return nil, true, e
	}
	if len(pairs) == 0 {
		return nil, true, nil
	}
	if len(pairs) > 65536 {
		return nil, false, nil
	}
	wanted := make([]int32, 0, len(ids))
	for id := range ids {
		wanted = append(wanted, id)
	}
	slices.Sort(wanted)
	rows, e = a.searchRows(ctx, "vocabulary", "SELECT id,key FROM terms WHERE id=ANY($1::integer[]) ORDER BY id", []any{wanted}, true)
	if e != nil {
		return nil, true, e
	}
	labels := map[int32]string{}
	for rows.Next() {
		var id int32
		var label string
		if e = rows.Scan(&id, &label); e != nil {
			break
		}
		labels[id] = label
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return nil, true, e
	}
	if len(labels) != len(ids) {
		return nil, true, recordIntegrity("missing ordered vocabulary entry")
	}
	selected := []orderedNamePair{}
	afterName, afterFirst := "", ""
	if q.Cursor != "" {
		afterName, _ = args[count].(string)
		afterFirst, _ = args[count+1].(string)
	}
	for _, p := range pairs {
		p.name = labels[p.nameID]
		p.first = labels[p.firstID]
		if q.Cursor != "" && (p.name < afterName || p.name == afterName && p.first < afterFirst) {
			continue
		}
		selected = append(selected, p)
	}
	slices.SortFunc(selected, func(a, b orderedNamePair) int {
		if c := strings.Compare(a.name, b.name); c != 0 {
			return c
		}
		return strings.Compare(a.first, b.first)
	})
	if len(selected) > 128 {
		selected = selected[:128]
	}
	return selected, true, nil
}
