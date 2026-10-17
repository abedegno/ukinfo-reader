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
)

type API struct {
	DB    *pgxpool.Pool
	slots chan struct{}
}

func NewAPI(db *pgxpool.Pool) *API { return &API{DB: db, slots: make(chan struct{}, 32)} }
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
	Query string `json:"q"`
	Name  string `json:"n"`
	First string `json:"f"`
	ID    int64  `json:"i"`
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
	args := []any{dataset}
	// A validated literal lets PostgreSQL use the partial index even with a generic plan.
	conditions := []string{"dataset_id=$1", "kind='" + q.Kind + "'"}
	add := func(column, value string) {
		if value != "" {
			args = append(args, value)
			conditions = append(conditions, fmt.Sprintf("%s=$%d", column, len(args)))
		}
	}
	add("surname", q.Surname)
	add("first_name", q.First)
	add("organisation", q.Organisation)
	add("postcode", q.Postcode)
	// Exact searches preserve short surnames and avoid unbounded wildcard scans.
	sortName, sortFirst := "surname", "first_name"
	if q.Kind == "business" {
		sortName = "organisation"
		sortFirst = "''::text COLLATE \"C\""
	}
	if q.Cursor != "" {
		b, e := base64.RawURLEncoding.DecodeString(q.Cursor)
		var c cursor
		if e != nil || json.Unmarshal(b, &c) != nil || c.Query != q.fingerprint() || c.ID < 1 {
			fail(w, 400, "Cursor does not match this search")
			return
		}
		args = append(args, c.Name, c.First, c.ID)
		conditions = append(conditions, fmt.Sprintf("(%s,%s,id)>($%d,$%d,$%d)", sortName, sortFirst, len(args)-2, len(args)-1, len(args)))
	}
	args = append(args, q.Limit+1)
	sql := fmt.Sprintf("SELECT id,source_id,record,%s,%s FROM entries WHERE %s ORDER BY %s,%s,id LIMIT $%d", sortName, sortFirst, strings.Join(conditions, " AND "), sortName, sortFirst, len(args))
	rows, err := a.DB.Query(ctx, sql, args...)
	if err != nil {
		fail(w, 503, "Search unavailable or timed out; narrow the search")
		return
	}
	defer rows.Close()
	items := []result{}
	next := ""
	var last cursor
	for rows.Next() {
		var item result
		var name, first string
		if err = rows.Scan(&item.ID, &item.SourceID, &item.Record, &name, &first); err != nil {
			fail(w, 503, "Search unavailable")
			return
		}
		if len(items) == q.Limit {
			b, _ := json.Marshal(last)
			next = base64.RawURLEncoding.EncodeToString(b)
			break
		}
		items = append(items, item)
		last = cursor{Query: q.fingerprint(), Name: name, First: first, ID: item.ID}
	}
	if rows.Err() != nil {
		fail(w, 503, "Search timed out; narrow the search")
		return
	}
	respond(w, 200, struct {
		Items []result `json:"items"`
		Next  string   `json:"next_cursor"`
	}{items, next})
}
