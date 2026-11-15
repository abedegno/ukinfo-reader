package catalog

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestParallelPostcodeSelection(t *testing.T) {
	for _, input := range []string{"postcode=ZZ9", "postcode=ZZ9+1", "postcode=ZZ9+1AA"} {
		q, err := parse(mustValues("dataset=all&" + input))
		if err != nil || !parallelPostcode(q) {
			t.Fatal("plain person postcode excluded", err)
		}
	}
	for _, input := range []string{
		"postcode=ZZ9&surname=Invented", "postcode=ZZ9&first_name=June", "postcode=ZZ9&middle_initial=A",
		"postcode=ZZ9&town=Invented", "postcode=ZZ9&street=Invented", "postcode=ZZ9&county=Invented",
		"postcode=ZZ9&house=1", "postcode=ZZ9&building=Invented", "postcode=ZZ9&flat=1",
		"postcode=ZZ9&sort=name", "postcode=ZZ9&sort=edition", "postcode=ZZ9&group=listing",
		"postcode=ZZ9&near=ZZ91AA", "kind=business&postcode=ZZ9", "surname=Invented"} {
		q, err := parse(mustValues("dataset=all&" + input))
		if err == nil && parallelPostcode(q) {
			t.Fatal("other filter shape selected", input)
		}
	}
}

type postcodeCancelTracer struct {
	mu      sync.Mutex
	started int
	both    chan struct{}
	cancel  context.CancelFunc
}

