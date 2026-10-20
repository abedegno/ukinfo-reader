package telemetry

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	collector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestDisabledAndInvalidConfiguration(t *testing.T) {
	t.Setenv("UKINFO_OTEL_ENABLED", "false")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "INVALID_SECRET")
	stop, err := Configure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_ = stop(context.Background())
	t.Setenv("UKINFO_OTEL_ENABLED", "true")
	for _, endpoint := range []string{"", "http://collector:4318/v1/traces", "https://SECRET@example.test/v1/traces", "https://example.test/v1/traces?token=SECRET"} {
		t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", endpoint)
		if _, err := Configure(context.Background()); err == nil || strings.Contains(err.Error(), "SECRET") {
			t.Fatalf("unsafe configuration accepted or exposed: %v", err)
		}
	}
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://127.0.0.1:4318/v1/traces")
	for _, ratio := range []string{"NaN", "-1", "2", "SECRET"} {
		t.Setenv("UKINFO_OTEL_SAMPLE_RATIO", ratio)
		if _, err := Configure(context.Background()); err == nil {
			t.Fatal("invalid ratio accepted")
		}
	}
}

func TestWireExportExcludesSQLValuesAndErrors(t *testing.T) {
	previous := otel.GetTracerProvider()
	defer otel.SetTracerProvider(previous)
	wire := make(chan []byte, 2)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		wire <- b
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	defer receiver.Close()
	t.Setenv("UKINFO_OTEL_ENABLED", "true")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", receiver.URL+"/v1/traces")
	t.Setenv("UKINFO_OTEL_SAMPLE_RATIO", "1")
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "SECRET_RESOURCE=SECRET_VALUE")
	stop, err := Configure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, root := otel.Tracer("test").Start(context.Background(), "HTTP /api/search")
	root.AddEvent("SECRET_EVENT")
	root.RecordError(errors.New("SECRET_EXCEPTION"))
	tracer := DBTracer{}
	acquired := tracer.TraceAcquireStart(ctx, nil, pgxpool.TraceAcquireStartData{})
	tracer.TraceAcquireEnd(acquired, nil, pgxpool.TraceAcquireEndData{Err: errors.New("SECRET_POOL")})
	query := tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: "SELECT SECRET_SQL", Args: []any{"SECRET_NAME", "SECRET_POSTCODE"}})
	tracer.TraceQueryEnd(query, nil, pgx.TraceQueryEndData{Err: errors.New("SECRET_DATABASE_ERROR")})
	_, child := Operation(ctx, "SECRET_OPERATION")
	End(child, errors.New("SECRET_RECORD"))
	root.End()
	flush, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := stop(flush); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-wire:
		var request collector.ExportTraceServiceRequest
		if err := proto.Unmarshal(body, &request); err != nil {
			t.Fatal(err)
		}
		raw, _ := protojson.Marshal(&request)
		if strings.Contains(string(raw), "SECRET") {
			t.Fatal("sensitive data exported")
		}
		count := 0
		for _, resource := range request.ResourceSpans {
			for _, scope := range resource.ScopeSpans {
				for _, span := range scope.Spans {
					count++
					rootID := root.SpanContext().SpanID()
					if span.Name != "HTTP /api/search" && !bytes.Equal(span.ParentSpanId, rootID[:]) {
						t.Fatal("operation lost its request parent")
					}
					if len(span.Events) > 0 || len(span.Links) > 0 {
						t.Fatal("unexpected events/links")
					}
				}
			}
		}
		if count != 4 {
			t.Fatalf("got %d spans, expected request, pool, query and operation", count)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no OTLP export")
	}
}
