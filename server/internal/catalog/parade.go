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
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
)

const paradeVersion = 1

func (q query) streamRadius() bool { return q.Near != "" && q.nearbyCandidateStream }

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

// Select only the search shapes validated against native pages and cold/load trials.
func paradeEligible(q query) bool {
	if paradeNameProbe(q) {
		return true
	}
	if q.Near != "" {
		return q.hasAddress() && q.Group == "" && q.Sort == ""
	}
	return q.hasAddress() && (q.Sort == "" || q.Sort == "name" || (q.Sort == "edition" && q.Group == "")) ||
		(q.Postcode != "" || q.Kind == "business" && q.hasBusinessFilters()) && q.Sort == "name" && q.Group == ""
}

// Full first-name discovery dominates broad alphabetical surnames. Keep narrow
// searches native and decide from at most 4,097 compact index rows, before any
// derived inventory/query. The probe never limits the eventual result set.
func paradeNameProbe(q query) bool {
	return q.Kind == "person" && q.Sort == "name" && q.Group == "" && q.Surname != "" &&
		q.Match == "exact" && !strings.HasSuffix(q.Surname, "*") && q.First == "" &&
		q.Postcode == "" && !q.hasAddress() && q.Near == "" &&
		q.House == "" && q.Building == "" && q.Flat == "" && q.Middle == ""
}

const paradeNameProbeSQL = `SELECT EXISTS(SELECT FROM entries e
 WHERE e.dataset_id=ANY($1::smallint[]) AND e.kind=0
 AND e.name_id=(SELECT id FROM terms WHERE key=$2)
 OFFSET 4096 LIMIT 1)`

func (a *API) paradeWideName(ctx context.Context, q query, editions []paradeEdition) (bool, error) {
	ids := make([]int16, len(editions))
	for i, d := range editions {
		ids[i] = d.ID
	}
	rows, err := a.searchRows(ctx, "source_probe", paradeNameProbeSQL, []any{ids, q.Surname}, false)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, err
		}
		return false, recordIntegrity("missing name population probe")
	}
	var wide bool
	if err := rows.Scan(&wide); err != nil {
		return false, err
	}
	return wide, rows.Err()
}

