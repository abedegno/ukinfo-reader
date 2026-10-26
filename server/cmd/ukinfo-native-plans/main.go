// Private, read-only query-plan diagnosis. Query text, arguments and raw plans
// exist only in memory on the database host and are never exported or logged.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abedegno/ukinfo-reader/server/internal/catalog"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type call struct {
	sql   string
	args  []any
	start time.Time
	ms    float64
}
type tracer struct {
	sync.Mutex
	enabled bool
	calls   []*call
}
type key struct{}

func (t *tracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	t.Lock()
	defer t.Unlock()
	if !t.enabled {
		return ctx
	}
	c := &call{sql: d.SQL, args: append([]any(nil), d.Args...), start: time.Now()}
	t.calls = append(t.calls, c)
	return context.WithValue(ctx, key{}, c)
}
func (t *tracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if c, ok := ctx.Value(key{}).(*call); ok {
		t.Lock()
		c.ms = float64(time.Since(c.start).Microseconds()) / 1000
		t.Unlock()
	}
}

type discard struct {
	header http.Header
	status int
}

func (d *discard) Header() http.Header { return d.header }
func (d *discard) WriteHeader(s int)   { d.status = s }
func (d *discard) Write(p []byte) (int, error) {
	if d.status == 0 {
		d.status = 200
	}
	return len(p), nil
}

type node struct {
	Type          string  `json:"type"`
	Rows          float64 `json:"rows"`
	Loops         float64 `json:"loops"`
	MS            float64 `json:"ms"`
	Hit           float64 `json:"hit_blocks"`
	Read          float64 `json:"read_blocks"`
	Temp          float64 `json:"temp_blocks"`
	EstimatedRows float64 `json:"estimated_rows"`
	EstimatedCost float64 `json:"estimated_cost"`
}
type observation struct {
	Case       int     `json:"case"`
	Rank       int     `json:"rank"`
	Status     int     `json:"status"`
	OriginalMS float64 `json:"original_ms"`
	ExplainMS  float64 `json:"explain_ms"`
	Outcome    string  `json:"outcome"`
	Nodes      []node  `json:"nodes"`
}

func summarize(root map[string]any) []node {
	out := []node{}
	var walk func(map[string]any)
	num := func(m map[string]any, k string) float64 { v, _ := m[k].(float64); return v }
	walk = func(m map[string]any) {
		kind, _ := m["Node Type"].(string)
		out = append(out, node{kind, num(m, "Actual Rows"), num(m, "Actual Loops"), num(m, "Actual Total Time"), num(m, "Shared Hit Blocks"), num(m, "Shared Read Blocks"), num(m, "Temp Read Blocks") + num(m, "Temp Written Blocks"), num(m, "Plan Rows"), num(m, "Total Cost")})
		children, _ := m["Plans"].([]any)
		for _, c := range children {
			if n, ok := c.(map[string]any); ok {
				walk(n)
			}
		}
	}
	walk(root)
	return out
}
func run() error {
	file := flag.String("cases", "", "NAS-only cases file")
	ords := flag.String("ordinals", "56,91,92,212", "up to 12 private case ordinals")
	output := flag.String("output", "", "new aggregate report")
	estimate := flag.Bool("estimate-only", false, "capture estimated structure without executing EXPLAIN ANALYZE")
	flag.Parse()
	raw, e := os.ReadFile(*file)
	if e != nil {
		return errors.New("cases unavailable")
	}
	var cases []catalog.ParadeTrialCase
	if json.Unmarshal(raw, &cases) != nil {
		return errors.New("invalid cases")
	}
	cfg, e := pgxpool.ParseConfig(os.Getenv("SOURCE_DATABASE_URL"))
	if e != nil || os.Getenv("SOURCE_DATABASE_URL") == "" {
		return errors.New("explicit private reader configuration required")
	}
	cfg.MaxConns = 2
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "4000"
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	tr := &tracer{}
	cfg.ConnConfig.Tracer = tr
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		return errors.New("reader unavailable")
	}
	defer db.Close()
	handler := catalog.NewAPI(db).Handler()
	out := []observation{}
	parts := strings.Split(*ords, ",")
	if len(parts) > 12 {
		return errors.New("at most 12 ordinals")
	}
	for _, part := range parts {
		ci, e := strconv.Atoi(part)
		if e != nil || ci < 0 || ci >= len(cases) {
			return errors.New("invalid ordinal")
		}
		c := cases[ci]
		tr.Lock()
		tr.calls = nil
		tr.enabled = true
		tr.Unlock()
		w := &discard{header: make(http.Header)}
		handler.ServeHTTP(w, httptest.NewRequest("GET", c.Path+"?"+c.Query, nil).WithContext(ctx))
		tr.Lock()
		tr.enabled = false
		calls := append([]*call(nil), tr.calls...)
		tr.Unlock()
		sort.SliceStable(calls, func(i, j int) bool { return calls[i].ms > calls[j].ms })
		rank := 0
		for _, c := range calls {
			prefix := strings.ToUpper(strings.TrimSpace(c.sql))
			if !strings.HasPrefix(prefix, "SELECT") && !strings.HasPrefix(prefix, "WITH") {
				continue
			}
			if rank >= 3 {
				break
			}
			rank++
			ob := observation{Case: ci, Rank: rank, Status: w.status, OriginalMS: c.ms, Outcome: "bounded_explain_failed", Nodes: []node{}}
			tx, e := db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
			if e != nil {
				return errors.New("diagnostic transaction unavailable")
			}
			_, e = tx.Exec(ctx, "SET LOCAL statement_timeout='10000'; SET LOCAL work_mem='32MB'")
			started := time.Now()
			var raw []byte
			if e == nil {
				prefix := "EXPLAIN(ANALYZE,BUFFERS,FORMAT JSON) "
				if *estimate {
					prefix = "EXPLAIN(FORMAT JSON) "
				}
				e = tx.QueryRow(ctx, prefix+c.sql, c.args...).Scan(&raw)
			}
			ob.ExplainMS = float64(time.Since(started).Microseconds()) / 1000
			if e == nil {
				var plans []map[string]any
				if json.Unmarshal(raw, &plans) == nil && len(plans) == 1 {
					root, _ := plans[0]["Plan"].(map[string]any)
					ob.Nodes = summarize(root)
					ob.Outcome = "ok"
					if *estimate {
						ob.Outcome = "estimate_only"
					}
				}
			}
			tx.Rollback(context.Background())
			out = append(out, ob)
		}
	}
	f, e := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return errors.New("new output required")
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(out)
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
