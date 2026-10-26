package catalog

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const paradeVersion = 1

type paradeEdition struct {
	ID       int16
	Key      string
	Manifest json.RawMessage
	Entries  int64
	Imported time.Time
}

func (d paradeEdition) generation() string {
	d.Imported = d.Imported.UTC()
	b, _ := json.Marshal(struct {
		Version int
		Edition paradeEdition
	}{paradeVersion, d})
	return fmt.Sprintf("%x", sha256.Sum256(b))
}
func (d paradeEdition) table() string {
	return "d" + strconv.Itoa(int(d.ID)) + "_" + d.generation()[:24]
}

// Keep the edition visibility rule identical to the compact backend. In
// particular, an imported full edition hides its sample only in the all scope.
func catalogueSelection(dataset string) (string, []any) {
	if dataset != "all" {
		return "key=$1", []any{dataset}
	}
	return `NOT (COALESCE((d.manifest->>'sample')::boolean,false) AND EXISTS (
 SELECT 1 FROM datasets full_edition WHERE full_edition.manifest->>'edition'=d.manifest->>'edition'
 AND NOT COALESCE((full_edition.manifest->>'sample')::boolean,false)))`, nil
}
func searchEditions(ctx context.Context, db *pgxpool.Pool, scope string) ([]paradeEdition, error) {
	where, args := catalogueSelection(scope)
	rows, err := db.Query(ctx, "SELECT id,key,manifest,entries,imported_at FROM datasets d WHERE "+where+" ORDER BY id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []paradeEdition{}
	for rows.Next() {
		var d paradeEdition
		if err = rows.Scan(&d.ID, &d.Key, &d.Manifest, &d.Entries, &d.Imported); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Restrict the optional accelerator to address filters validated by the real trial.
func paradeEligible(q query) bool {
	return q.hasAddress() && q.Group == "" && q.Near == "" && (q.Sort == "" || q.Sort == "name")
}

// ParadeDB contains derived search rows only; permissions, visibility and original
// records come from DB. Missing generations fall back before candidate execution.
// Active failures are returned, never retried through a second expensive backend.
func (a *API) paradeResults(ctx context.Context, q query) (searchResult, int, bool, error) {
	if a.ParadeDB == nil {
		return searchResult{}, 0, false, nil
	}
	// The all-edition serving trial favors ParadeDB for address filters, while
	// broad postcode and simple-name searches can regress. Select before touching
	// the target; native-only queries also work when that dependency is offline.
	if !paradeEligible(q) {
		a.metrics.plan("paradedb_fallback")
		return searchResult{}, 0, false, nil
	}
	inventoryCtx, end := a.searchPhase(ctx, "parade_inventory")
	editions, err := searchEditions(inventoryCtx, a.DB, q.Dataset)
	if err != nil {
		end(err)
		return searchResult{}, 503, true, sourceReadError(err)
	}
	if len(editions) == 0 {
		end(nil)
		return searchResult{}, 404, true, errors.New("Edition not imported")
	}
	var exists bool
	err = a.ParadeDB.QueryRow(inventoryCtx, "SELECT to_regclass('ukinfo_search.editions') IS NOT NULL").Scan(&exists)
	if err != nil {
		end(err)
		return searchResult{}, 503, true, searchReadError(err)
	}
	if !exists {
		end(nil)
		a.metrics.plan("paradedb_fallback")
		return searchResult{}, 0, false, nil
	}
	for _, d := range editions {
		var ready bool
		err = a.ParadeDB.QueryRow(inventoryCtx, `SELECT EXISTS(SELECT FROM ukinfo_search.editions WHERE dataset_id=$1 AND generation=$2 AND entries=$3 AND version=$4 AND ready AND last_id=entries)`, d.ID, d.generation(), d.Entries, paradeVersion).Scan(&ready)
		if err != nil {
			end(err)
			return searchResult{}, 503, true, searchReadError(err)
		}
		if !ready {
			end(nil)
			a.metrics.plan("paradedb_fallback")
			return searchResult{}, 0, false, nil
		}
	}
	end(nil)
	a.metrics.plan("paradedb")
	keys := map[int16]string{}
	for _, d := range editions {
		keys[d.ID] = d.Key
	}
	b, _ := json.Marshal(keys)
	q.Catalogue = fmt.Sprintf("%x", sha256.Sum256(b))
	after, err := paradeAfter(q, keys)
	if err != nil {
		return searchResult{}, 400, true, err
	}
	limit := q.Limit
	out := searchResult{Items: []result{}}
	// Refinements use displayed original fields and complete batches, just like
	// the native path. No sparse-match candidate cap or false end of results.
	for {
		batchQ := q
		batchQ.Limit = q.Limit + 1
		if q.House != "" || q.Building != "" || q.Flat != "" || q.Middle != "" {
			batchQ.Limit = 100
		}
		positions, e := a.paradePositions(ctx, batchQ, editions, after)
		if e != nil {
			return searchResult{}, 503, true, searchReadError(e)
		}
		found := map[[2]int32]result{}
		requested := map[int16][]int32{}
		for _, p := range positions {
			requested[p.Dataset] = append(requested[p.Dataset], p.ID)
		}
		for dataset, ids := range requested {
			records, e := a.loadRecords(ctx, dataset, ids)
			if e != nil {
				return searchResult{}, 503, true, sourceReadError(e)
			}
			for _, r := range records {
				r.Dataset = keys[dataset]
				found[[2]int32{int32(dataset), int32(r.ID)}] = r
			}
		}
		for _, p := range positions {
			item, ok := found[[2]int32{int32(p.Dataset), p.ID}]
			if !ok {
				return searchResult{}, 503, true, recordIntegrity("search source missing")
			}
			accepted, e := q.accepts(item.Record)
			if e != nil {
				return searchResult{}, 503, true, sourceReadError(e)
			}
			if !accepted {
				continue
			}
			if len(out.Items) == limit {
				out.Next = paradeEncode(q, after)
				return out, 200, true, nil
			}
			out.Items = append(out.Items, item)
			after = p
		}
		if len(positions) < batchQ.Limit {
			return out, 200, true, nil
		}
		// Advance past rejected records as well, but retain the last accepted position
		// when returning a full page so the next matching lookahead is not skipped.
		after = positions[len(positions)-1]
	}
}

type paradePosition struct {
	Dataset                         int16
	ID, NameID, FirstID, PostcodeID int32
	Name, First                     string
}

func paradeAfter(q query, keys map[int16]string) (paradePosition, error) {
	if q.Cursor == "" {
		return paradePosition{}, nil
	}
	b, e := base64.RawURLEncoding.DecodeString(q.Cursor)
	invalid := errors.New("Cursor does not match this search")
	if e != nil {
		return paradePosition{}, invalid
	}
	if q.Sort == "name" {
		var c orderCursor
		if json.Unmarshal(b, &c) != nil || c.Query != q.fingerprint() || keys[c.Dataset] == "" || c.ID < 1 || len(c.Name) > 256 || len(c.First) > 256 || c.Group != "" {
			return paradePosition{}, invalid
		}
		return paradePosition{Dataset: c.Dataset, ID: c.ID, Name: c.Name, First: c.First}, nil
	}
	var c cursor
	if json.Unmarshal(b, &c) != nil || c.Query != q.fingerprint() || keys[c.Dataset] == "" || c.ID < 1 || c.Name < 0 || c.First < 0 || c.Postcode < 0 {
		return paradePosition{}, invalid
	}
	return paradePosition{Dataset: c.Dataset, ID: c.ID, NameID: c.Name, FirstID: c.First, PostcodeID: c.Postcode}, nil
}
func paradeEncode(q query, p paradePosition) string {
	if q.Sort == "name" {
		return encodeOrder(orderCursor{Query: q.fingerprint(), Dataset: p.Dataset, ID: p.ID, Name: p.Name, First: p.First})
	}
	b, _ := json.Marshal(cursor{Query: q.fingerprint(), Dataset: p.Dataset, ID: p.ID, Name: p.NameID, First: p.FirstID, Postcode: p.PostcodeID})
	return base64.RawURLEncoding.EncodeToString(b)
}
func paradeOrder(q query) []string {
	if q.Sort == "name" {
		return []string{"name", "first_full", "dataset_id", "id"}
	}
	if q.hasAddress() {
		return []string{"id", "dataset_id"}
	}
	if q.Postcode != "" {
		return []string{"postcode_id", "name_id", "first_id", "id", "dataset_id"}
	}
	return []string{"name_id", "first_id", "id", "dataset_id"}
}
func (p paradePosition) value(key string) any {
	switch key {
	case "dataset_id":
		return p.Dataset
	case "id":
		return p.ID
	case "name_id":
		return p.NameID
	case "first_id":
		return p.FirstID
	case "postcode_id":
		return p.PostcodeID
	case "name":
		return p.Name
	default:
		return p.First
	}
}
func paradeCompare(q query, a, b paradePosition) int {
	for _, key := range paradeOrder(q) {
		var c int
		switch av := a.value(key).(type) {
		case int16:
			c = cmp.Compare(av, b.value(key).(int16))
		case int32:
			c = cmp.Compare(av, b.value(key).(int32))
		case string:
			c = cmp.Compare(av, b.value(key).(string))
		}
		if c != 0 {
			return c
		}
	}
	return 0
}
func paradeSQL(q query, d paradeEdition, after paradePosition) (string, []any) {
	return paradeSQLPrefix(q, d, after, true)
}
func paradeSQLPrefix(q query, d paradeEdition, after paradePosition, prefilter bool) (string, []any) {
	args := []any{}
	conditions := searchFieldConditions(q, &args)
	// Keep the complete scalar range/precision predicate. The redundant literal
	// token prefix lets the inverted index narrow candidates before fast-field
	// range checks and sorting; user regex syntax is never accepted.
	if prefilter && q.Postcode != "" && q.PostcodeLength > 0 {
		args = append(args, regexp.QuoteMeta(q.Postcode)+".*")
		conditions = append(conditions, fmt.Sprintf("postcode @@@ pdb.regex($%d)", len(args)))
	}
	conditions = append(conditions, "id @@@ pdb.all()")
	order := []string{}
	for _, key := range paradeOrder(q) {
		if key != "dataset_id" {
			order = append(order, key)
		}
	}
	if after.ID > 0 {
		prefix, clauses := []string{}, []string{}
		for _, key := range paradeOrder(q) {
			if key == "dataset_id" {
				if d.ID > after.Dataset {
					clauses = append(clauses, "("+strings.Join(append(prefix, "true"), " AND ")+")")
				}
				if d.ID != after.Dataset {
					break
				}
				continue
			}
			args = append(args, after.value(key))
			v := fmt.Sprintf("$%d", len(args))
			clauses = append(clauses, "("+strings.Join(append(slices.Clone(prefix), key+">"+v), " AND ")+")")
			prefix = append(prefix, key+"="+v)
		}
		if len(clauses) == 0 {
			conditions = append(conditions, "false")
		} else {
			conditions = append(conditions, "("+strings.Join(clauses, " OR ")+")")
		}
	}
	args = append(args, q.Limit)
	return "SELECT id,name_id,first_id,postcode_id,name,first_full FROM ukinfo_search." + d.table() + " WHERE " + strings.Join(conditions, " AND ") + " ORDER BY " + strings.Join(order, ",") + fmt.Sprintf(" LIMIT $%d", len(args)), args
}
func (a *API) paradePositions(ctx context.Context, q query, editions []paradeEdition, after paradePosition) ([]paradePosition, error) {
	ctx, end := a.searchPhase(ctx, "parade_candidates")
	var failure error
	defer func() { end(failure) }()
	out := []paradePosition{}
	for _, d := range editions {
		sql, args := paradeSQL(q, d, after)
		rows, err := a.ParadeDB.Query(ctx, sql, args...)
		if err != nil {
			failure = err
			return nil, err
		}
		for rows.Next() {
			p := paradePosition{Dataset: d.ID}
			if err = rows.Scan(&p.ID, &p.NameID, &p.FirstID, &p.PostcodeID, &p.Name, &p.First); err != nil {
				break
			}
			out = append(out, p)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			failure = err
			return nil, err
		}
	}
	slices.SortFunc(out, func(a, b paradePosition) int { return paradeCompare(q, a, b) })
	if len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, nil
}
