// Operator-only profiling of private API workloads. No SQL, arguments or records
// are exported; execution uses the real read-only API and existing deadlines.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/abedegno/ukinfo-reader/server/internal/catalog"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type workloadCase struct{ Path, Query string }
type query struct {
	SQL          string
	Args         []any
	Started      time.Time
	Phase        string
	Milliseconds float64
	Target       bool
	Failed       bool
}
type queryKey struct{}
type tracer struct {
	mu      sync.Mutex
	queries []*query
	explain bool
	root    *tracer
	target  bool
}

func (t *tracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	owner := t
	if t.root != nil {
		owner = t.root
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if owner.explain {
		return ctx
	}
	phase := "metadata"
	if span, ok := trace.SpanFromContext(ctx).(interface{ Name() string }); ok {
		phase = profilePhase(span.Name())
	}
	q := &query{SQL: d.SQL, Args: d.Args, Started: time.Now(), Phase: phase, Target: t.target}
	owner.queries = append(owner.queries, q)
	return context.WithValue(ctx, queryKey{}, q)
}
func (t *tracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryEndData) {
	if q, ok := ctx.Value(queryKey{}).(*query); ok {
		owner := t
		if t.root != nil {
			owner = t.root
		}
		owner.mu.Lock()
		defer owner.mu.Unlock()
		q.Milliseconds = float64(time.Since(q.Started)) / float64(time.Millisecond)
		q.Failed = d.Err != nil
	}
}

// Predicates, expressions, output lists and parameter values are deliberately absent.
func cleanPlan(n map[string]any) map[string]any {
	out := map[string]any{}
	for _, k := range []string{"Node Type", "Relation Name", "Index Name", "Plan Rows", "Total Cost", "Actual Rows", "Actual Loops", "Heap Fetches", "Shared Hit Blocks", "Shared Read Blocks", "Temp Read Blocks", "Temp Written Blocks", "Rows Removed by Filter", "Sort Space Type", "Sort Space Used", "Hash Batches"} {
		if v, ok := n[k]; ok {
			out[k] = v
		}
	}
	if children, ok := n["Plans"].([]any); ok {
		a := []any{}
		for _, c := range children {
			a = append(a, cleanPlan(c.(map[string]any)))
		}
		out["children"] = a
	}
	return out
}
func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string) error {
	f := flag.NewFlagSet("ukinfo-profile-search", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	casesPath := f.String("cases", "", "private workload JSON")
	output := f.String("output", "", "aggregate profiling report")
	limit := f.Int("max-cases", 20, "maximum cases (1-100)")
	duration := f.Duration("max-duration", 5*time.Minute, "overall budget (up to 15 minutes)")
	slow := f.Duration("plan-after", 100*time.Millisecond, "explain queries slower than this; failed reads always explained")
	page := f.Int("page", 1, "last cursor page to profile, 1..10")
	ioEnabled := f.Bool("io-stats", false, "private isolated source only: read database-wide IO counters")
	repeats := f.Int("repeats", 1, "bounded repetitions, 1 or 2")
	plans := f.Bool("plans", true, "capture estimated plans after requests; disable for cold/warm profiling")
	backend := f.String("backend", "native", "native or paradedb with explicit TARGET_DATABASE_URL")
	addressProbe := f.Bool("address-probe", false, "enable the opt-in address probe experiment")
	if f.Parse(args) != nil || *page < 1 || *page > 10 || (*limit)*(*repeats)*(*page) > 200 || (*repeats != 1 && *repeats != 2) || (*backend != "native" && *backend != "paradedb") || *limit < 1 || *limit > 100 || *duration <= 0 || *duration > 15*time.Minute || *slow < 0 || *output == "" {
		return errors.New("invalid profiling configuration")
	}
	data, err := os.ReadFile(*casesPath)
	if err != nil {
		return errors.New("cannot read private workload")
	}
	var cases []workloadCase
	if json.Unmarshal(data, &cases) != nil || len(cases) == 0 {
		return errors.New("invalid private workload")
	}
	if len(cases) > *limit {
		return errors.New("workload exceeds case budget; select a private subset first")
	}
	for i := range cases {
		if cases[i].Path == "" {
			cases[i].Path = "/api/search"
		}
		switch cases[i].Path {
		case "/api/search", "/api/export", "/api/group", "/api/address", "/api/record":
		default:
			return errors.New("unsupported workload endpoint")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()
	if os.Getenv("DATABASE_URL") == "" {
		return errors.New("explicit source reader required")
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	if err != nil {
		return errors.New("invalid private database configuration")
	}
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "2000"
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	cfg.MaxConns = 8
	cfg.MinConns = 1
	t := &tracer{}
	cfg.ConnConfig.Tracer = t
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return errors.New("cannot initialize profiling pool")
	}
	defer db.Close()
	// Recording spans supply fixed stage names, with no exporter or network egress.
	previous := otel.GetTracerProvider()
	spans := &phaseRecorder{}
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()), sdktrace.WithSpanProcessor(spans))
	otel.SetTracerProvider(provider)
	defer otel.SetTracerProvider(previous)
	defer provider.Shutdown(context.Background())
	api := catalog.NewAPI(db)

	api.ExperimentalAddressProbe = *addressProbe
	if *backend == "paradedb" {
		dsn := os.Getenv("TARGET_DATABASE_URL")
		if dsn == "" {
			return errors.New("explicit target reader required")
		}
		target, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			return errors.New("invalid target reader")
		}
		target.MaxConns = 4
		target.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
		target.ConnConfig.RuntimeParams["statement_timeout"] = "4000"
		target.ConnConfig.RuntimeParams["plan_cache_mode"] = "force_generic_plan"
		target.ConnConfig.Tracer = &tracer{root: t, target: true}
		api.ParadeDB, err = pgxpool.NewWithConfig(ctx, target)
		if err != nil {
			return errors.New("target pool unavailable")
		}
		defer api.ParadeDB.Close()
	}
	reports := []any{}
	failed := 0
	pageExhausted := false
	for repeat := 1; repeat <= *repeats; repeat++ {
		for i, c := range cases {
			values, e := url.ParseQuery(c.Query)
			if e != nil {
				return errors.New("invalid private query")
			}
			for currentPage := 1; currentPage <= *page; currentPage++ {
				if ctx.Err() != nil {
					break
				}
				t.mu.Lock()
				t.queries = nil
				t.explain = false
				t.mu.Unlock()
				var beforeIO ioStats
				if *ioEnabled {
					t.mu.Lock()
					t.explain = true
					t.mu.Unlock()
					beforeIO = readIO(ctx, db)
				}
				t.mu.Lock()
				t.queries = nil
				t.explain = false
				t.mu.Unlock()
				w := httptest.NewRecorder()
				spans.reset()
				acquired := db.Stat().AcquireDuration()
				empty := db.Stat().EmptyAcquireCount()
				cpu := processCPU()
				started := time.Now()
				req, requestErr := http.NewRequestWithContext(ctx, "GET", "http://localhost"+c.Path+"?"+values.Encode(), nil)
				if requestErr != nil {
					return errors.New("invalid private request")
				}
				api.Handler().ServeHTTP(w, req)
				elapsed := time.Since(started)
				cpu = processCPU().minus(cpu)
				acquireMS := float64(db.Stat().AcquireDuration()-acquired) / float64(time.Millisecond)
				emptyAcquires := db.Stat().EmptyAcquireCount() - empty
				phaseMS := spans.snapshot()
				status := w.Code
				if status != 200 {
					failed++
				}
				t.mu.Lock()
				steps := append([]*query(nil), t.queries...)
				t.explain = true
				t.mu.Unlock()
				var ioDelta ioStats
				if *ioEnabled {
					ioDelta = readIO(ctx, db).minus(beforeIO)
				}
				observations := []any{}
				for _, q := range steps {
					row := map[string]any{"phase": q.Phase, "milliseconds": q.Milliseconds, "failed": q.Failed, "target": q.Target}
					if strings.Contains(q.SQL, ") source_batch ORDER BY id,dataset_id LIMIT") {
						row["access_path"] = "reverse_address_batch"
					}
					text := strings.ToUpper(strings.TrimSpace(q.SQL))
					if *plans && (q.Failed || q.Milliseconds >= float64(*slow)/float64(time.Millisecond)) && (strings.HasPrefix(text, "SELECT ") || strings.HasPrefix(text, "WITH ")) {
						var raw []byte
						pool := db
						if q.Target {
							pool = api.ParadeDB
						}
						if e := pool.QueryRow(ctx, "EXPLAIN (FORMAT JSON) "+q.SQL, q.Args...).Scan(&raw); e != nil {
							row["plan_unavailable"] = true
						} else {
							var p []map[string]any
							if json.Unmarshal(raw, &p) == nil && len(p) > 0 {
								row["plan"] = cleanPlan(p[0]["Plan"].(map[string]any))
								row["jit_planned"] = p[0]["JIT"] != nil
							}
						}
					}
					observations = append(observations, row)
				}
				reports = append(reports, map[string]any{"case": i, "page": currentPage, "status": status, "milliseconds": float64(elapsed) / float64(time.Millisecond), "queries": observations, "repeat": repeat, "phase_ms": phaseMS, "record_query_wall_ms": queryWallMS(steps, "records"), "pool_acquire_ms": acquireMS, "empty_acquires": emptyAcquires, "cpu": cpu, "database_io": ioDelta})
				if status != 200 {
					break
				}
				if currentPage < *page {
					var body struct {
						Next string `json:"next_cursor"`
					}
					if json.Unmarshal(w.Body.Bytes(), &body) != nil || body.Next == "" {
						pageExhausted = true
						break
					}
					values.Set("cursor", body.Next)
				}
			}
		}
	}
	metrics := httptest.NewRecorder()
	api.Metrics().ServeHTTP(metrics, httptest.NewRequest("GET", "/metrics", nil))
	result := map[string]any{"schema_version": 2, "last_page_requested": *page, "page_exhausted": pageExhausted, "cases_requested": len(cases), "cases_profiled": len(reports), "io_stats_enabled": *ioEnabled, "repeats": *repeats, "backend": *backend, "plans_enabled": *plans, "failed_requests": failed, "budget_exhausted": ctx.Err() != nil, "cases": reports, "metrics": metrics.Body.String(), "notes": []string{"Plans are estimates, not EXPLAIN ANALYZE; failed queries are not re-executed.", "SQL, arguments, predicates, query values, response bodies and raw errors are omitted.", "Uses eight source/four target connections and existing API statement/request deadlines; no external span exporter.", "IO counters are database-wide and include stats-query overhead; canceled backends may publish stats asynchronously. Availability is successful observation, not exact attribution. Read timing requires private track_io_timing.", "Acquisition duration is all successful source-pool acquisition time, not pure contention wait; target acquisition is not included.", "Phase spans and query durations overlap. Record-query wall time is their interval union. CPU covers the whole handler; phase residuals are not exact decoder time."}}
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return errors.New("cannot encode aggregate profile")
	}
	if os.WriteFile(*output, append(raw, '\n'), 0600) != nil {
		return errors.New("cannot write aggregate profile")
	}
	fmt.Printf("profiled_cases=%d failures=%d budget_exhausted=%t\n", len(reports), failed, ctx.Err() != nil)
	if ctx.Err() != nil {
		return errors.New("profiling budget exhausted; inspect aggregate report")
	}
	return nil
}
