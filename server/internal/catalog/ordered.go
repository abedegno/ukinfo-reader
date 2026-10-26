package catalog

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
)

type orderCursor struct {
	Query    string  `json:"q"`
	Name     string  `json:"n"`
	First    string  `json:"f"`
	Edition  string  `json:"e"`
	Distance float64 `json:"m"`
	Group    string  `json:"g"`
	Dataset  int16   `json:"d"`
	ID       int32   `json:"i"`
	Postcode int32   `json:"p"`
}

func encodeOrder(c orderCursor) string {
	raw, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(raw)
}
func (a *API) orderedResults(ctx context.Context, q query) (searchResult, int, error) {
	return orderedResultsWith(ctx, q, a.orderedCandidates)
}

func orderedResultsWith(ctx context.Context, q query, candidates func(context.Context, query) (searchResult, []orderCursor, int, error)) (searchResult, int, error) {
	if !q.hasRefinements() {
		out, _, status, err := candidates(ctx, q)
		return out, status, err
	}
	limit := q.Limit
	q.Limit = 100
	out := searchResult{Items: []result{}}
	var last orderCursor
	for {
		batch, positions, status, err := candidates(ctx, q)
		if err != nil {
			return searchResult{}, status, err
		}
		out.Geography = batch.Geography
		for i, item := range batch.Items {
			accepted, e := q.accepts(item.Record)
			if e != nil {
				return searchResult{}, 503, e
			}
			if !accepted {
				continue
			}
			if len(out.Items) == limit {
				out.Next = encodeOrder(last)
				return out, 200, nil
			}
			out.Items = append(out.Items, item)
			last = positions[i]
		}
		if batch.Next == "" {
			return out, 200, nil
		}
		q.Cursor = batch.Next
	}
}
func (a *API) orderedCandidates(ctx context.Context, q query) (searchResult, []orderCursor, int, error) {
	f, status, err := a.candidateFilters(ctx, q)
	if err != nil {
		return searchResult{}, nil, status, err
	}
	q = f.q
	out := searchResult{Items: []result{}, Geography: f.geography}
	positions := []orderCursor{}
	if f.missing {
		return out, positions, 200, nil
	}
	args := f.args
	pairPlan := !q.hasAddress() && q.Near == "" && (q.Surname != "" || q.Organisation != "" || q.Postcode != "") && (q.Sort == "name" || (q.Group != "" && q.Sort == ""))
	conditions := append([]string(nil), f.conditions...)

	base := "SELECT * FROM entries WHERE " + strings.Join(conditions, " AND ")
	if q.hasAddress() && !f.addressMatched {
		predicate := addressPredicate(q, &args)
		f.termCTEs = append(f.termCTEs, "ordered_addresses AS MATERIALIZED (SELECT m.dataset_id,unnest(m.ids) AS id FROM address_members m JOIN address_locations l ON l.id=m.location_id WHERE m.dataset_id=ANY($1::smallint[]) AND "+predicate+")")
		if q.Surname != "" || q.First != "" || q.Organisation != "" || q.Postcode != "" || q.Near != "" || q.hasBusinessFilters() {
			base = "SELECT e.* FROM (" + base + ") e JOIN ordered_addresses a USING(dataset_id,id)"
		} else {
			base = "SELECT e.* FROM ordered_addresses a CROSS JOIN LATERAL (SELECT * FROM entries WHERE " + strings.Join(conditions, " AND ") + " AND dataset_id=a.dataset_id AND id=a.id OFFSET 0) e"
		}
	}
	// The materialization fence fixes the selective candidate set before name-order
	// joins. Otherwise an order-preserving plan can walk the entire name dictionary
	// and probe a filtered source millions of times before reaching one surname.
	if pairPlan {
		base = "SELECT name_id,first_id,count(*) AS occurrences FROM (" + base + ") eligible GROUP BY name_id,first_id"
	}
	pairPrefix := append([]string(nil), f.termCTEs...)
	f.termCTEs = append(f.termCTEs, "ordered_candidates AS MATERIALIZED ("+base+")")
	joins := " CROSS JOIN LATERAL (SELECT key FROM terms WHERE id=e.name_id OFFSET 0) n CROSS JOIN LATERAL (SELECT key FROM terms WHERE id=e.first_id OFFSET 0) t JOIN datasets d ON d.id=e.dataset_id"
	distance := "0::double precision"
	if q.Sort == "nearest" {
		ids := []int32{}
		metres := []float64{}
		for id, point := range f.distances {
			ids = append(ids, id)
			metres = append(metres, point.Distance)
		}
		args = append(args, ids, metres)
		joins += fmt.Sprintf(" JOIN unnest($%d::integer[],$%d::double precision[]) geo(postcode_id,metres) ON geo.postcode_id=e.postcode_id", len(args)-1, len(args))
		distance = "geo.metres"
	}
	edition := "COALESCE(substring(d.manifest->>'edition' FROM '([12][0-9]{3}-[01][0-9])'),'') COLLATE \"C\""
	projection := "SELECT ''::bytea AS g,n.key AS name,t.key AS first," + edition + " AS edition," + distance + " AS distance,e.dataset_id,e.id,e.postcode_id FROM ordered_candidates e" + joins
	if q.Group != "" {
		projection = "SELECT k.group_key AS g,min(n.key) AS name,min(t.key) AS first,max(" + edition + ") AS edition,min(" + distance + ") AS distance,0::smallint AS dataset_id,0::integer AS id,min(e.postcode_id) AS postcode_id FROM ordered_candidates e" + joins + " CROSS JOIN LATERAL (SELECT group_key FROM research_keys WHERE dataset_id=e.dataset_id AND id=e.id OFFSET 0) k GROUP BY k.group_key"
	}
	order := "name,first,g,dataset_id,id"
	compare := "(name,first,g,dataset_id,id)"
	after := ""
	pairAfter, pairRowAfter := "", ""
	if q.Sort == "edition" {
		order = "edition DESC," + order
	}
	if q.Sort == "nearest" {
		order = "distance," + order
	}
	if q.Cursor != "" {
		raw, e := base64.RawURLEncoding.DecodeString(q.Cursor)
		var c orderCursor
		if e != nil || json.Unmarshal(raw, &c) != nil || c.Query != q.fingerprint() || len(c.Name) > 256 || len(c.First) > 256 || len(c.Edition) > 7 || c.Dataset < 0 || c.ID < 0 || (q.Group == "" && (f.datasetKeys[c.Dataset] == "" || c.ID < 1)) {
			return searchResult{}, nil, 400, errors.New("Cursor does not match this ordered search")
		}
		g, e := hex.DecodeString(c.Group)
		if e != nil || (q.Group != "" && len(g) != 32) || (q.Group == "" && len(g) != 0) {
			return searchResult{}, nil, 400, errors.New("Invalid ordered cursor")
		}
		args = append(args, c.Name, c.First, g, c.Dataset, c.ID)

		if pairPlan {
			nameArg, firstArg, groupArg, datasetArg, idArg := len(args)-4, len(args)-3, len(args)-2, len(args)-1, len(args)
			pairAfter = fmt.Sprintf(" WHERE (n.key,t.key)>=($%d,$%d)", nameArg, firstArg)
			if q.Group != "" {
				pairRowAfter = fmt.Sprintf(" AND ((p.name,p.first)>($%d,$%d) OR k.group_key>$%d)", nameArg, firstArg, groupArg)
			} else {
				pairRowAfter = fmt.Sprintf(" WHERE ((p.name,p.first)>($%d,$%d) OR (e.dataset_id,e.id)>($%d,$%d))", nameArg, firstArg, datasetArg, idArg)
			}
		}
		tuple := fmt.Sprintf("($%d,$%d,$%d,$%d,$%d)", len(args)-4, len(args)-3, len(args)-2, len(args)-1, len(args))
		after = compare + ">" + tuple
		if q.Sort == "edition" {
			args = append(args, c.Edition)
			after = fmt.Sprintf("(edition<$%d OR (edition=$%d AND %s))", len(args), len(args), after)
		}
		if q.Sort == "nearest" {
			args = append(args, c.Distance)
			after = fmt.Sprintf("(distance>$%d OR (distance=$%d AND %s))", len(args), len(args), after)
		}
		after = " WHERE " + after
	}
	args = append(args, q.Limit+1)

	pairQuery, pairInner := "", ""
	if pairPlan {
		// At most one pair can be exhausted by the incoming cursor. 128 pairs therefore
		// provide lookahead for every supported page size (<=100), without truncating
		// the search. Subsequent requests seek from the last returned name/pair/key.
		pairQuery = "SELECT p.name_id,p.first_id,n.key AS name,t.key AS first,p.occurrences FROM ordered_candidates p CROSS JOIN LATERAL (SELECT key FROM terms WHERE id=p.name_id OFFSET 0) n CROSS JOIN LATERAL (SELECT key FROM terms WHERE id=p.first_id OFFSET 0) t" + pairAfter + " ORDER BY n.key,t.key LIMIT 128"
		source := "SELECT * FROM entries WHERE " + strings.Join(conditions, " AND ") + " AND name_id=p.name_id AND first_id=p.first_id"
		inner := "SELECT ''::bytea AS g," + edition + " AS edition,0::double precision AS distance,e.dataset_id,e.id,e.postcode_id FROM (" + source + ") e JOIN datasets d ON d.id=e.dataset_id" + pairRowAfter + " ORDER BY e.dataset_id,e.id"
		if q.Group != "" {
			inner = "SELECT k.group_key AS g,max(" + edition + ") AS edition,0::double precision AS distance,0::smallint AS dataset_id,0::integer AS id,min(e.postcode_id) AS postcode_id FROM (" + source + ") e JOIN datasets d ON d.id=e.dataset_id CROSS JOIN LATERAL (SELECT group_key FROM research_keys WHERE dataset_id=e.dataset_id AND id=e.id OFFSET 0) k WHERE true" + pairRowAfter + " GROUP BY k.group_key ORDER BY k.group_key"
		}
		inner += fmt.Sprintf(" LIMIT $%d", len(args))
		pairInner = inner
	}
	sql := "SELECT name,first,edition,distance,g,dataset_id,id,postcode_id FROM (" + projection + ") ordered" + after + " ORDER BY " + order + fmt.Sprintf(" LIMIT $%d", len(args))
	if len(f.termCTEs) > 0 {
		sql = "WITH " + strings.Join(f.termCTEs, ",") + " " + sql
	}
	if pairPlan {
		// Sorting a lateral expansion for all 128 pairs executes every pair before
		// LIMIT, even when the first pair fills the page. Read labels first, then
		// expand pairs in lexical order only until the page and lookahead are full.
		positions, err = a.orderedPairPositions(ctx, q, f.termCTEs, pairPrefix, pairQuery, pairInner, after, args)
	} else {
		positions, err = a.readOrderedPositions(ctx, q, sql, args, q.Limit+1)
	}
	if err != nil {
		return searchResult{}, nil, 503, searchReadError(err)
	}
	return a.finishOrderedCandidates(ctx, q, f, positions)
}

