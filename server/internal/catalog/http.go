package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/abedegno/ukinfo-reader/server/internal/telemetry"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/klauspost/compress/zstd"
)

type API struct {
	DB             *pgxpool.Pool
	HealthDB       *pgxpool.Pool
	metrics        *apiMetrics
	slots          chan struct{}
	decoder        *zstd.Decoder
	geographyCache *geographyCache
	geographySlots chan struct{}
	addressSlots   chan struct{}
}

func NewAPI(db *pgxpool.Pool) *API {
	decoder, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(16<<20))
	if err != nil {
		panic(err)
	}
	return &API{DB: db, HealthDB: db, metrics: newAPIMetrics(), slots: make(chan struct{}, 32), decoder: decoder, geographyCache: newGeographyCache(), geographySlots: make(chan struct{}, 2), addressSlots: make(chan struct{}, 4)}
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, message string) {
	code := "unavailable"
	switch status {
	case 400:
		code = "invalid_request"
	case 404:
		code = "not_found"
	case 429:
		code = "rate_limited"
	}
	writeFailure(w, status, code, message)
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if err := a.HealthDB.Ping(ctx); err != nil {
			fail(w, 503, "Database unavailable")
			return
		}
		respond(w, 200, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/datasets", a.datasets)
	mux.HandleFunc("GET /api/geography", a.geography)
	mux.HandleFunc("GET /api/map", a.mapLocations)
	mux.HandleFunc("GET /api/addresses", a.addresses)
	mux.HandleFunc("GET /api/research", a.researchAvailability)
	mux.HandleFunc("GET /api/group", a.groupSources)
	mux.HandleFunc("GET /api/record", a.record)
	mux.HandleFunc("GET /api/address", a.address)
	mux.HandleFunc("GET /api/search", a.search)
	mux.HandleFunc("GET /api/export", func(w http.ResponseWriter, r *http.Request) { a.searchPage(w, r, true) })
	return a.metrics.wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Request-ID", newRequestID())
		if r.URL.Path == "/api/health" {
			mux.ServeHTTP(w, r)
			return
		}
		select {
		case a.slots <- struct{}{}:
			defer func() { <-a.slots }()
		default:
			fail(w, 503, "Search capacity reached; retry shortly")
			return
		}
		ctx := withRecordCache(r.Context())
		recordCache(ctx).observe = a.metrics
		mux.ServeHTTP(w, r.WithContext(ctx))
	}))
}
func (a *API) datasets(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	rows, err := a.DB.Query(ctx, "SELECT key,manifest,entries,imported_at FROM datasets ORDER BY key")
	if err != nil {
		fail(w, 503, "Catalogue unavailable")
		return
	}
	defer rows.Close()
	type dataset struct {
		Key        string          `json:"key"`
		Manifest   json.RawMessage `json:"manifest"`
		Entries    int64           `json:"entries"`
		ImportedAt time.Time       `json:"imported_at"`
	}
	result := []dataset{}
	for rows.Next() {
		var d dataset
		if err = rows.Scan(&d.Key, &d.Manifest, &d.Entries, &d.ImportedAt); err != nil {
			fail(w, 503, "Catalogue unavailable")
			return
		}
		result = append(result, d)
	}
	if rows.Err() != nil {
		fail(w, 503, "Catalogue unavailable")
		return
	}
	respond(w, 200, result)
}

type query struct {
	Group, Sort, SIC, Industry, Catalogue                 string `json:",omitempty"`
	Dataset, Kind, Surname, First, Organisation, Postcode string
	House                                                 string `json:",omitempty"`
	Building                                              string `json:",omitempty"`
	Flat                                                  string `json:",omitempty"`
	FirstMatch                                            string `json:",omitempty"`
	Middle                                                string `json:",omitempty"`
	Street                                                string `json:",omitempty"`
	Locality                                              string `json:",omitempty"`
	Town                                                  string `json:",omitempty"`
	County                                                string `json:",omitempty"`
	Near                                                  string `json:",omitempty"`
	NearLength                                            int    `json:",omitempty"`
	GeoRelease                                            string `json:",omitempty"`
	Radius                                                int    `json:",omitempty"`
	Match                                                 string
	PostcodeLength                                        int
	Limit                                                 int
	Cursor                                                string
}
type cursor struct {
	Query    string `json:"q"`
	Postcode int32  `json:"p"`
	Name     int32  `json:"n"`
	First    int32  `json:"f"`
	ID       int32  `json:"i"`
	Dataset  int16  `json:"d"`
}
type result struct {
	Archive            string          `json:"archive_id,omitempty"`
	Group              string          `json:"group_key,omitempty"`
	Occurrences        int64           `json:"occurrence_count,omitempty"`
	Editions           []string        `json:"editions,omitempty"`
	Dataset            string          `json:"dataset"`
	ID                 int64           `json:"id"`
	SourceID           string          `json:"source_id"`
	Record             json.RawMessage `json:"record"`
	Distance           *float64        `json:"distance_m,omitempty"`
	LocationQuality    int16           `json:"location_quality,omitempty"`
	LocationTerminated string          `json:"location_terminated,omitempty"`
}

