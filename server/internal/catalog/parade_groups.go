package catalog

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
)

// Grouping is ordered by the canonical name/first pair and then group key. A
// complete pair must be consumed before emitting any of its groups. We retain
// only this page's smallest keys while streaming all matching occurrences; neither
// a dense pair nor sparse refinements can truncate a successful result set.
func (a *API) paradeGroupedCandidates(ctx context.Context, q query, editions []paradeEdition) (searchResult, []orderCursor, int, error) {
	f, status, err := a.candidateFilters(ctx, q)
	if err != nil {
		return searchResult{}, nil, status, err
	}
	q = f.q
	if f.missing {
		return a.finishOrderedCandidates(ctx, q, f, nil)
	}
	var incoming orderCursor
	if q.Cursor != "" {
		raw, e := base64.RawURLEncoding.DecodeString(q.Cursor)
		if e != nil || json.Unmarshal(raw, &incoming) != nil || incoming.Query != q.fingerprint() || len(incoming.Name) > 256 || len(incoming.First) > 256 || len(incoming.Edition) > 7 || incoming.Dataset < 0 || incoming.ID < 0 {
			return searchResult{}, nil, 400, errors.New("Cursor does not match this ordered search")
		}
		g, e := hex.DecodeString(incoming.Group)
		if e != nil || len(g) != 32 {
			return searchResult{}, nil, 400, errors.New("Invalid ordered cursor")
		}
		incoming.Group = hex.EncodeToString(g)
	}
	scan := q
	scan.Group = ""
	scan.Sort = "name"
	scan.Cursor = ""
	scan.Limit = 128
	after := paradePosition{}
	if q.Cursor != "" {
		// Include the cursor's complete pair; its source IDs never decide group ordering.
		after = paradePosition{Name: incoming.Name, First: incoming.First, PairSeek: true, PairInclusive: true}
	}
	positions := []orderCursor{}
	groups := map[string]orderCursor{}
	name, first := "", ""
	havePair := false
	flush := func() {
		keys := make([]string, 0, len(groups))
		for key := range groups {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			if len(positions) > q.Limit {
				break
			}
			positions = append(positions, groups[key])
		}
		clear(groups)
	}
	consume := func(batch []paradePosition) (bool, error) {
		keys, e := a.paradeGroupKeys(ctx, batch)
		if e != nil {
			return false, e
		}
		for _, p := range batch {
			if havePair && (p.Name != name || p.First != first) {
				flush()
				if len(positions) > q.Limit {
					return true, nil
				}
				havePair = false
			}
			if !havePair {
				name, first, havePair = p.Name, p.First, true
			}
			key := keys[[2]int32{int32(p.Dataset), p.ID}]
			if q.Cursor != "" && name == incoming.Name && first == incoming.First && key <= incoming.Group {
				continue
			}
			if c, ok := groups[key]; ok {
				if p.Edition > c.Edition {
					c.Edition = p.Edition
				}
				if p.PostcodeID < c.Postcode {
					c.Postcode = p.PostcodeID
				}
				groups[key] = c
				continue
			}
			remaining := q.Limit + 1 - len(positions)
			largest := ""
			if len(groups) >= remaining {
				for k := range groups {
					if k > largest {
						largest = k
					}
				}
				if key >= largest {
					continue
				}
				delete(groups, largest)
			}
			groups[key] = orderCursor{Query: q.fingerprint(), Name: name, First: first, Edition: p.Edition, Group: key, Postcode: p.PostcodeID}
		}
		return false, nil
	}
	for {
		batch, e := a.paradePositions(ctx, scan, editions, after)
		if e != nil {
			return searchResult{}, nil, 503, searchReadError(e)
		}
		done, e := consume(batch)
		if e != nil {
			return searchResult{}, nil, 503, sourceReadError(e)
		}
		if done {
			return a.finishOrderedCandidates(ctx, q, f, positions)
		}
		if len(batch) < scan.Limit {
			flush()
			return a.finishOrderedCandidates(ctx, q, f, positions)
		}
		last := batch[len(batch)-1]
		// The last pair may be incomplete. Finish it once per remaining edition
		// using numeric, ID-ordered batches; do not reread later editions' first
		// 128 occurrences on every continuation of a dense earlier edition.
		err = a.drainParadePair(ctx, scan, editions, last, func(batch []paradePosition) error { _, e := consume(batch); return e })
		if err != nil {
			return searchResult{}, nil, 503, sourceReadError(err)
		}
		flush()
		if len(positions) > q.Limit {
			return a.finishOrderedCandidates(ctx, q, f, positions)
		}
		havePair = false
		after = paradePosition{Name: last.Name, First: last.First, PairSeek: true}
	}
}

// Resolve bounded source batches and fail on missing publication rows. Group
// identity remains the source research index's canonical contract, not a new hash
// computed from the derived projection or an inferred household/person identity.
func (a *API) paradeGroupKeys(ctx context.Context, positions []paradePosition) (map[[2]int32]string, error) {
	requested := map[int16][]int32{}
	for _, p := range positions {
		requested[p.Dataset] = append(requested[p.Dataset], p.ID)
	}
	out := map[[2]int32]string{}
	for dataset, ids := range requested {
		rows, e := a.searchRows(ctx, "group_members", "SELECT id,group_key FROM research_keys WHERE dataset_id=$1 AND id=ANY($2::integer[])", []any{dataset, ids}, false)
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			var id int32
			var key []byte
			if e = rows.Scan(&id, &key); e != nil {
				break
			}
			if len(key) != 32 {
				e = recordIntegrity("invalid grouped source key")
				break
			}
			out[[2]int32{int32(dataset), id}] = hex.EncodeToString(key)
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			return nil, e
		}
	}
	if len(out) != len(positions) {
		return nil, recordIntegrity("missing grouped source key")
	}
	return out, nil
}

func (a *API) drainParadePair(ctx context.Context, q query, editions []paradeEdition, last paradePosition, consume func([]paradePosition) error) error {
	ctx, end := a.searchPhase(ctx, "parade_pair")
	var failure error
	defer func() { end(failure) }()
	q.Sort = ""
	q.Limit = 2048
	ordered := slices.Clone(editions)
	slices.SortFunc(ordered, func(a, b paradeEdition) int { return int(a.ID) - int(b.ID) })
	pair := &paradeNamePair{Name: last.Name, First: last.First}
	for _, d := range ordered {
		if d.ID < last.Dataset {
			continue
		}
		after := paradePosition{}
		if d.ID == last.Dataset {
			after = last
		}
		for {
			sql, args := paradeSQLBounds(q, d, after, true, pair)
			rows, e := a.ParadeDB.Query(ctx, sql, args...)
			if e != nil {
				failure = e
				return e
			}
			batch := []paradePosition{}
			edition := d.sortEdition()
			for rows.Next() {
				p := paradePosition{Dataset: d.ID, Edition: edition}
				if e = rows.Scan(&p.ID, &p.NameID, &p.FirstID, &p.PostcodeID, &p.Name, &p.First); e != nil {
					break
				}
				p.Name = pair.Name
				p.First = pair.First
				batch = append(batch, p)
			}
			if e == nil {
				e = rows.Err()
			}
			rows.Close()
			if e != nil {
				failure = e
				return e
			}
			if e = consume(batch); e != nil {
				failure = e
				return e
			}
			if len(batch) < q.Limit {
				break
			}
			after = batch[len(batch)-1]
		}
	}
	return nil
}
