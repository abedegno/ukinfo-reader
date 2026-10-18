package catalog

import (
	"fmt"
	"net/http"
	"sync"
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
type apiMetrics struct {
	mu       sync.Mutex
	requests map[metricKey]requestMetric
	active   int
}

var latencyBounds = [...]float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5}

func newAPIMetrics() *apiMetrics { return &apiMetrics{requests: make(map[metricKey]requestMetric)} }
func metricRoute(path string) string {
	switch path {
	case "/api/search", "/api/export", "/api/record", "/api/address", "/api/map", "/api/datasets", "/api/addresses", "/api/geography", "/api/health":
		return path
	}
	return "other"
}

type metricWriter struct {
	http.ResponseWriter
	status int
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
		stats := a.DB.Stat()
		fmt.Fprintln(w, "# TYPE ukinfo_pool_acquired gauge")
		fmt.Fprintf(w, "ukinfo_pool_acquired %d\n", stats.AcquiredConns())
		fmt.Fprintln(w, "# TYPE ukinfo_pool_max gauge")
		fmt.Fprintf(w, "ukinfo_pool_max %d\n", stats.MaxConns())
		fmt.Fprintln(w, "# TYPE ukinfo_pool_canceled_acquires_total counter")
		fmt.Fprintf(w, "ukinfo_pool_canceled_acquires_total %d\n", stats.CanceledAcquireCount())
	})
}
