package telemetry

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"regexp"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
)

// Logs are explicit operational events. The standard stderr logger is not bridged:
// raw database errors, command output and query values must not be exported.
func configureLogs(ctx context.Context) (func(context.Context) error, error) {
	noop := func(context.Context) error { return nil }
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT")
	if endpoint == "" {
		return noop, nil
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return noop, errors.New("telemetry requires an explicit OTLP logs endpoint without credentials or query parameters")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && ip != nil && ip.IsLoopback()) {
		return noop, errors.New("telemetry logs endpoint requires HTTPS or an explicit loopback HTTP address")
	}
	// The log SDK merges Environment into WithResource. Fail closed rather than
	// exporting arbitrary environment attributes before the Collector privacy gate.
	if os.Getenv("OTEL_RESOURCE_ATTRIBUTES") != "" || os.Getenv("OTEL_SERVICE_NAME") != "" {
		return noop, errors.New("log telemetry does not permit ambient resource attributes")
	}
	exporter, err := otlploghttp.New(ctx, otlploghttp.WithEndpointURL(endpoint),
		otlploghttp.WithTimeout(2*time.Second), otlploghttp.WithRetry(otlploghttp.RetryConfig{Enabled: false}))
	if err != nil {
		return noop, errors.New("telemetry logs exporter configuration failed")
	}
	provider := sdklog.NewLoggerProvider(
		sdklog.WithResource(resource.NewSchemaless(attribute.String("service.name", "ukinfo-server"))),
		sdklog.WithAttributeCountLimit(10), sdklog.WithAttributeValueLengthLimit(64),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter, sdklog.WithMaxQueueSize(512),
			sdklog.WithExportMaxBatchSize(128), sdklog.WithExportInterval(5*time.Second), sdklog.WithExportTimeout(2*time.Second))))
	global.SetLoggerProvider(provider)
	return provider.Shutdown, nil
}

var opaqueRequestID = regexp.MustCompile(`^[0-9a-f]{24}$`)

// LogRequest accepts only fixed enums and numeric values, never a request, URL,
// error or SQL string. Logs do not depend on the trace sampling decision.
func LogRequest(ctx context.Context, route, method, class, requestID, failure string, status int, elapsed time.Duration) {
	switch route {
	case "/api/research", "/api/group", "/api/search", "/api/export", "/api/record", "/api/address", "/api/map", "/api/datasets", "/api/addresses", "/api/geography", "/api/health":
	default:
		route = "other"
	}
	if route != "/api/search" && route != "/api/export" && status < 400 {
		return
	}
	switch method {
	case "GET", "POST", "HEAD", "OPTIONS":
	default:
		method = "OTHER"
	}
	switch class {
	case "name", "postcode", "address", "nearby", "ordered", "grouped":
	default:
		class = "other"
	}
	switch failure {
	case "query_timeout", "catalogue_integrity", "unavailable":
	default:
		failure = "none"
	}
	if status < 100 || status > 599 {
		status = 500
	}
	severity, severityText := otellog.SeverityInfo, "INFO"
	if status >= 500 {
		severity, severityText = otellog.SeverityError, "ERROR"
	} else if status >= 400 {
		severity, severityText = otellog.SeverityWarn, "WARN"
	}
	logger := global.Logger("ukinfo/http")
	if !logger.Enabled(ctx, otellog.EnabledParameters{Severity: severity}) {
		return
	}
	var record otellog.Record
	record.SetTimestamp(time.Now())
	record.SetSeverity(severity)
	record.SetSeverityText(severityText)
	record.SetBody(attribute.StringValue("HTTP request completed"))
	duration := elapsed.Milliseconds()
	if duration < 0 {
		duration = 0
	}
	record.AddAttributes(attribute.String("event.name", "ukinfo.http.request"),
		attribute.String("http.route", route), attribute.String("http.request.method", method),
		attribute.Int("http.response.status_code", status), attribute.Int64("duration_ms", duration),
		attribute.String("ukinfo.query.class", class), attribute.String("ukinfo.failure.reason", failure))
	if opaqueRequestID.MatchString(requestID) {
		record.AddAttributes(attribute.String("request_id", requestID))
	}
	logger.Emit(ctx, record)
}