func (p *postcodeCancelTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if strings.Contains(d.SQL, "FROM entries WHERE") {
		p.mu.Lock()
		p.started++
		n := p.started
		if n == 2 {
			close(p.both)
			p.cancel()
		}
		p.mu.Unlock()
		select {
		case <-p.both:
		case <-ctx.Done():
		}
	}
	return ctx
}
func (*postcodeCancelTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestParallelPostcodeIntegration(t *testing.T) {
	dsn := os.Getenv("UKINFO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("disposable PostgreSQL 18 required")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	schema := fmt.Sprintf("ukinfo_parallel_postcode_%d", os.Getpid())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.MaxConns = 8
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	originals := map[string][]byte{}
	for edition, n := range []int{0, 1, 5, 25} {
		key := fmt.Sprintf("parallel-postcode-%d", edition)
		var stream bytes.Buffer
		enc := json.NewEncoder(&stream)
		enc.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: key, Edition: fmt.Sprint(edition), Title: "Invented postcode pagination", SHA256: map[string]string{"fixture": "invented"}, Expected: int64(n + 1)})
		for i := 0; i < n; i++ {
			row := map[string]any{"surname": []string{"Alpha", "Beta", "Alpha"}[i%3], "firstName": []string{"June", "", "June"}[i%3], "postcode": []string{"ZZ9 1AA", "ZZ9 1AB", "ZZ9", "ZZ9 1", "ZZ99 1AA", "ZZ9 2AA", "ZZ9 1Åß"}[i%7]}
			raw, _ := json.Marshal(row)
			originals[fmt.Sprintf("%s:%d", key, i+1)] = raw
			enc.Encode(map[string]any{"type": "record", "source_id": fmt.Sprintf("person-%d", i), "record": row})
		}
		enc.Encode(map[string]any{"type": "record", "source_id": "business", "record": map[string]any{"entry_type": "business", "organisation": "Invented Only", "postcode": "ZZ9 1AA"}})
		enc.Encode(map[string]any{"type": "end", "count": n + 1})
		if _, err = Import(ctx, db, &stream); err != nil {
			t.Fatal(err)
		}
	}
	type expectedPoint struct {
		key string
		c   cursor
	}
	references := map[string][]expectedPoint{}
	for _, postcode := range []string{"ZZ9", "ZZ9 1", "ZZ9 1AA"} {
		q, err := parse(url.Values{"dataset": {"all"}, "postcode": {postcode}})
		if err != nil {
			t.Fatal(err)
		}
		predicate := "p.key=$1"
		args := []any{q.Postcode}
		if q.PostcodeLength > 0 {
			predicate = "p.key >= $1 AND p.key < $2 AND (length(p.key)=$3 OR p.key=$1)"
			args = append(args, q.Postcode+"\U0010ffff", q.PostcodeLength)
		}
		rows, err := db.Query(ctx, `SELECT d.key,e.dataset_id,e.id,e.postcode_id,e.name_id,e.first_id FROM entries e JOIN datasets d ON d.id=e.dataset_id JOIN terms p ON p.id=e.postcode_id WHERE e.kind=0 AND `+predicate+` ORDER BY e.postcode_id,e.name_id,e.first_id,e.id,e.dataset_id`, args...)
		if err != nil {
			t.Fatal(err)
		}
		want := []expectedPoint{}
		for rows.Next() {
			var p expectedPoint
			if err = rows.Scan(&p.key, &p.c.Dataset, &p.c.ID, &p.c.Postcode, &p.c.Name, &p.c.First); err != nil {
				t.Fatal(err)
			}
			want = append(want, p)
		}
		err = rows.Err()
		rows.Close()
		if err != nil || len(want) < 3 {
			t.Fatal("independent population", len(want), err)
		}
		references[postcode] = want
		for _, limit := range []int{1, 2, 7, 50, len(want)} {
			api := NewAPI(db)
			v := url.Values{"dataset": {"all"}, "postcode": {postcode}, "limit": {fmt.Sprint(limit)}}
			parsed, err := parse(v)
			if err != nil {
				t.Fatal(err)
			}
			f, status, err := api.candidateFilters(ctx, parsed)
			if err != nil || status != 200 {
				t.Fatal(err)
			}
			offset := 0
			for page := 0; page < 100; page++ {
				w := httptest.NewRecorder()
				api.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?"+v.Encode(), nil))
				var out searchResult
				if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
					t.Fatal("invented postcode page failed", w.Code)
				}
				expectedSize := min(limit, len(want)-offset)
				if len(out.Items) != expectedSize {
					t.Fatal("page lost or invented records", len(out.Items), expectedSize)
				}
				for i, item := range out.Items {
					p := want[offset+i]
					identity := fmt.Sprintf("%s:%d", item.Dataset, item.ID)
					if item.Dataset != p.key || item.ID != int64(p.c.ID) || item.SourceID != fmt.Sprintf("person-%d", p.c.ID-1) || !bytes.Equal(item.Record, originals[identity]) {
						t.Fatal("source order, identity or bytes changed")
					}
				}
				offset += len(out.Items)
				if offset == len(want) {
					if out.Next != "" {
						t.Fatal("false continuation after exact exhaustion")
					}
					break
				}
				if len(out.Items) == 0 || out.Next == "" {
					t.Fatal("false page exhaustion")
				}
				expected := want[offset-1].c
				expected.Query = f.q.fingerprint()
				raw, _ := json.Marshal(expected)
				if out.Next != base64.RawURLEncoding.EncodeToString(raw) {
					t.Fatal("native cursor contract changed")
				}
				v.Set("cursor", out.Next)
			}
			if offset != len(want) || api.metrics.plans["source_postcode_parallel"] == 0 {
				t.Fatal("parallel path or complete pagination missing")
			}
		}
	}
	// Single-edition searches and business searches retain their existing readers.
	single := NewAPI(db)
	if got := paradeWalk(t, single, single, "dataset=parallel-postcode-1&postcode=ZZ9&limit=1"); len(got) != 1 || single.metrics.plans["source_postcode_parallel"] != 0 {
		t.Fatal("single edition routing changed")
	}
	business := NewAPI(db)
	if got := paradeWalk(t, business, business, "dataset=all&kind=business&postcode=ZZ9&limit=2"); len(got) != 4 || business.metrics.plans["source_postcode_parallel"] != 0 {
		t.Fatal("business routing changed")
	}
	// Both branches have acquired distinct connections before cancellation. No
	// partial result may survive and both connections must return to the pool.
	canceled, cancel := context.WithCancel(ctx)
	defer cancel()
	traced := db.Config()
	tracer := &postcodeCancelTracer{both: make(chan struct{}), cancel: cancel}
	traced.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(ctx, traced)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	failed := NewAPI(pool)
	w := httptest.NewRecorder()
	failed.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?dataset=all&postcode=ZZ9+1AA&limit=2", nil).WithContext(canceled))
	if w.Code != 503 || strings.Contains(w.Body.String(), `"items"`) {
		t.Fatal("canceled branches returned partial results")
	}
	tracer.mu.Lock()
	started := tracer.started
	tracer.mu.Unlock()
	if started < 2 {
		t.Fatal("cancellation did not hold both source connections")
	}
	waitForPoolIdle(t, pool)
	// Unused dictionary rows deliberately force the complete shared-CTE route.
	// They must not cause truncation or be copied into per-edition SQL without it.
	if _, err = db.Exec(ctx, `INSERT INTO terms(key) SELECT 'ZZ9'||chr(65+(n/676)%26)||chr(65+(n/26)%26)||chr(65+n%26) FROM generate_series(0,8299) n ON CONFLICT(key) DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	broad := NewAPI(db)
	got := paradeWalk(t, broad, broad, "dataset=all&postcode=ZZ9&limit=2")
	want := []string{}
	actual := []string{}
	for _, p := range references["ZZ9"] {
		want = append(want, fmt.Sprintf("%s:%d", p.key, p.c.ID))
	}
	for _, item := range got {
		actual = append(actual, fmt.Sprintf("%s:%d", item.Dataset, item.ID))
	}
	if !reflect.DeepEqual(actual, want) || broad.metrics.plans["source_postcode_parallel"] != 0 {
		t.Fatal("shared-CTE fallback changed population or routing")
	}
}
