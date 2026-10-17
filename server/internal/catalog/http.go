package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/klauspost/compress/zstd"
)

type API struct {
	DB      *pgxpool.Pool
	slots   chan struct{}
	decoder *zstd.Decoder
}

func NewAPI(db *pgxpool.Pool) *API {
	decoder, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(16<<20))
	if err != nil {
		panic(err)
	}
	return &API{DB: db, slots: make(chan struct{}, 32), decoder: decoder}
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, message string) {
	respond(w, status, map[string]string{"error": message})
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if err := a.DB.Ping(ctx); err != nil {
			fail(w, 503, "Database unavailable")
			return
		}
		respond(w, 200, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/datasets", a.datasets)
	mux.HandleFunc("GET /api/search", a.search)
	mux.HandleFunc("GET /api/export", func(w http.ResponseWriter, r *http.Request) { a.searchPage(w, r, true) })
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		select {
		case a.slots <- struct{}{}:
			defer func() { <-a.slots }()
		default:
			fail(w, 503, "Search capacity reached; retry shortly")
			return
		}
		mux.ServeHTTP(w, r)
	})
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
	Dataset, Kind, Surname, First, Organisation, Postcode string
	Limit                                                 int
	Cursor                                                string
}
type cursor struct {
	Query   string `json:"q"`
	Name    int32  `json:"n"`
	First   int32  `json:"f"`
	ID      int32  `json:"i"`
	Dataset int16  `json:"d"`
}
type result struct {
	ID       int64           `json:"id"`
	SourceID string          `json:"source_id"`
	Record   json.RawMessage `json:"record"`
}

