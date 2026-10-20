package telemetry

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/log/global"
	logs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestLogsConfigurationRejectsAmbientAndUnsafeEndpoints(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "https://SECRET@example.test/v1/logs")
	if _, err := configureLogs(context.Background()); err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatal("unsafe log endpoint accepted or disclosed")
	}
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "http://127.0.0.1:4318/v1/logs")
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "SECRET=SECRET")
	if _, err := configureLogs(context.Background()); err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatal("ambient log resource attributes accepted or disclosed")
	}
}

func TestLogWirePrivacyAndUnsampledTraceCorrelation(t *testing.T) {
	previousLogs, previousTraces := global.GetLoggerProvider(), otel.GetTracerProvider()
	defer global.SetLoggerProvider(previousLogs)
	defer otel.SetTracerProvider(previousTraces)
	wire := make(chan []byte, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		if r.URL.Path == "/v1/logs" {
			wire <- data
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	defer server.Close()
	t.Setenv("UKINFO_OTEL_ENABLED", "true")
	t.Setenv("UKINFO_OTEL_SAMPLE_RATIO", "0")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", server.URL+"/v1/traces")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", server.URL+"/v1/logs")
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "")
	t.Setenv("OTEL_SERVICE_NAME", "")
	stop, err := Configure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, span := otel.Tracer("test").Start(context.Background(), "HTTP /api/search")
	if span.SpanContext().IsSampled() {
		t.Fatal("test must exercise unsampled request logs")
	}
	LogRequest(ctx, "/api/search", "GET", "name", "0123456789abcdef01234567", "query_timeout", 503, 120*time.Millisecond)
	LogRequest(ctx, "SECRET_PATH", "SECRET_METHOD", "SECRET_CLASS", "SECRET_REQUEST_ID", "SECRET_ERROR", 404, -time.Second)
	LogRequest(ctx, "/api/health", "GET", "name", "", "", 200, time.Millisecond)
	span.End()
	flush, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := stop(flush); err != nil {
		t.Fatal(err)
	}
	select {
	case data := <-wire:
		var request logs.ExportLogsServiceRequest
		if err := proto.Unmarshal(data, &request); err != nil {
			t.Fatal(err)
		}
		raw, _ := protojson.Marshal(&request)
		if strings.Contains(string(raw), "SECRET") {
			t.Fatal("log privacy canary escaped")
		}
		count := 0
		for _, resource := range request.ResourceLogs {
			if len(resource.Resource.Attributes) != 1 || resource.Resource.Attributes[0].Key != "service.name" {
				t.Fatal("unexpected log resource attributes")
			}
			for _, scope := range resource.ScopeLogs {
				for _, record := range scope.LogRecords {
					count++
					if len(record.TraceId) != 16 || len(record.SpanId) != 8 {
						t.Fatal("request log lost trace correlation")
					}
					if record.Body.GetStringValue() != "HTTP request completed" {
						t.Fatal("unexpected log body")
					}
				}
			}
		}
		if count != 2 {
			t.Fatalf("got %d logs; health should not produce request noise", count)
		}
	case <-flush.Done():
		t.Fatal("log exporter did not deliver records")
	}
}
