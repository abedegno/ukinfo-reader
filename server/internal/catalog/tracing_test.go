package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestRequestTracesExcludePersonalData(t *testing.T) {
	previous := otel.GetTracerProvider()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()), sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(provider)
	defer func() { _ = provider.Shutdown(context.Background()); otel.SetTracerProvider(previous) }()
	m := newAPIMetrics()
	wrapped := m.wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	for _, path := range []string{"/api/search?surname=SECRET_NAME&town=SECRET_TOWN", "/SECRET_PATH?postcode=SECRET_POSTCODE"} {
		request := httptest.NewRequest("GET", path, nil)
		request.Header.Set("User-Agent", "SECRET_AGENT")
		request.Header.Set("baggage", "secret=SECRET_BAGGAGE")
		request.Header.Set("traceparent", "00-11111111111111111111111111111111-2222222222222222-01")
		wrapped.ServeHTTP(httptest.NewRecorder(), request)
	}
	spans := recorder.Ended()
	if len(spans) != 2 {
		t.Fatal(len(spans))
	}
	for _, span := range spans {
		raw, _ := json.Marshal(span.Attributes())
		if strings.Contains(string(raw)+span.Name()+span.Status().Description, "SECRET") || span.Parent().IsValid() || len(span.Events()) > 0 || len(span.Links()) > 0 {
			t.Fatal("unsafe request trace")
		}
	}
	if spans[0].Name() != "HTTP /api/search" || spans[1].Name() != "HTTP other" {
		t.Fatal("unbounded route")
	}
}