func parse(v url.Values) (query, error) {
	q := query{Dataset: strings.TrimSpace(v.Get("dataset")), Kind: v.Get("kind"), Surname: Normalize(v.Get("surname")), First: Normalize(v.Get("first_name")), Organisation: Normalize(v.Get("organisation")), Postcode: Postcode(v.Get("postcode")), Limit: 50, Cursor: v.Get("cursor")}
	if q.Kind == "" {
		q.Kind = "person"
	}
	if q.Kind != "person" && q.Kind != "business" {
		return q, fmt.Errorf("kind must be person or business")
	}
	if q.Dataset == "" {
		return q, fmt.Errorf("choose an imported edition")
	}
	if q.Kind == "person" && (q.Organisation != "" || (q.Surname == "" && q.Postcode == "")) {
		return q, fmt.Errorf("enter a surname or postcode for a people search")
	}
	if q.Kind == "business" && (q.Surname != "" || q.First != "" || (q.Organisation == "" && q.Postcode == "")) {
		return q, fmt.Errorf("enter a business name or postcode")
	}
	for _, s := range []string{q.Dataset, q.Surname, q.First, q.Organisation, q.Postcode} {
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
		fail(w, 400, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	var dataset int16
	if err = a.DB.QueryRow(ctx, "SELECT id FROM datasets WHERE key=$1", q.Dataset).Scan(&dataset); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, 404, "Edition not imported")
		} else {
			fail(w, 503, "Catalogue unavailable")
		}
		return
	}
	kind := int16(0)
	if q.Kind == "business" {
		kind = 1
	}
	requested := []string{}
	for _, key := range []string{q.Surname, q.First, q.Organisation, q.Postcode} {
		if key != "" {
			requested = append(requested, key)
		}
	}
	terms := map[string]int32{}
	termRows, err := a.DB.Query(ctx, "SELECT key,id FROM terms WHERE key=ANY($1::text[])", requested)
	if err != nil {
		fail(w, 503, "Search vocabulary unavailable")
		return
	}
	for termRows.Next() {
		var key string
		var id int32
		if err = termRows.Scan(&key, &id); err != nil {
			termRows.Close()
			fail(w, 503, "Search vocabulary unavailable")
			return
		}
		terms[key] = id
	}
	err = termRows.Err()
	termRows.Close()
	if err != nil {
		fail(w, 503, "Search vocabulary unavailable")
		return
	}
	args := []any{dataset, kind}
	conditions := []string{"dataset_id=$1", "kind=$2"}
	missing := false
	add := func(column, key string) {
		if key != "" {
			id, ok := terms[key]
			if !ok {
				missing = true
			}
			args = append(args, id)
			conditions = append(conditions, fmt.Sprintf("%s=$%d", column, len(args)))
		}
	}
	add("name_id", q.Surname)
	add("first_id", q.First)
	add("name_id", q.Organisation)
	add("postcode_id", q.Postcode)
	if q.Cursor != "" {
		b, e := base64.RawURLEncoding.DecodeString(q.Cursor)
		var c cursor
		if e != nil || json.Unmarshal(b, &c) != nil || c.Query != q.fingerprint() || c.Dataset != dataset || c.ID < 1 || c.Name < 0 || c.First < 0 {
			fail(w, 400, "Cursor does not match this search")
			return
		}
		args = append(args, c.Name, c.First, c.ID)
		conditions = append(conditions, fmt.Sprintf("(name_id,first_id,id)>($%d,$%d,$%d)", len(args)-2, len(args)-1, len(args)))
	}
	ids := []int32{}
	next := ""
	var last cursor
	if !missing {
		args = append(args, q.Limit+1)
		sql := fmt.Sprintf("SELECT id,name_id,first_id FROM entries WHERE %s ORDER BY name_id,first_id,id LIMIT $%d", strings.Join(conditions, " AND "), len(args))
		rows, e := a.DB.Query(ctx, sql, args...)
		if e != nil {
			fail(w, 503, "Search unavailable or timed out; narrow the search")
			return
		}
		for rows.Next() {
			var id, name, first int32
			if err = rows.Scan(&id, &name, &first); err != nil {
				rows.Close()
				fail(w, 503, "Search unavailable")
				return
			}
			if len(ids) == q.Limit {
				b, _ := json.Marshal(last)
				next = base64.RawURLEncoding.EncodeToString(b)
				break
			}
			ids = append(ids, id)
			last = cursor{Query: q.fingerprint(), Name: name, First: first, ID: id, Dataset: dataset}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			fail(w, 503, "Search timed out; narrow the search")
			return
		}
	}
	items, e := a.loadRecords(ctx, dataset, ids)
	if e != nil {
		fail(w, 503, "Source records unavailable; catalogue needs verification")
		return
	}
	if export {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Content-Disposition", `attachment; filename="ukinfo-page.jsonl"`)
		encoder := json.NewEncoder(w)
		for _, item := range items {
			if err := encoder.Encode(struct {
				Dataset  string          `json:"dataset"`
				SourceID string          `json:"source_id"`
				Record   json.RawMessage `json:"record"`
			}{q.Dataset, item.SourceID, item.Record}); err != nil {
				return
			}
		}
		return
	}
	respond(w, 200, struct {
		Items []result `json:"items"`
		Next  string   `json:"next_cursor"`
	}{items, next})
}

func (a *API) loadRecords(ctx context.Context, dataset int16, ids []int32) ([]result, error) {
	resultRows := make([]result, 0, len(ids))
	if len(ids) == 0 {
		return resultRows, nil
	}
	groups := map[int32][]int32{}
	blockIDs := []int32{}
	for _, id := range ids {
		block := (id - 1) / blockRows
		if _, ok := groups[block]; !ok {
			blockIDs = append(blockIDs, block)
		}
		groups[block] = append(groups[block], id)
	}
	rows, e := a.DB.Query(ctx, "SELECT id,payload FROM record_blocks WHERE dataset_id=$1 AND id=ANY($2::integer[])", dataset, blockIDs)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	found := make(map[int32]storedRecord, len(ids))
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
	for _, id := range ids {
		r, ok := found[id]
		if !ok {
			return nil, fmt.Errorf("missing source record block")
		}
		resultRows = append(resultRows, result{ID: int64(id), SourceID: r.SourceID, Record: r.Record})
	}
	return resultRows, nil
}
