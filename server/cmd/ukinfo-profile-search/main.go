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
	Failed       bool
}
type queryKey struct{}
type tracer struct {
	mu      sync.Mutex
	queries []*query
	explain bool
}

func (t *tracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.explain {
		return ctx
	}
	phase := "metadata"
	if s, ok := trace.SpanFromContext(ctx).(interface{ Name() string }); ok {
		name := strings.TrimPrefix(s.Name(), "search.")
		switch name {
		case "vocabulary", "source_probe", "address_probe", "address_blocks", "address_dictionary", "candidates", "ordered", "group_members", "records":
			phase = name
		}
	}
	q := &query{SQL: d.SQL, Args: d.Args, Started: time.Now(), Phase: phase}
	t.queries = append(t.queries, q)
	return context.WithValue(ctx, queryKey{}, q)
}
func (t *tracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryEndData) {
	if q, ok := ctx.Value(queryKey{}).(*query); ok {
		t.mu.Lock()
		defer t.mu.Unlock()
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
	if f.Parse(args) != nil || *limit < 1 || *limit > 100 || *duration <= 0 || *duration > 15*time.Minute || *slow < 0 || *output == "" {
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
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	otel.SetTracerProvider(provider)
	defer otel.SetTracerProvider(previous)
	defer provider.Shutdown(context.Background())
	api := catalog.NewAPI(db)
	reports := []any{}
	failed := 0
	for i, c := range cases {
		if ctx.Err() != nil {
			break
		}
		t.mu.Lock()
		t.queries = nil
		t.explain = false
		t.mu.Unlock()
		w := httptest.NewRecorder()
		started := time.Now()
		req, requestErr := http.NewRequestWithContext(ctx, "GET", "http://localhost"+c.Path+"?"+c.Query, nil)
		if requestErr != nil {
			return errors.New("invalid private request")
		}
		api.Handler().ServeHTTP(w, req)
		elapsed := time.Since(started)
		status := w.Code
		if status != 200 {
			failed++
		}
		t.mu.Lock()
		steps := append([]*query(nil), t.queries...)
		t.explain = true
		t.mu.Unlock()
		observations := []any{}
		for _, q := range steps {
			row := map[string]any{"phase": q.Phase, "milliseconds": q.Milliseconds, "failed": q.Failed}
			text := strings.ToUpper(strings.TrimSpace(q.SQL))
			if (q.Failed || q.Milliseconds >= float64(*slow)/float64(time.Millisecond)) && (strings.HasPrefix(text, "SELECT ") || strings.HasPrefix(text, "WITH ")) {
				var raw []byte
				if e := db.QueryRow(ctx, "EXPLAIN (FORMAT JSON) "+q.SQL, q.Args...).Scan(&raw); e != nil {
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
		reports = append(reports, map[string]any{"case": i, "status": status, "milliseconds": float64(elapsed) / float64(time.Millisecond), "queries": observations})
	}
	metrics := httptest.NewRecorder()
	api.Metrics().ServeHTTP(metrics, httptest.NewRequest("GET", "/metrics", nil))
	result := map[string]any{"schema_version": 1, "cases_requested": len(cases), "cases_profiled": len(reports), "failed_requests": failed, "budget_exhausted": ctx.Err() != nil, "cases": reports, "metrics": metrics.Body.String(), "notes": []string{"Plans are estimates, not EXPLAIN ANALYZE; failed queries are not re-executed.", "SQL, arguments, predicates, query values, response bodies and raw errors are omitted.", "Uses read-only transactions, eight connections and existing API statement/request deadlines."}}
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
