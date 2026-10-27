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
	sql    string
	args   []any
	start  time.Time
	ms     float64
	target bool
}
type tracer struct {
	sync.Mutex
	enabled bool
	calls   []*call
	target  bool
}
type key struct{}

func (t *tracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	t.Lock()
	defer t.Unlock()
	if !t.enabled {
		return ctx
	}
	c := &call{sql: d.SQL, args: append([]any(nil), d.Args...), start: time.Now(), target: t.target}
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
	HeapFetches   float64 `json:"heap_fetches"`
	Filtered      float64 `json:"rows_removed_filter"`
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
	Pool       string  `json:"pool"`
	Phase      string  `json:"phase"`
	Case       int     `json:"case"`
	Rank       int     `json:"rank"`
	Status     int     `json:"status"`
	RequestMS  float64 `json:"request_ms"`
	OriginalMS float64 `json:"original_ms"`
	ExplainMS  float64 `json:"explain_ms"`
	Outcome    string  `json:"outcome"`
	JITMS      float64 `json:"jit_ms"`
	Nodes      []node  `json:"nodes"`
}

func summarize(root map[string]any) []node {
	out := []node{}
	var walk func(map[string]any)
	num := func(m map[string]any, k string) float64 { v, _ := m[k].(float64); return v }
	walk = func(m map[string]any) {
		kind, _ := m["Node Type"].(string)
		out = append(out, node{num(m, "Heap Fetches"), num(m, "Rows Removed by Filter"), kind, num(m, "Actual Rows"), num(m, "Actual Loops"), num(m, "Actual Total Time"), num(m, "Shared Hit Blocks"), num(m, "Shared Read Blocks"), num(m, "Temp Read Blocks") + num(m, "Temp Written Blocks"), num(m, "Plan Rows"), num(m, "Total Cost")})
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
	analysisMS := flag.Int("analysis-ms", 10000, "private diagnostic statement budget, 1000..60000 ms; API deadlines stay unchanged")
	backend := flag.String("backend", "native", "native or paradedb; explicit SELECT-only DSNs required")
	flag.Parse()
	if *backend != "native" && *backend != "paradedb" {
		return errors.New("invalid diagnostic backend")
	}
	if *analysisMS < 1000 || *analysisMS > 60000 {
		return errors.New("invalid diagnostic statement budget")
	}
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
	cfg.MaxConns = 8
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "2000"
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
	a := catalog.NewAPI(db)
	var target *pgxpool.Pool
	targetTrace := &tracer{target: true}
	if *backend == "paradedb" {
		targetConfig, e := pgxpool.ParseConfig(os.Getenv("TARGET_DATABASE_URL"))
		if e != nil || os.Getenv("TARGET_DATABASE_URL") == "" {
			return errors.New("explicit private target reader required")
		}
		targetConfig.MaxConns = 4
		targetConfig.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
		targetConfig.ConnConfig.RuntimeParams["statement_timeout"] = "4000"
		targetConfig.ConnConfig.Tracer = targetTrace
		target, e = pgxpool.NewWithConfig(ctx, targetConfig)
		if e != nil {
			return errors.New("target reader unavailable")
		}
		defer target.Close()
		a.ParadeDB = target
	}
	handler := a.Handler()
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
		targetTrace.Lock()
		targetTrace.calls = nil
		targetTrace.enabled = true
		targetTrace.Unlock()
		w := &discard{header: make(http.Header)}
		requestStart := time.Now()
		handler.ServeHTTP(w, httptest.NewRequest("GET", c.Path+"?"+c.Query, nil).WithContext(ctx))
		requestMS := float64(time.Since(requestStart).Microseconds()) / 1000
		tr.Lock()
		tr.enabled = false
		calls := append([]*call(nil), tr.calls...)
		tr.Unlock()
		targetTrace.Lock()
		targetTrace.enabled = false
		calls = append(calls, targetTrace.calls...)
		targetTrace.Unlock()
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
			ob := observation{Case: ci, Rank: rank, Status: w.status, RequestMS: requestMS, OriginalMS: c.ms, Outcome: "bounded_explain_failed", Nodes: []node{}}
			pool := db
			ob.Pool, ob.Phase = "source", diagnosticPhase(c.sql)
			if c.target {
				pool = target
				ob.Pool, ob.Phase = "target", "derived_candidates"
			}
			tx, e := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
			if e != nil {
				return errors.New("diagnostic transaction unavailable")
			}
			_, e = tx.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout='%d'; SET LOCAL work_mem='32MB'", *analysisMS))
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
					if jit, ok := plans[0]["JIT"].(map[string]any); ok {
						if timing, ok := jit["Timing"].(map[string]any); ok {
							ob.JITMS, _ = timing["Total"].(float64)
						}
					}
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

func diagnosticPhase(sql string) string {
	sql = strings.ToLower(sql)
	for _, phase := range []struct{ fragment, label string }{{"from record_blocks", "records"}, {"postcode_locations", "geography"}, {"ordered_candidates", "ordered"}, {"address_members", "address"}, {"from entries", "source_candidates"}, {"from terms", "vocabulary"}} {
		if strings.Contains(sql, phase.fragment) {
			return phase.label
		}
	}
	return "inventory"
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