// ParadeDB contains derived search rows only; permissions, visibility and original
// records come from DB. Missing generations fall back before candidate execution.
// Active failures are returned, never retried through a second expensive backend.
func (a *API) paradeResults(ctx context.Context, q query) (searchResult, int, bool, error) {
	if a.ParadeDB == nil {
		return searchResult{}, 0, false, nil
	}
	// Default-order postcode and simple-name searches retain native execution.
	// Select before contacting the target so native-only shapes remain independent
	// of the optional database.
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
	if paradeNameProbe(q) {
		wide, err := a.paradeWideName(ctx, q, editions)
		if err != nil {
			end(err)
			return searchResult{}, 503, true, sourceReadError(err)
		}
		if !wide {
			end(nil)
			a.metrics.plan("paradedb_fallback")
			return searchResult{}, 0, false, nil
		}
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
	var distances map[int32]postcodeDistance
	var geography *GeographySearch
	if q.Near != "" {
		geography, err = a.nearbyCentre(ctx, &q)
		if err != nil {
			status := 503
			var geoErr *geographyError
			if errors.As(err, &geoErr) {
				status = geoErr.Status
			}
			return searchResult{}, status, true, err
		}
		q.nearbyCandidateStream = true
		a.metrics.plan("paradedb_radius_stream")
	}
	if q.Group != "" {
		out, status, err := orderedResultsWith(ctx, q, func(ctx context.Context, q query) (searchResult, []orderCursor, int, error) {
			return a.paradeGroupedCandidates(ctx, q, editions)
		})
		return out, status, true, err
	}
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
	out := searchResult{Items: []result{}, Geography: geography}
	// Refinements use displayed original fields and complete batches, just like
	// the native path. No sparse-match candidate cap or false end of results.
	for {
		batchQ := q
		batchQ.Limit = q.Limit + 1
		if q.House != "" || q.Building != "" || q.Flat != "" || q.Middle != "" || q.streamRadius() {
			batchQ.Limit = 100
		}
		positions, e := a.paradePositions(ctx, batchQ, editions, after)
		if e != nil {
			return searchResult{}, 503, true, searchReadError(e)
		}
		if q.streamRadius() {
			distances, e = a.nearbyCandidateDistances(ctx, geography, positions)
			if e != nil {
				return searchResult{}, 503, true, e
			}
		}
		found := map[[2]int32]result{}
		requested := map[int16][]int32{}
		for _, p := range positions {
			if q.streamRadius() {
				if _, ok := distances[p.PostcodeID]; !ok {
					continue
				}
			}
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
			if q.streamRadius() {
				if _, ok := distances[p.PostcodeID]; !ok {
					continue
				}
			}
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
			if q.Near != "" {
				location, ok := distances[p.PostcodeID]
				if !ok {
					return searchResult{}, 503, true, recordIntegrity("search radius candidate outside source geography")
				}
				item.Distance = &location.Distance
				item.LocationQuality = location.Quality
				item.LocationTerminated = location.Terminated
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
	Name, First, Edition            string
	PairSeek, PairInclusive         bool
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
	if q.Sort == "name" || q.Sort == "edition" {
		var c orderCursor
		if json.Unmarshal(b, &c) != nil || c.Query != q.fingerprint() || keys[c.Dataset] == "" || c.ID < 1 || len(c.Name) > 256 || len(c.First) > 256 || len(c.Edition) > 7 || c.Group != "" {
			return paradePosition{}, invalid
		}
		return paradePosition{Dataset: c.Dataset, ID: c.ID, Name: c.Name, First: c.First, Edition: c.Edition}, nil
	}
	var c cursor
	if json.Unmarshal(b, &c) != nil || c.Query != q.fingerprint() || keys[c.Dataset] == "" || c.ID < 1 || c.Name < 0 || c.First < 0 || c.Postcode < 0 {
		return paradePosition{}, invalid
	}
	return paradePosition{Dataset: c.Dataset, ID: c.ID, NameID: c.Name, FirstID: c.First, PostcodeID: c.Postcode}, nil
}
func paradeEncode(q query, p paradePosition) string {
	if q.Sort == "name" || q.Sort == "edition" {
		return encodeOrder(orderCursor{Query: q.fingerprint(), Dataset: p.Dataset, ID: p.ID, Name: p.Name, First: p.First, Edition: p.Edition})
	}
	b, _ := json.Marshal(cursor{Query: q.fingerprint(), Dataset: p.Dataset, ID: p.ID, Name: p.NameID, First: p.FirstID, Postcode: p.PostcodeID})
	return base64.RawURLEncoding.EncodeToString(b)
}
func paradeOrder(q query) []string {
	if q.Sort == "name" || q.Sort == "edition" {
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
	if q.Sort == "edition" {
		if c := cmp.Compare(b.Edition, a.Edition); c != 0 {
			return c
		}
	}
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
	return paradeSQLBounds(q, d, after, prefilter, nil)
}

type paradeNamePair struct{ Name, First string }

func paradeSQLBounds(q query, d paradeEdition, after paradePosition, prefilter bool, pair *paradeNamePair) (string, []any) {
	args := []any{}
	conditions := searchFieldConditions(q, &args)
	if q.Near != "" && !q.streamRadius() {
		// Resolve the complete radius against authoritative ONSPD first. Empty or
		// unresolved state matches nothing; it can never remove the radius filter.
		if len(q.nearbyPostcodes) == 0 {
			conditions = append(conditions, "false")
		} else if !q.streamRadius() {
			args = append(args, q.nearbyPostcodes)
			conditions = append(conditions, fmt.Sprintf("postcode_id=ANY($%d::integer[])", len(args)))
		}
	}
	if pair != nil {
		args = append(args, pair.Name, pair.First)
		conditions = append(conditions, fmt.Sprintf("name=$%d AND first_full=$%d", len(args)-1, len(args)))
	}
	if after.PairSeek {
		args = append(args, after.Name, after.First)
		op := ">"
		if after.PairInclusive {
			op = ">="
		}
		conditions = append(conditions, fmt.Sprintf("(name>$%d OR (name=$%d AND first_full%s$%d))", len(args)-1, len(args)-1, op, len(args)))
		after = paradePosition{}
	}
	if q.Sort == "edition" && after.ID > 0 && d.sortEdition() != after.Edition {
		if d.sortEdition() > after.Edition {
			conditions = append(conditions, "false")
		}
		after = paradePosition{}
	}
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
	projection := "id,name_id,first_id,postcode_id,name,first_full"
	if q.Sort != "name" && q.Sort != "edition" {
		projection = "id,name_id,first_id,postcode_id,''::text,''::text"
	}
	return "SELECT " + projection + " FROM ukinfo_search." + d.table() + " WHERE " + strings.Join(conditions, " AND ") + " ORDER BY " + strings.Join(order, ",") + fmt.Sprintf(" LIMIT $%d", len(args)), args
}
func (a *API) paradePositions(ctx context.Context, q query, editions []paradeEdition, after paradePosition) ([]paradePosition, error) {
	ctx, end := a.searchPhase(ctx, "parade_candidates")
	var failure error
	defer func() { end(failure) }()
	out := []paradePosition{}
	read := func(ctx context.Context, d paradeEdition) ([]paradePosition, error) {
		sql, args := paradeSQL(q, d, after)
		rows, err := a.ParadeDB.Query(ctx, sql, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		batch := []paradePosition{}
		edition := d.sortEdition()
		for rows.Next() {
			p := paradePosition{Dataset: d.ID, Edition: edition}
			if err = rows.Scan(&p.ID, &p.NameID, &p.FirstID, &p.PostcodeID, &p.Name, &p.First); err != nil {
				return nil, err
			}
			batch = append(batch, p)
		}
		return batch, rows.Err()
	}
	if q.Sort == "edition" {
		editions = slices.Clone(editions)
		slices.SortStableFunc(editions, func(a, b paradeEdition) int { return cmp.Compare(b.sortEdition(), a.sortEdition()) })
		for _, d := range editions {
			if after.ID > 0 && d.sortEdition() > after.Edition {
				continue
			}
			if len(out) >= q.Limit && d.sortEdition() < out[len(out)-1].Edition {
				break
			}
			batch, err := read(ctx, d)
			if err != nil {
				failure = err
				return nil, err
			}
			out = append(out, batch...)
		}
	} else {
		// Two edition scans may overlap I/O. The shared four-connection target
		// pool still bounds all requests; any failed branch cancels its sibling
		// and discards the entire partial result. Merge order remains exact.
		g, groupCtx := errgroup.WithContext(ctx)
		g.SetLimit(2)
		var mu sync.Mutex
		for _, d := range editions {
			g.Go(func() error {
				batch, err := read(groupCtx, d)
				if err != nil {
					return err
				}
				mu.Lock()
				out = append(out, batch...)
				mu.Unlock()
				return nil
			})
		}
		if err := g.Wait(); err != nil {
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

var paradeEditionDate = regexp.MustCompile(`[12][0-9]{3}-[01][0-9]`)

func (d paradeEdition) sortEdition() string {
	var m struct {
		Edition string `json:"edition"`
	}
	_ = json.Unmarshal(d.Manifest, &m)
	return paradeEditionDate.FindString(m.Edition)
}
