package catalog

import (
	"fmt"
	"github.com/abedegno/ukinfo-reader/server/internal/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type requestMetric struct {
	Count   uint64
	Seconds float64
	Buckets [7]uint64
}
type metricKey struct {
	Route  string
	Status int
}
type queryMetricKey struct {
	Class  string
	Status int
}
type failureMetricKey struct {
	Route  string
	Reason string
}
type apiMetrics struct {
	mu            sync.Mutex
	requests      map[metricKey]requestMetric
	active        int
	plans         map[string]uint64
	classes       map[queryMetricKey]uint64
	failures      map[failureMetricKey]uint64
	cacheHits     atomic.Uint64
	cacheMisses   atomic.Uint64
	decodedBlocks atomic.Uint64
}

var latencyBounds = [...]float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5}

func newAPIMetrics() *apiMetrics {
	return &apiMetrics{requests: make(map[metricKey]requestMetric), plans: map[string]uint64{}, classes: map[queryMetricKey]uint64{}, failures: map[failureMetricKey]uint64{}}
}
func metricRoute(path string) string {
	switch path {
	case "/api/research", "/api/group", "/api/search", "/api/export", "/api/record", "/api/address", "/api/map", "/api/datasets", "/api/addresses", "/api/geography", "/api/health":
		return path
	}
	return "other"
}

func queryClass(r *http.Request) string {
	q := r.URL.Query()
	if q.Get("group") == "listing" {
		return "grouped"
	}
	if q.Get("sort") != "" && q.Get("sort") != "catalogue" {
		return "ordered"
	}
	for _, key := range []string{"street", "locality", "town", "county"} {
		if q.Get(key) != "" {
			return "address"
		}
	}
	if q.Get("near") != "" {
		return "nearby"
	}
	if q.Get("postcode") != "" {
		return "postcode"
	}
	return "name"
}
func (m *apiMetrics) plan(name string) {
	switch name {
	case "address", "source", "indexed_source", "fallback", "error":
	default:
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.plans[name]++
}

type metricWriter struct {
	http.ResponseWriter
	status  int
	failure string
}

// Failure labels come from structured response codes, never error text.
func (w *metricWriter) setFailureCode(code string) {
	switch code {
	case "query_timeout", "catalogue_integrity":
		w.failure = code
	default:
		w.failure = "unavailable"
	}
}

func (w *metricWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
		w.ResponseWriter.WriteHeader(status)
	}
}
func (w *metricWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(b)
}
func (m *apiMetrics) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		// Use only allowlisted route/class values. Deliberately ignore incoming
		// traceparent and baggage: public visitors cannot force sampling or inject data.
		route := metricRoute(r.URL.Path)
		method := r.Method
		switch method {
		case "GET", "POST", "HEAD", "OPTIONS":
		default:
			method = "OTHER"
		}
		ctx, span := otel.Tracer("ukinfo/http").Start(r.Context(), "HTTP "+route, trace.WithNewRoot(), trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(attribute.String("http.route", route), attribute.String("http.request.method", method)))
		if route == "/api/search" || route == "/api/export" {
			span.SetAttributes(attribute.String("ukinfo.query.class", queryClass(r)))
		}
		r = r.WithContext(ctx)
		writer := &metricWriter{ResponseWriter: w}
		m.mu.Lock()
		m.active++
		m.mu.Unlock()
		defer func() {
			elapsed := time.Since(started).Seconds()
			status := writer.status
			if status == 0 {
				status = 200
			}
			span.SetAttributes(attribute.Int("http.response.status_code", status))
			if status >= 500 {
				span.SetStatus(codes.Error, "Request failed")
			}
			telemetry.LogRequest(r.Context(), route, method, queryClass(r), w.Header().Get("X-Request-ID"), writer.failure, status, time.Since(started))
			span.End()
			m.mu.Lock()
			defer m.mu.Unlock()
			m.active--
			key := metricKey{metricRoute(r.URL.Path), status}
			item := m.requests[key]
			item.Count++
			item.Seconds += elapsed
			for i, bound := range latencyBounds {
				if elapsed <= bound {
					item.Buckets[i]++
				}
			}
			m.requests[key] = item
			if status >= 500 {
				reason := writer.failure
				if reason == "" {
					reason = "unavailable"
				}
				m.failures[failureMetricKey{key.Route, reason}]++
			}
			if key.Route == "/api/search" || key.Route == "/api/export" {
				class := queryClass(r)
				m.classes[queryMetricKey{class, status}]++
				if status >= 500 {
					log.Printf("Request %s: route=%s class=%s status=%d", w.Header().Get("X-Request-ID"), key.Route, class, status)
				}
			}
		}()
		next.ServeHTTP(writer, r)
	})
}

