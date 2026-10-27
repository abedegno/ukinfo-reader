package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMetricsExcludeQueriesAndUnboundedLabels(t *testing.T) {
	config, err := pgxpool.ParseConfig("postgres://invented@127.0.0.1:1/invented?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	config.MinConns = 0
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	api := NewAPI(pool)
	handler := api.metrics.wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failError(w, 503, searchReadError(context.DeadlineExceeded))
	}))
	for _, path := range []string{"/api/search?surname=PRIVATE_NAME", "/PRIVATE_PATH?source_id=PRIVATE_SOURCE"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
	}
	response := httptest.NewRecorder()
	api.Metrics().ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
	body := response.Body.String()
	if strings.Contains(body, "PRIVATE") || !strings.Contains(body, `route="/api/search",status="503"`) || !strings.Contains(body, `route="other"`) || !strings.Contains(body, "ukinfo_api_active 0") {
		t.Fatal(body)
	}
	for _, required := range []string{
		`ukinfo_api_failures_total{route="/api/search",reason="query_timeout"} 1`,
		"ukinfo_pool_waits_total 0", "ukinfo_pool_wait_seconds_total 0",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("missing %s", required)
		}
	}
	denied := httptest.NewRecorder()
	api.Metrics().ServeHTTP(denied, httptest.NewRequest("GET", "/api/search", nil))
	if denied.Code != 404 {
		t.Fatal(denied.Code)
	}
}

func testHealthIsolation(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	config := db.Config().Copy()
	config.MaxConns, config.MinConns = 1, 0
	health, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer health.Close()
	api := NewAPI(db)
	api.HealthDB = health
	for i := 0; i < cap(api.slots); i++ {
		api.slots <- struct{}{}
	}
	var held []*pgxpool.Conn
	defer func() {
		for _, conn := range held {
			conn.Release()
		}
	}()
	for i := int32(0); i < db.Config().MaxConns; i++ {
		conn, err := db.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, conn)
	}
	handler := api.Handler()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/health", nil))
	if response.Code != 200 {
		t.Fatalf("health shares saturated search resources: %d %s", response.Code, response.Body.String())
	}
	busy := httptest.NewRecorder()
	handler.ServeHTTP(busy, httptest.NewRequest("GET", "/api/search?dataset=invented&surname=Invented", nil))
	if busy.Code != 503 || !strings.Contains(busy.Body.String(), "capacity reached") {
		t.Fatal(busy.Code, busy.Body.String())
	}
}

func TestQueryMetricsBoundedAndCacheReuse(t *testing.T) {
	m := newAPIMetrics()
	for path, want := range map[string]string{
		"/api/search?group=listing&surname=PRIVATE": "grouped",
		"/api/search?sort=name":                     "ordered",
		"/api/search?town=PRIVATE":                  "address",
		"/api/search?near=PRIVATE":                  "nearby",
		"/api/search?postcode=PRIVATE":              "postcode",
		"/api/search?surname=PRIVATE":               "name",
	} {
		if got := queryClass(httptest.NewRequest("GET", path, nil)); got != want {
			t.Fatalf("class=%s want=%s", got, want)
		}
	}
	m.plan("PRIVATE")
	m.plan("source")
	if len(m.plans) != 1 || m.plans["source"] != 1 {
		t.Fatal(m.plans)
	}
	m.plan("paradedb_radius_stream")
	m.observePhase("geography", 0.1, false)
	if m.plans["paradedb_radius_stream"] != 1 || m.phases[phaseKey{"geography", "ok"}].Count != 1 {
		t.Fatal("candidate geography metrics missing")
	}
	ctx := withRecordCache(context.Background())
	cache := recordCache(ctx)
	cache.observe = m
	key := recordBlockKey{dataset: 1, block: 2}
	if cache.get(key) != nil {
		t.Fatal("unexpected cached block")
	}
	cache.put(key, []byte("invented source block"))
	for i := 0; i < 100; i++ {
		if cache.get(key) == nil {
			t.Fatal("request-local block lost")
		}
	}
	if m.cacheMisses.Load() != 1 || m.cacheHits.Load() != 100 {
		t.Fatal("repeated source reads are not being observed correctly")
	}
	if recordCache(withRecordCache(context.Background())).get(key) != nil {
		t.Fatal("source block leaked between requests")
	}
}

func TestSearchPhaseMetricsBounded(t *testing.T) {
	config, _ := pgxpool.ParseConfig("postgres://invented@127.0.0.1:1/invented?sslmode=disable")
	config.MinConns = 0
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	a := NewAPI(pool)
	a.metrics.observePhase("PRIVATE_PHASE", .2, true)
	a.metrics.observePhase("address_dictionary", .1, false)
	w := httptest.NewRecorder()
	a.Metrics().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	body := w.Body.String()
	for _, want := range []string{`ukinfo_search_phase_duration_seconds_count{phase="other",outcome="error"} 1`, `ukinfo_search_phase_duration_seconds_count{phase="address_dictionary",outcome="ok"} 1`, `ukinfo_search_phase_duration_seconds_bucket{phase="address_dictionary",outcome="ok",le="0.1"} 1`} {
		if !strings.Contains(body, want) {
			t.Fatal("missing bounded stage metric", want)
		}
	}
	if strings.Contains(body, "PRIVATE") {
		t.Fatal("arbitrary phase exported")
	}
}