func (a *API) finishOrderedCandidates(ctx context.Context, q query, f candidateFilters, positions []orderCursor) (searchResult, []orderCursor, int, error) {
	out := searchResult{Items: []result{}, Geography: f.geography}
	if len(positions) > q.Limit {
		positions = positions[:q.Limit]
		out.Next = encodeOrder(positions[len(positions)-1])
	}

	members := make(map[string]listingMember)
	if q.Group != "" && len(positions) > 0 {
		groups := [][]byte{}
		for _, c := range positions {
			g, _ := hex.DecodeString(c.Group)
			groups = append(groups, g)
		}
		var e error
		members, e = a.groupRepresentatives(ctx, f.datasetIDs, groups)
		if e != nil {
			return searchResult{}, nil, 503, sourceReadError(e)
		}

		rows, e := a.searchRows(ctx, "group_members", "SELECT k.group_key,count(*),array_agg(DISTINCT d.key ORDER BY d.key) FROM research_keys k JOIN datasets d ON d.id=k.dataset_id WHERE k.group_key=ANY($1::bytea[]) AND k.dataset_id=ANY($2::smallint[]) GROUP BY k.group_key", []any{groups, f.datasetIDs}, false)
		if e != nil {
			return searchResult{}, nil, 503, searchReadError(e)
		}
		for rows.Next() {
			var g []byte
			var count int64
			var editions []string
			if e = rows.Scan(&g, &count, &editions); e != nil {
				break
			}
			key := hex.EncodeToString(g)
			m := members[key]
			m.count, m.editions = count, editions
			members[key] = m
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			return searchResult{}, nil, 503, e
		}
	}
	requested := map[int16][]int32{}
	for _, c := range positions {
		dataset, id := c.Dataset, c.ID
		if q.Group != "" {
			m, ok := members[c.Group]
			if !ok {
				return searchResult{}, nil, 503, errors.New("Grouped source unavailable")
			}
			dataset, id = m.dataset, m.id
		}
		requested[dataset] = append(requested[dataset], id)
	}
	type recordKey struct {
		dataset int16
		id      int32
	}
	found := map[recordKey]result{}
	for dataset, ids := range requested {
		records, e := a.loadRecords(ctx, dataset, ids)
		if e != nil {
			return searchResult{}, nil, 503, sourceReadError(e)
		}
		for _, item := range records {
			item.Dataset = f.datasetKeys[dataset]
			found[recordKey{dataset, int32(item.ID)}] = item
		}
	}
	for _, c := range positions {
		dataset, id := c.Dataset, c.ID
		if q.Group != "" {
			m := members[c.Group]
			dataset, id = m.dataset, m.id
		}
		item := found[recordKey{dataset, id}]
		if q.Group != "" {
			m := members[c.Group]
			item.Group, item.Occurrences, item.Editions = c.Group, m.count, m.editions
		}
		if q.Near != "" {
			point := f.distances[c.Postcode]
			distance := point.Distance
			item.Distance = &distance
			item.LocationQuality = point.Quality
			item.LocationTerminated = point.Terminated
		}
		out.Items = append(out.Items, item)
	}

	return out, positions, 200, nil
}