// Metrics are deliberately aggregate: no query strings, source IDs, names, IPs,
// user-agent strings or arbitrary request paths can become labels.
func (a *API) Metrics() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" || r.Method != "GET" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		a.metrics.mu.Lock()
		defer a.metrics.mu.Unlock()
		fmt.Fprintln(w, "# TYPE ukinfo_api_active gauge")
		fmt.Fprintf(w, "ukinfo_api_active %d\n", a.metrics.active)
		fmt.Fprintln(w, "# TYPE ukinfo_api_request_duration_seconds histogram")
		for key, item := range a.metrics.requests {
			for i, bound := range latencyBounds {
				fmt.Fprintf(w, "ukinfo_api_request_duration_seconds_bucket{route=%q,status=%q,le=%q} %d\n", key.Route, fmt.Sprint(key.Status), fmt.Sprint(bound), item.Buckets[i])
			}
			fmt.Fprintf(w, "ukinfo_api_request_duration_seconds_bucket{route=%q,status=%q,le=\"+Inf\"} %d\n", key.Route, fmt.Sprint(key.Status), item.Count)
			fmt.Fprintf(w, "ukinfo_api_request_duration_seconds_count{route=%q,status=%q} %d\n", key.Route, fmt.Sprint(key.Status), item.Count)
			fmt.Fprintf(w, "ukinfo_api_request_duration_seconds_sum{route=%q,status=%q} %g\n", key.Route, fmt.Sprint(key.Status), item.Seconds)
		}
		fmt.Fprintln(w, "# TYPE ukinfo_query_requests_total counter")
		for key, count := range a.metrics.classes {
			fmt.Fprintf(w, "ukinfo_query_requests_total{class=%q,status=%q} %d\n", key.Class, fmt.Sprint(key.Status), count)
		}
		fmt.Fprintln(w, "# TYPE ukinfo_api_failures_total counter")
		for key, count := range a.metrics.failures {
			fmt.Fprintf(w, "ukinfo_api_failures_total{route=%q,reason=%q} %d\n", key.Route, key.Reason, count)
		}
		fmt.Fprintln(w, "# TYPE ukinfo_address_plans_total counter")
		for plan, count := range a.metrics.plans {
			fmt.Fprintf(w, "ukinfo_address_plans_total{plan=%q} %d\n", plan, count)
		}
		fmt.Fprintf(w, "# TYPE ukinfo_record_block_cache_lookups_total counter\nukinfo_record_block_cache_lookups_total{result=\"hit\"} %d\nukinfo_record_block_cache_lookups_total{result=\"miss\"} %d\n", a.metrics.cacheHits.Load(), a.metrics.cacheMisses.Load())
		fmt.Fprintf(w, "# TYPE ukinfo_record_blocks_decoded_total counter\nukinfo_record_blocks_decoded_total %d\n", a.metrics.decodedBlocks.Load())
		stats := a.DB.Stat()
		// Successful acquisitions that had to wait, plus separately canceled waits.
		fmt.Fprintln(w, "# TYPE ukinfo_pool_waits_total counter")
		fmt.Fprintf(w, "ukinfo_pool_waits_total %d\n", stats.EmptyAcquireCount())
		fmt.Fprintln(w, "# TYPE ukinfo_pool_wait_seconds_total counter")
		fmt.Fprintf(w, "ukinfo_pool_wait_seconds_total %g\n", stats.EmptyAcquireWaitTime().Seconds())
		fmt.Fprintln(w, "# TYPE ukinfo_pool_acquired gauge")
		fmt.Fprintf(w, "ukinfo_pool_acquired %d\n", stats.AcquiredConns())
		fmt.Fprintln(w, "# TYPE ukinfo_pool_max gauge")
		fmt.Fprintf(w, "ukinfo_pool_max %d\n", stats.MaxConns())
		fmt.Fprintln(w, "# TYPE ukinfo_pool_canceled_acquires_total counter")
		fmt.Fprintf(w, "ukinfo_pool_canceled_acquires_total %d\n", stats.CanceledAcquireCount())
	})
}
