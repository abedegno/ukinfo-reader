// Package telemetry exports only explicitly approved operational data.
package telemetry

import (
	"context"
	"errors"
	"log"
	"net"
	"net/url"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Configure is opt-in. Desktop and CLI operations never call it. No host,
// process, environment-resource or HTTP/SQL auto-instrumentation is installed.
func Configure(ctx context.Context) (func(context.Context) error, error) {
	noop := func(context.Context) error { return nil }
	if os.Getenv("UKINFO_OTEL_ENABLED") != "true" {
		return noop, nil
	}
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return noop, errors.New("telemetry requires an explicit OTLP traces endpoint without credentials or query parameters")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && ip != nil && ip.IsLoopback()) {
		return noop, errors.New("telemetry endpoint requires HTTPS or an explicit loopback HTTP address")
	}
	ratio := 0.1
	if value := os.Getenv("UKINFO_OTEL_SAMPLE_RATIO"); value != "" {
		ratio, err = strconv.ParseFloat(value, 64)
		if err != nil || !(ratio >= 0 && ratio <= 1) {
			return noop, errors.New("telemetry sample ratio must be between zero and one")
		}
	}
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint), otlptracehttp.WithTimeout(2*time.Second), otlptracehttp.WithRetry(otlptracehttp.RetryConfig{Enabled: false}))
	if err != nil {
		return noop, errors.New("telemetry exporter configuration failed")
	}
	var reported atomic.Bool
	// SDK/exporter errors may contain endpoint details; never log their raw text.
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(error) {
		if reported.CompareAndSwap(false, true) {
			log.Print("Telemetry export failed; inspect the private Collector")
		}
	}))
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", "ukinfo-server"))),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))),
		sdktrace.WithRawSpanLimits(sdktrace.SpanLimits{AttributeCountLimit: 8, AttributeValueLengthLimit: 128, EventCountLimit: 0, LinkCountLimit: 0}),
		sdktrace.WithBatcher(privateExporter{exporter}, sdktrace.WithMaxQueueSize(512), sdktrace.WithMaxExportBatchSize(128), sdktrace.WithBatchTimeout(5*time.Second), sdktrace.WithExportTimeout(2*time.Second)),
	)
	stopLogs, err := configureLogs(ctx)
	if err != nil {
		_ = provider.Shutdown(ctx)
		return noop, err
	}
	otel.SetTracerProvider(provider)
	return func(ctx context.Context) error {
		return errors.Join(provider.Shutdown(ctx), stopLogs(ctx))
	}, nil
}

// Operation accepts only fixed operation names. Caller-controlled values and
// raw error messages must never become span names, attributes or events.
func Operation(ctx context.Context, name string) (context.Context, trace.Span) {
	switch name {
	case "db.query", "db.pool.acquire", "records.load", "address.select",
		"search.vocabulary", "search.source_probe", "search.address_probe", "search.address_blocks",
		"search.address_dictionary", "search.candidates", "search.ordered", "search.group_members", "search.records", "search.geography", "search.parade_inventory", "search.parade_candidates", "search.parade_pair", "search.other":
	default:
		name = "operation"
	}
	if !trace.SpanFromContext(ctx).IsRecording() {
		return ctx, trace.SpanFromContext(context.Background())
	}
	return otel.Tracer("ukinfo/backend").Start(ctx, name)
}
func End(span trace.Span, err error) {
	if err != nil {
		span.SetStatus(codes.Error, "Operation failed")
	}
	span.End()
}

// DBTracer measures duration without inspecting SQL, arguments, result tags,
// connection strings or database error text.
type DBTracer struct{}
type queryKey struct{}
type acquireKey struct{}

func (DBTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	next, span := Operation(ctx, "db.query")
	return context.WithValue(next, queryKey{}, span)
}
func (DBTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryEndData) {
	if span, ok := ctx.Value(queryKey{}).(trace.Span); ok {
		End(span, d.Err)
	}
}
func (DBTracer) TraceAcquireStart(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireStartData) context.Context {
	next, span := Operation(ctx, "db.pool.acquire")
	return context.WithValue(next, acquireKey{}, span)
}
func (DBTracer) TraceAcquireEnd(ctx context.Context, _ *pgxpool.Pool, d pgxpool.TraceAcquireEndData) {
	if span, ok := ctx.Value(acquireKey{}).(trace.Span); ok {
		End(span, d.Err)
	}
}

// The upstream SDK merges OTEL_RESOURCE_ATTRIBUTES into WithResource. Enforce
// our resource allowlist at the final export boundary as well as in Collector.
type privateExporter struct{ sdktrace.SpanExporter }
type privateSpan struct{ sdktrace.ReadOnlySpan }

func (privateSpan) Resource() *resource.Resource {
	return resource.NewSchemaless(attribute.String("service.name", "ukinfo-server"))
}
func (e privateExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	safe := make([]sdktrace.ReadOnlySpan, len(spans))
	for i, span := range spans {
		safe[i] = privateSpan{span}
	}
	return e.SpanExporter.ExportSpans(ctx, safe)
}