// readOrderedPositions preserves the database order and releases the private
// ordered transaction before record loading or another pair query begins.
func (a *API) readOrderedPositions(ctx context.Context, q query, sql string, args []any, limit int) ([]orderCursor, error) {
	rows, err := a.searchRows(ctx, "ordered", sql, args, false)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	positions := []orderCursor{}
	for len(positions) < limit && rows.Next() {
		var c orderCursor
		var g []byte
		if err = rows.Scan(&c.Name, &c.First, &c.Edition, &c.Distance, &g, &c.Dataset, &c.ID, &c.Postcode); err != nil {
			return nil, err
		}
		c.Query = q.fingerprint()
		c.Group = hex.EncodeToString(g)
		positions = append(positions, c)
	}
	return positions, rows.Err()
}

type orderedNamePair struct {
	nameID, firstID int32
	name, first     string
	count           int64
}

func (a *API) orderedPairPositions(ctx context.Context, q query, ctes, prefix []string, pairs, inner, after string, args []any) ([]orderCursor, error) {
	pairArgs := args[:len(args)-1]
	if q.Cursor != "" {
		pairArgs = args[:len(args)-4]
	}
	selected, batched, err := a.postcodeNamePairs(ctx, q, ctes, args)
	if err != nil {
		return nil, err
	}
	if !batched {
		rows, err := a.searchRows(ctx, "ordered", "WITH "+strings.Join(ctes, ",")+" "+pairs, pairArgs, false)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var p orderedNamePair
			if err = rows.Scan(&p.nameID, &p.firstID, &p.name, &p.first, &p.count); err != nil {
				rows.Close()
				return nil, err
			}
			selected = append(selected, p)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	positions := []orderCursor{}
	for start := 0; start < len(selected); {
		remaining := q.Limit + 1 - len(positions)
		width := 1
		count := selected[start].count
		// Batch sparse pairs to avoid one transaction per returned record. Never
		// speculate into another dense pair: it could fill the entire page itself.
		for width < 4 && start+width < len(selected) && count < int64(remaining) && selected[start+width].count < int64(remaining) {
			count += selected[start+width].count
			width++
		}
		bound := append([]any(nil), args...)
		bound[len(args)-1] = remaining
		values := []string{}
		for _, p := range selected[start : start+width] {
			n := len(bound)
			values = append(values, fmt.Sprintf("($%d::integer,$%d::integer,$%d::text COLLATE \"C\",$%d::text COLLATE \"C\")", n+1, n+2, n+3, n+4))
			bound = append(bound, p.nameID, p.firstID, p.name, p.first)
		}
		sql := "SELECT * FROM (SELECT p.name,p.first,s.edition,s.distance,s.g,s.dataset_id,s.id,s.postcode_id FROM (VALUES " + strings.Join(values, ",") + ") p(name_id,first_id,name,first) CROSS JOIN LATERAL (" + inner + ") s) ordered" + after + " ORDER BY name,first,g,dataset_id,id" + fmt.Sprintf(" LIMIT $%d", len(args))
		if len(prefix) > 0 {
			sql = "WITH " + strings.Join(prefix, ",") + " " + sql
		}
		a.metrics.orderedPairs.Add(uint64(width))
		batch, e := a.readOrderedPositions(ctx, q, sql, bound, remaining)
		if e != nil {
			return nil, e
		}
		positions = append(positions, batch...)
		if len(positions) > q.Limit {
			break
		}
		start += width
	}

	return positions, nil
}
func (a *API) researchAvailability(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	rows, err := a.DB.Query(ctx, "SELECT d.key FROM research_indexes i JOIN datasets d ON d.id=i.dataset_id AND d.entries=i.entries WHERE i.version=1 ORDER BY d.key")
	if err != nil {
		respond(w, 200, map[string]any{"datasets": []string{}, "notice": "Research indexes are not available; migrate the catalogue and build index-research."})
		return
	}
	defer rows.Close()
	keys := []string{}
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			break
		}
		keys = append(keys, key)
	}
	if err != nil || rows.Err() != nil {
		fail(w, 503, "Research index inventory unavailable")
		return
	}
	business := []string{}
	rows, e := a.DB.Query(ctx, "SELECT d.key FROM datasets d WHERE EXISTS(SELECT 1 FROM entries e WHERE e.dataset_id=d.id AND e.kind=1 LIMIT 1) ORDER BY d.key")
	if e != nil {
		fail(w, 503, "Listing types unavailable")
		return
	}
	for rows.Next() {
		var key string
		if e = rows.Scan(&key); e != nil {
			break
		}
		business = append(business, key)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		fail(w, 503, "Listing types unavailable")
		return
	}
	respond(w, 200, map[string]any{"datasets": keys, "business_datasets": business})
}

func (a *API) orderedRows(ctx context.Context, sql string, args []any) (pgx.Rows, func(), error) {
	tx, err := a.DB.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, func() {}, err
	}
	finish := func() { _ = tx.Rollback(context.Background()) }
	if _, err = tx.Exec(ctx, "SET LOCAL statement_timeout='4000'; SET LOCAL work_mem='32MB'"); err != nil {
		finish()
		return nil, func() {}, err
	}
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		finish()
		return nil, func() {}, err
	}
	return rows, finish, nil
}
