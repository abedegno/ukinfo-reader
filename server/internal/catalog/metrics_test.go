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
	handler := api.metrics.wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	for _, path := range []string{"/api/search?surname=PRIVATE_NAME", "/PRIVATE_PATH?source_id=PRIVATE_SOURCE"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
	}
	response := httptest.NewRecorder()
	api.Metrics().ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
	body := response.Body.String()
	if strings.Contains(body, "PRIVATE") || !strings.Contains(body, `route="/api/search",status="503"`) || !strings.Contains(body, `route="other"`) || !strings.Contains(body, "ukinfo_api_active 0") {
		t.Fatal(body)
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