var postcodeSearch = regexp.MustCompile(`^(?:[A-Z]{1,2}[0-9][A-Z0-9]?(?:[0-9](?:[A-Z]{2})?)?|GIR(?:0(?:AA)?)?)$`)

var fullPostcodeSearch = regexp.MustCompile(`^(?:[A-Z]{1,2}[0-9][A-Z0-9]?[0-9][A-Z]{2}|GIR0AA)$`)

// Preserve the district/sector boundary before removing input spaces (W11 vs W1 1).
func postcodeFullLength(input, compact string) int {
	if compact == "" || fullPostcodeSearch.MatchString(compact) {
		return 0
	}
	raw := strings.Fields(strings.ToUpper(strings.TrimSuffix(strings.TrimSpace(input), "*")))
	sector := compact == "GIR0" || len(compact) > 4 || (len(raw) == 2 && len(raw[1]) == 1 && raw[1][0] >= '0' && raw[1][0] <= '9')
	if sector {
		return len(compact) + 2
	}
	return len(compact) + 3
}

type searchFieldError struct{ field, message string }

func (e *searchFieldError) Error() string      { return e.message }
func invalidField(field, message string) error { return &searchFieldError{field, message} }

func parse(v url.Values) (query, error) {
	q := query{Street: Normalize(v.Get("street")), Locality: Normalize(v.Get("locality")), Town: Normalize(v.Get("town")), County: Normalize(v.Get("county")), Dataset: strings.TrimSpace(v.Get("dataset")), Kind: v.Get("kind"), Surname: Normalize(v.Get("surname")), First: Normalize(v.Get("first_name")), Organisation: Normalize(v.Get("organisation")), Postcode: strings.TrimSuffix(Postcode(v.Get("postcode")), "*"), Match: v.Get("match"), Near: Postcode(v.Get("near")), Limit: 50, Cursor: v.Get("cursor")}
	q.Group, q.Sort, q.SIC, q.Industry = v.Get("group"), v.Get("sort"), Normalize(v.Get("sic")), Normalize(v.Get("industry"))
	if q.Group == "page" {
		q.Group = ""
	}
	if q.Group != "" && q.Group != "listing" {
		return q, errors.New("group must be listing or omitted")
	}
	if q.Sort != "" && q.Sort != "catalogue" && q.Sort != "name" && q.Sort != "nearest" && q.Sort != "edition" {
		return q, errors.New("sort must be catalogue, name, nearest or edition")
	}
	if q.Sort == "catalogue" {
		q.Sort = ""
	}
	if q.Sort == "nearest" && q.Near == "" {
		return q, invalidField("postcode", "Nearest order requires a nearby search. Enter a postcode and choose Nearby.")
	}
	for _, value := range []string{q.SIC, q.Industry} {
		base := strings.TrimSuffix(value, "*")
		if len(value) > 256 || strings.ContainsAny(base, "*?%\x00") || (strings.HasSuffix(value, "*") && len(base) < 2) {
			return q, errors.New("classification accepts exact text or at least two characters followed by *")
		}
	}
	q.House, q.Building, q.Flat = Normalize(v.Get("house")), Normalize(v.Get("building")), Normalize(v.Get("flat"))
	q.FirstMatch, q.Middle = v.Get("first_match"), Normalize(strings.TrimSuffix(strings.TrimSpace(v.Get("middle_initial")), "."))
	if q.FirstMatch != "" && q.FirstMatch != "exact" && q.FirstMatch != "prefix" && q.FirstMatch != "given" {
		return q, invalidField("first_match", "first-name matching must be exact, prefix or given")
	}
	if q.Middle != "" && (utf8.RuneCountInString(q.Middle) != 1 || !isLetter(q.Middle)) {
		return q, invalidField("middle_initial", "enter one letter for the middle initial")
	}
	for i, value := range []string{q.Street, q.Locality, q.Town, q.County, q.Building, q.House, q.Flat} {
		minimum := 2
		if i >= 5 {
			minimum = 1
		}
		if value == "" {
			continue
		}
		base := strings.TrimSuffix(value, "*")
		if strings.ContainsAny(base, "*?%") || (strings.HasSuffix(value, "*") && utf8.RuneCountInString(base) < minimum) {
			return q, invalidField([]string{"street", "locality", "town", "county", "building", "house", "flat"}[i], "Use exact text or a single trailing *. Enter at least two prefix characters, or one for house/flat.")
		}
	}
	if q.Near != "" {
		if !postcodeSearch.MatchString(q.Near) {
			return q, invalidField("postcode", "Enter a valid centre postcode, district or sector. See the format guide beside Postcode.")
		}
		q.NearLength = postcodeFullLength(v.Get("near"), q.Near)
		q.Radius = 8047
		if v.Has("radius_m") {
			n, err := strconv.Atoi(v.Get("radius_m"))
			if err != nil || n < 50 || n > 50000 {
				return q, fmt.Errorf("radius_m must be 50–50000 metres")
			}
			q.Radius = n
		}
	} else if v.Has("radius_m") {
		return q, fmt.Errorf("radius requires a centre postcode")
	}
	if q.Match == "" {
		q.Match = "exact"
	}
	if q.Match != "exact" && q.Match != "prefix" {
		return q, invalidField("match", "name matching must be exact or prefix")
	}
	for _, value := range []string{q.Surname, q.Organisation} {
		if value == "" {
			continue
		}
		prefix := strings.TrimSuffix(value, "*")
		if strings.ContainsAny(prefix, "*?%") || (strings.HasSuffix(value, "*") || q.Match == "prefix") && utf8.RuneCountInString(prefix) < 2 {
			field := "surname"
			if q.Organisation != "" {
				field = "organisation"
			}
			return q, invalidField(field, "Use at least two characters followed by a single trailing *.")
		}
	}
	if strings.ContainsAny(strings.TrimSuffix(q.First, "*"), "*?%") || q.First == "*" {
		return q, invalidField("first_name", "Use exact text or a single trailing * for first name prefixes.")
	}
	if q.Postcode != "" && !postcodeSearch.MatchString(q.Postcode) {
		return q, invalidField("postcode", "Enter a valid postcode, district or sector. See the format guide beside Postcode.")
	}
	q.PostcodeLength = postcodeFullLength(v.Get("postcode"), q.Postcode)

	if q.Kind == "" {
		q.Kind = "person"
	}
	if q.Kind != "person" && q.Kind != "business" {
		return q, fmt.Errorf("kind must be person or business")
	}
	if q.Dataset == "" {
		return q, invalidField("dataset", "choose an imported edition")
	}
	if q.Kind == "person" && q.hasBusinessFilters() {
		return q, errors.New("classification filters are only available for businesses")
	}
	if q.Kind == "person" && (q.Organisation != "" || (q.Surname == "" && q.Postcode == "" && q.Near == "" && !q.hasAddress())) {
		return q, fmt.Errorf("enter a surname, address or postcode for a people search")
	}
	if q.Kind == "business" && (q.Surname != "" || q.First != "" || (q.Organisation == "" && q.Postcode == "" && q.Near == "" && !q.hasAddress() && !q.hasBusinessFilters())) {
		return q, fmt.Errorf("enter a business name, address or postcode")
	}
	if q.Kind == "business" && q.Middle != "" {
		return q, errors.New("middle initial is only available for people searches")
	}
	for _, s := range []string{q.Dataset, q.Surname, q.First, q.Organisation, q.Postcode, q.Near, q.Street, q.Locality, q.Town, q.County, q.House, q.Building, q.Flat} {
		if len(s) > 256 || strings.ContainsRune(s, 0) {
			return q, fmt.Errorf("search field too long or invalid")
		}
	}
	if len(q.Cursor) > 2048 {
		return q, fmt.Errorf("invalid cursor")
	}
	if v.Has("limit") {
		n, err := strconv.Atoi(v.Get("limit"))
		if err != nil || n < 1 || n > 100 {
			return q, fmt.Errorf("limit must be 1–100")
		}
		q.Limit = n
	}
	return q, nil
}
func (q query) fingerprint() string {
	q.Cursor = ""
	q.Limit = 0
	b, _ := json.Marshal(q)
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

func (a *API) search(w http.ResponseWriter, r *http.Request) {
	a.searchPage(w, r, false)
}
func (a *API) searchPage(w http.ResponseWriter, r *http.Request, export bool) {
	q, err := parse(r.URL.Query())
	if err != nil {
		var field *searchFieldError
		if errors.As(err, &field) {
			respond(w, 400, map[string]string{"error": err.Error(), "field": field.field})
		} else {
			fail(w, 400, err.Error())
		}
		return
	}
	timeout := 2 * time.Second
	if q.Near != "" || q.hasAddress() || q.hasRefinements() || q.needsResearch() || q.Sort != "" {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	if q.hasAddress() || q.hasRefinements() || q.needsResearch() || q.Sort != "" {
		select {
		case a.addressSlots <- struct{}{}:
			defer func() { <-a.addressSlots }()
		case <-ctx.Done():
			fail(w, 503, "Address search is busy; retry shortly")
			return
		}
	}
	data, status, err := a.searchResults(ctx, q)
	if err != nil {
		failError(w, status, err)
		return
	}
	items, geography := data.Items, data.Geography
	if export {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Content-Disposition", `attachment; filename="ukinfo-page.jsonl"`)
		encoder := json.NewEncoder(w)
		for _, item := range items {
			if err := encoder.Encode(struct {
				Dataset            string           `json:"dataset"`
				SourceID           string           `json:"source_id"`
				Record             json.RawMessage  `json:"record"`
				Distance           *float64         `json:"distance_m,omitempty"`
				LocationQuality    int16            `json:"location_quality,omitempty"`
				LocationTerminated string           `json:"location_terminated,omitempty"`
				Geography          *GeographySearch `json:"geography,omitempty"`
				Archive            string           `json:"archive_id,omitempty"`
			}{item.Dataset, item.SourceID, item.Record, item.Distance, item.LocationQuality, item.LocationTerminated, geography, item.Archive}); err != nil {
				return
			}
		}
		return
	}
	respond(w, 200, data)
}

type searchResult struct {
	Items     []result         `json:"items"`
	Next      string           `json:"next_cursor"`
	Geography *GeographySearch `json:"geography,omitempty"`
	positions []cursor
}

// searchCandidates reads one indexed page. Its positions also allow a displayed-field
// refinement to resume after an accepted occurrence rather than an arbitrary batch.
type candidateFilters struct {
	q                    query
	datasetKeys          map[int16]string
	datasetIDs           []int16
	args                 []any
	conditions, termCTEs []string
	missing, areaOnly    bool
	addressMatched       bool
	geography            *GeographySearch
	nearbyIDs            []int32
	radiusArg            int
	distances            map[int32]postcodeDistance
}

func (a *API) candidateFilters(ctx context.Context, q query) (candidateFilters, int, error) {
	cache := recordCache(ctx)
	key := q.fingerprint()
	if f, ok := cache.getPlan(key); ok {
		f.q.Cursor, f.q.Limit = q.Cursor, q.Limit
		return f, 200, nil
	}
	f, status, err := a.buildCandidateFilters(ctx, q)
	if err == nil {
		cache.putPlan(key, f)
	}
	return f, status, err
}

func (a *API) buildCandidateFilters(ctx context.Context, q query) (candidateFilters, int, error) {
	var err error
	datasetKeys := map[int16]string{}
	datasetIDs := []int16{}
	datasetSQL := "SELECT id,key FROM datasets WHERE key=$1 ORDER BY id"
	datasetArgs := []any{q.Dataset}
	if q.Dataset == "all" {
		datasetSQL = `SELECT d.id,d.key FROM datasets d WHERE NOT (
   COALESCE((d.manifest->>'sample')::boolean,false) AND EXISTS (
    SELECT 1 FROM datasets full_edition WHERE full_edition.manifest->>'edition'=d.manifest->>'edition'
    AND NOT COALESCE((full_edition.manifest->>'sample')::boolean,false))) ORDER BY d.id`
		datasetArgs = nil
	}
	ds, err := a.DB.Query(ctx, datasetSQL, datasetArgs...)
	if err != nil {
		return candidateFilters{}, 503, errors.New("Catalogue unavailable")
	}
	for ds.Next() {
		var id int16
		var key string
		if err = ds.Scan(&id, &key); err != nil {
			break
		}
		datasetIDs = append(datasetIDs, id)
		datasetKeys[id] = key
	}
	if err == nil {
		err = ds.Err()
	}
	ds.Close()
	if err != nil {
		return candidateFilters{}, 503, errors.New("Catalogue unavailable")
	}
	generationRaw, _ := json.Marshal(datasetKeys)
	q.Catalogue = fmt.Sprintf("%x", sha256.Sum256(generationRaw))

	if len(datasetIDs) == 0 {
		return candidateFilters{}, 404, errors.New("Edition not imported")
	}
	if q.hasAddress() {
		var indexed int
		err = a.DB.QueryRow(ctx, "SELECT count(*) FROM address_indexes i JOIN datasets d ON d.id=i.dataset_id AND d.entries=i.entries WHERE i.dataset_id=ANY($1::smallint[]) AND i.version=1", datasetIDs).Scan(&indexed)
		if err != nil || indexed != len(datasetIDs) {
			return candidateFilters{}, 503, errors.New("Address search needs a complete address index for every selected edition")
		}
	}
	kind := int16(0)
	if q.Kind == "business" {
		kind = 1
	}
	if q.hasAddress() || q.needsResearch() {
		// A kind-prefix probe avoids expanding whole address areas for editions
		// containing no listings of the requested type (notably pre-2005 businesses).
		selected := []int16{}
		for _, dataset := range datasetIDs {
			var first int32
			e := a.DB.QueryRow(ctx, `SELECT id FROM entries WHERE dataset_id=$1 AND kind=$2 ORDER BY name_id,first_id,id LIMIT 1`, dataset, kind).Scan(&first)
			if errors.Is(e, pgx.ErrNoRows) {
				continue
			}
			if e != nil {
				return candidateFilters{}, 503, errors.New("Catalogue listing types unavailable")
			}
			selected = append(selected, dataset)
		}
		datasetIDs = selected
	}
	args := []any{datasetIDs, kind}
	conditions := []string{"dataset_id=ANY($1::smallint[])", "kind=$2"}
	missing := len(datasetIDs) == 0
	var geography *GeographySearch
	var nearbyIDs []int32
	radiusArg := 0
	areaOnly := q.Near != "" && q.Surname == "" && q.First == "" && q.Organisation == "" && q.Postcode == "" && !q.hasAddress()
	distances := map[int32]postcodeDistance{}
	if q.Near != "" {
		distances, geography, err = a.nearby(ctx, &q)
		if err != nil {
			status := 503
			var geoErr *geographyError
			if errors.As(err, &geoErr) {
				status = geoErr.Status
			}
			return candidateFilters{}, status, errors.New(err.Error())
		}
		nearbyIDs = make([]int32, 0, len(distances))
		for id := range distances {
			nearbyIDs = append(nearbyIDs, id)
		}
		args = append(args, nearbyIDs)
		radiusArg = len(args) - 1
		conditions = append(conditions, fmt.Sprintf("postcode_id=ANY($%d::integer[])", len(args)))
		if len(nearbyIDs) == 0 {
			missing = true
		}
	}
	termCTEs := []string{}
	for _, filter := range []struct {
		column, key string
		postcode    bool
	}{
		{"name_id", q.Surname, false}, {"first_id", q.First, false}, {"name_id", q.Organisation, false}, {"postcode_id", q.Postcode, true},
	} {
		if filter.key == "" {
			continue
		}
		key := strings.TrimSuffix(filter.key, "*")
		matching := q.Match
		if filter.column == "first_id" && q.FirstMatch != "" {
			matching = q.FirstMatch
		}
		prefix := (!filter.postcode && (matching == "prefix" || matching == "given" || strings.HasSuffix(filter.key, "*"))) || (filter.postcode && q.PostcodeLength > 0)
		ids := []int32{}
		sql := "SELECT id,key FROM terms WHERE key=$1"
		termArgs := []any{key}
		if prefix {
			sql = "SELECT id,key FROM terms WHERE key >= $1 AND key < $2 ORDER BY key LIMIT 8193"
			termArgs = append(termArgs, key+"\U0010ffff")
			if filter.postcode {
				sql = "SELECT id,key FROM terms WHERE key >= $1 AND key < $2 AND (length(key)=$3 OR key=$1) ORDER BY key LIMIT 8193"
				termArgs = append(termArgs, q.PostcodeLength)
			}
		}
		rows, e := a.DB.Query(ctx, sql, termArgs...)
		if e != nil {
			return candidateFilters{}, 503, errors.New("Search vocabulary unavailable")
		}
		observed := 0
		for rows.Next() {
			var id int32
			var term string
			if err = rows.Scan(&id, &term); err != nil {
				break
			}
			observed++
			if filter.column == "first_id" && !q.acceptsGivenName(term) {
				continue
			}
			ids = append(ids, id)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return candidateFilters{}, 503, errors.New("Search vocabulary unavailable")
		}
		if observed > 8192 {
			// Small expansions use bound integer arrays. Broad expansions stay in
			// PostgreSQL and are shared by all edition branches, without truncation.
			args = append(args, key, key+"\U0010ffff")
			predicate := fmt.Sprintf("key >= $%d AND key < $%d", len(args)-1, len(args))
			if filter.postcode {
				args = append(args, q.PostcodeLength)
				predicate += fmt.Sprintf(" AND (length(key)=$%d OR key=$%d)", len(args), len(args)-2)
			}
			name := fmt.Sprintf("matching_terms_%d", len(termCTEs))
			termCTEs = append(termCTEs, fmt.Sprintf("%s AS MATERIALIZED (SELECT array_agg(id) AS ids FROM terms WHERE %s)", name, predicate))
			conditions = append(conditions, fmt.Sprintf("%s=ANY((SELECT ids FROM %s)::integer[])", filter.column, name))
			continue
		}
		if len(ids) == 0 {
			missing = true
		}
		args = append(args, ids)
		conditions = append(conditions, fmt.Sprintf("%s=ANY($%d::integer[])", filter.column, len(args)))
	}
	if q.needsResearch() {
		var ready int
		if err = a.DB.QueryRow(ctx, "SELECT count(*) FROM research_indexes i JOIN datasets d ON d.id=i.dataset_id AND d.entries=i.entries WHERE i.dataset_id=ANY($1::smallint[]) AND i.version=1", datasetIDs).Scan(&ready); err != nil || ready != len(datasetIDs) {
			return candidateFilters{}, 503, errors.New("Build a complete research index for each selected edition to use grouping or business classifications")
		}
	}
	if q.hasBusinessFilters() {
		clauses := []string{"r.dataset_id=entries.dataset_id", "r.id=entries.id"}
		for _, filter := range []struct{ column, value string }{{"sic", q.SIC}, {"industry", q.Industry}} {
			if filter.value == "" {
				continue
			}
			base := strings.TrimSuffix(filter.value, "*")
			args = append(args, base)
			predicate := fmt.Sprintf("r.%s=$%d", filter.column, len(args))
			if strings.HasSuffix(filter.value, "*") {
				args = append(args, base+"\U0010ffff")
				predicate = fmt.Sprintf("r.%s >= $%d AND r.%s < $%d", filter.column, len(args)-1, filter.column, len(args))
			}
			clauses = append(clauses, "r."+filter.column+"<>''", predicate)
		}
		// Keep a selective source anchor ahead of classification probes. Pulling
		// this EXISTS up into a semi-join can start with a national industry prefix
		// and do millions of random source-ID lookups for one postcode district.
		// Classification-only searches retain their classification-driven plan.
		fence := ""
		if q.Postcode != "" || q.Near != "" || q.Surname != "" || q.First != "" || q.Organisation != "" {
			fence = " OFFSET 0"
		}
		conditions = append(conditions, "EXISTS(SELECT 1 FROM research_keys r WHERE "+strings.Join(clauses, " AND ")+fence+")")
	}

	f := candidateFilters{q: q, datasetKeys: datasetKeys, datasetIDs: datasetIDs, args: args, conditions: conditions, termCTEs: termCTEs, missing: missing, areaOnly: areaOnly, geography: geography, nearbyIDs: nearbyIDs, radiusArg: radiusArg, distances: distances}
	if err = a.matchSelectiveAddresses(ctx, &f); err != nil {
		return candidateFilters{}, 503, errors.New("Selective address search unavailable or timed out")
	}
	return f, 200, nil
}
func (a *API) searchCandidates(ctx context.Context, q query) (searchResult, int, error) {
	f, status, err := a.candidateFilters(ctx, q)
	if err != nil {
		return searchResult{}, status, err
	}
	q = f.q
	datasetKeys, datasetIDs, args, conditions, termCTEs, missing, areaOnly, geography, nearbyIDs, radiusArg, distances := f.datasetKeys, f.datasetIDs, f.args, f.conditions, f.termCTEs, f.missing, f.areaOnly, f.geography, f.nearbyIDs, f.radiusArg, f.distances

	afterPostcode := int32(0)
	order := "name_id,first_id,id,dataset_id"
	if q.Postcode != "" || q.Near != "" {
		order = "postcode_id,name_id,first_id,id,dataset_id"
	}
	if q.hasAddress() {
		order = "id,dataset_id"
	}
	afterID := int32(0)
	if q.Cursor != "" {
		b, e := base64.RawURLEncoding.DecodeString(q.Cursor)
		var c cursor
		if e != nil || json.Unmarshal(b, &c) != nil || c.Query != q.fingerprint() || datasetKeys[c.Dataset] == "" || c.ID < 1 || c.Name < 0 || c.First < 0 || c.Postcode < 0 {
			return searchResult{}, 400, errors.New("Cursor does not match this search")
		}
		afterPostcode = c.Postcode
		afterID = c.ID
		if q.hasAddress() {
			args = append(args, c.ID, c.Dataset)
			conditions = append(conditions, fmt.Sprintf("(id,dataset_id)>($%d,$%d)", len(args)-1, len(args)))
		} else if q.Postcode != "" || q.Near != "" {
			args = append(args, c.Postcode, c.Name, c.First, c.ID, c.Dataset)
			conditions = append(conditions, fmt.Sprintf("(postcode_id,name_id,first_id,id,dataset_id)>($%d,$%d,$%d,$%d,$%d)", len(args)-4, len(args)-3, len(args)-2, len(args)-1, len(args)))
		} else {
			args = append(args, c.Name, c.First, c.ID, c.Dataset)
			conditions = append(conditions, fmt.Sprintf("(name_id,first_id,id,dataset_id)>($%d,$%d,$%d,$%d)", len(args)-3, len(args)-2, len(args)-1, len(args)))
		}
	}
	if areaOnly {
		sort.Slice(nearbyIDs, func(i, j int) bool { return nearbyIDs[i] < nearbyIDs[j] })
		nearbyIDs = nearbyIDs[sort.Search(len(nearbyIDs), func(i int) bool { return nearbyIDs[i] >= afterPostcode }):]
		if len(nearbyIDs) == 0 {
			missing = true
		}
	}
	ids := []int32{}
	recordDatasets := []int16{}
	recordPostcodes := []int32{}
	next := ""
	var last cursor
	positions := []cursor{}
	// Radius-only searches traverse postcode-ID batches in the same order as
	// their keyset cursor. Later batches cannot precede this page's candidates.
	for batchStart := 0; !missing; {
		queryArgs := append([]any(nil), args...)
		if areaOnly {
			end := min(batchStart+512, len(nearbyIDs))
			if batchStart >= end {
				break
			}
			queryArgs[radiusArg] = nearbyIDs[batchStart:end]
			batchStart = end
		}
		args := queryArgs
		args = append(args, q.Limit-len(ids)+1)
		limitArg := len(args)
		branches := []string{}
		// Each edition contributes at most limit+1 candidates. A global top-N is
		// then exact without sorting every matching occurrence in the catalogue.
		for _, dataset := range datasetIDs {
			args = append(args, dataset)
			datasetArg := len(args)
			if q.hasAddress() && !f.addressMatched {
				predicate := addressPredicate(q, &args)
				args = append(args, afterID)
				afterArg := len(args)
				addressIDs := fmt.Sprintf(`SELECT candidate FROM address_members m JOIN address_locations l ON l.id=m.location_id
      CROSS JOIN LATERAL unnest(m.ids) candidate WHERE m.dataset_id=$%d AND %s
      AND m.ids[cardinality(m.ids)] >= $%d AND candidate >= $%d`, datasetArg, predicate, afterArg, afterArg)
				// Each occurrence belongs to exactly one location tuple: no DISTINCT/hash
				// aggregation is necessary. Preserve candidate ID order before looking up
				// source projections so LIMIT stops the indexed probes after a page.
				if q.Kind != "business" && q.Surname == "" && q.First == "" && q.Organisation == "" && q.Postcode == "" && q.Near == "" {
					branches = append(branches, fmt.Sprintf(`(SELECT e.* FROM (%s ORDER BY candidate) address_ids
      CROSS JOIN LATERAL (SELECT dataset_id,id,name_id,first_id,postcode_id FROM entries
       WHERE %s AND dataset_id=$%d AND id=address_ids.candidate OFFSET 0) e
      ORDER BY address_ids.candidate LIMIT $%d)`, addressIDs, strings.Join(conditions, " AND "), datasetArg, limitArg))
				} else {
					name := fmt.Sprintf("address_source_%d", dataset)
					// Keep source candidates index-only. Fetching postcode/projection heaps
					// for every national name match defeats selective address intersection.
					termCTEs = append(termCTEs, fmt.Sprintf(`%s AS MATERIALIZED (SELECT id FROM entries WHERE %s AND dataset_id=$%d)`, name, strings.Join(conditions, " AND "), datasetArg))
					branches = append(branches, fmt.Sprintf(`(SELECT e.* FROM
      (SELECT id FROM %s WHERE id IN (%s) ORDER BY id LIMIT $%d) chosen
      CROSS JOIN LATERAL (SELECT dataset_id,id,name_id,first_id,postcode_id FROM entries WHERE dataset_id=$%d AND id=chosen.id OFFSET 0) e
      ORDER BY chosen.id)`, name, addressIDs, limitArg, datasetArg))
				}

			} else {
				branches = append(branches, fmt.Sprintf("(SELECT dataset_id,id,name_id,first_id,postcode_id FROM entries WHERE %s AND dataset_id=$%d ORDER BY %s LIMIT $%d)", strings.Join(conditions, " AND "), datasetArg, order, limitArg))
			}

		}

		sql := fmt.Sprintf("SELECT * FROM (%s) candidates ORDER BY %s LIMIT $%d", strings.Join(branches, " UNION ALL "), order, limitArg)
		if len(termCTEs) > 0 {
			sql = "WITH " + strings.Join(termCTEs, ", ") + " " + sql
		}
		var rows pgx.Rows
		finish := func() {}
		var e error
		if q.hasAddress() {
			rows, finish, e = a.addressRows(ctx, sql, args)
		} else {
			rows, e = a.DB.Query(ctx, sql, args...)
		}
		if e != nil {
			log.Printf("Search query failed: %v", e)
			return searchResult{}, 503, errors.New("Search unavailable or timed out; narrow the search")
		}
		for rows.Next() {
			var id, name, first, postcode int32
			var dataset int16
			if err = rows.Scan(&dataset, &id, &name, &first, &postcode); err != nil {
				rows.Close()
				finish()
				return searchResult{}, 503, errors.New("Search unavailable")
			}
			if len(ids) == q.Limit {
				b, _ := json.Marshal(last)
				next = base64.RawURLEncoding.EncodeToString(b)
				break
			}
			ids = append(ids, id)
			recordDatasets = append(recordDatasets, dataset)
			recordPostcodes = append(recordPostcodes, postcode)
			last = cursor{Query: q.fingerprint(), Name: name, First: first, ID: id, Dataset: dataset, Postcode: postcode}
			positions = append(positions, last)
		}
		err = rows.Err()
		rows.Close()
		finish()
		if err != nil {
			return searchResult{}, 503, errors.New("Search timed out; narrow the search")
		}
		if next != "" || !areaOnly || batchStart >= len(nearbyIDs) {
			break
		}
	}
	// Batch source blocks by edition, then restore the interleaved search order.
	type recordKey struct {
		dataset int16
		id      int32
	}
	requestedRecords := map[int16][]int32{}
	for i, id := range ids {
		requestedRecords[recordDatasets[i]] = append(requestedRecords[recordDatasets[i]], id)
	}
	foundRecords := map[recordKey]result{}
	for dataset, requestedIDs := range requestedRecords {
		records, e := a.loadRecords(ctx, dataset, requestedIDs)
		if e != nil {
			return searchResult{}, 503, sourceReadError(e)
		}
		for _, record := range records {
			record.Dataset = datasetKeys[dataset]
			foundRecords[recordKey{dataset, int32(record.ID)}] = record
		}
	}
	items := make([]result, 0, len(ids))
	for i, id := range ids {
		item := foundRecords[recordKey{recordDatasets[i], id}]
		if q.Near != "" {
			location := distances[recordPostcodes[i]]
			item.Distance = &location.Distance
			item.LocationQuality = location.Quality
			item.LocationTerminated = location.Terminated
		}
		items = append(items, item)
	}
	return searchResult{Items: items, Next: next, Geography: geography, positions: positions}, 200, nil
}

func (a *API) loadRecords(ctx context.Context, dataset int16, ids []int32) (out []result, loadErr error) {
	ctx, span := telemetry.Operation(ctx, "records.load")
	defer func() { telemetry.End(span, loadErr) }()
	resultRows := make([]result, 0, len(ids))
	if len(ids) == 0 {
		return resultRows, nil
	}
	groups := map[int32][]int32{}
	blockIDs := []int32{}
	cache := recordCache(ctx)
	found := make(map[int32]storedRecord, len(ids))
	for _, id := range ids {
		block := (id - 1) / blockRows
		if raw := cache.get(recordBlockKey{dataset, block}); raw != nil {
			record, err := recordAt(raw, int((id-1)%blockRows))
			if err != nil {
				return nil, err
			}
			found[id] = record
			continue
		}
		if _, ok := groups[block]; !ok {
			blockIDs = append(blockIDs, block)
		}
		groups[block] = append(groups[block], id)
	}
	if len(blockIDs) > 0 {
		rows, e := a.DB.Query(ctx, "SELECT id,payload FROM record_blocks WHERE dataset_id=$1 AND id=ANY($2::integer[])", dataset, blockIDs)
		if e != nil {
			return nil, e
		}
		defer rows.Close()
		for rows.Next() {
			if e = ctx.Err(); e != nil {
				return nil, e
			}
			var block int32
			var compressed []byte
			if e = rows.Scan(&block, &compressed); e != nil {
				return nil, e
			}
			raw, err := unpackBlock(a.decoder, compressed)
			if err != nil {
				return nil, err
			}
			cache.put(recordBlockKey{dataset, block}, raw)
			if cache != nil && cache.observe != nil {
				cache.observe.decodedBlocks.Add(1)
			}
			for _, id := range groups[block] {
				r, err := recordAt(raw, int((id-1)%blockRows))
				if err != nil {
					return nil, err
				}
				found[id] = r
			}
		}
		if e = rows.Err(); e != nil {
			return nil, e
		}
		rows.Close()
	}
	var manifest Manifest
	var rawManifest []byte
	if e := a.DB.QueryRow(ctx, "SELECT manifest FROM datasets WHERE id=$1", dataset).Scan(&rawManifest); e != nil {
		return nil, e
	}
	if e := json.Unmarshal(rawManifest, &manifest); e != nil {
		return nil, recordIntegrity("invalid stored manifest")
	}
	archive := archiveIdentity(manifest)

	for _, id := range ids {
		r, ok := found[id]
		if !ok {
			return nil, recordIntegrity("missing source record block")
		}
		resultRows = append(resultRows, result{ID: int64(id), SourceID: r.SourceID, Record: r.Record, Archive: archive})
	}
	return resultRows, nil
}
