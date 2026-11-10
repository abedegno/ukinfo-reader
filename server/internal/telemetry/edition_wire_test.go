package telemetry

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
	collector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEditionWirePrivacyAndParenting(t *testing.T) {
	previous := otel.GetTracerProvider()
	defer otel.SetTracerProvider(previous)
	wire := make(chan []byte, 4)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		wire <- b
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	defer receiver.Close()
	t.Setenv("UKINFO_OTEL_ENABLED", "true")
	t.Setenv("UKINFO_OTEL_SAMPLE_RATIO", "1")
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", receiver.URL+"/v1/traces")
	stop, err := Configure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, root := otel.Tracer("test").Start(context.Background(), "HTTP /api/search")
	editionCtx, end := ParadeEdition(ctx, "se2003-dpcs0104", "positions")
	tracer := DBTracer{}
	query := tracer.TraceQueryStart(editionCtx, nil, pgx.TraceQueryStartData{SQL: "SELECT SECRET_SQL", Args: []any{"SECRET_NAME", "SECRET_POSTCODE"}})
	dbErr := &pgconn.PgError{Code: "57014", Message: "SECRET_DB_ERROR"}
	tracer.TraceQueryEnd(query, nil, pgx.TraceQueryEndData{Err: dbErr})
	end(dbErr)
	_, unknownEnd := ParadeEdition(ctx, "SECRET_IMPORT", "SECRET_STAGE")
	unknownEnd(errors.New("SECRET_ERROR"))
	root.End()
	flush, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err = stop(flush); err != nil {
		t.Fatal(err)
	}
	select {
	case raw := <-wire:
		var r collector.ExportTraceServiceRequest
		if proto.Unmarshal(raw, &r) != nil {
			t.Fatal("invalid OTLP")
		}
		text, _ := protojson.Marshal(&r)
		if strings.Contains(string(text), "SECRET") {
			t.Fatal("private value exported")
		}
		found, other := false, false
		var childParent, editionSpan []byte
		for _, resource := range r.ResourceSpans {
			for _, scope := range resource.ScopeSpans {
				for _, span := range scope.Spans {
					attrs := map[string]string{}
					for _, v := range span.Attributes {
						attrs[v.Key] = v.Value.GetStringValue()
					}
					if span.Name == "search.parade_edition" {
						if attrs["ukinfo.edition"] == "se2003-dpcs0104" {
							found = attrs["ukinfo.backend"] == "paradedb" && attrs["ukinfo.parade.stage"] == "positions" && attrs["ukinfo.outcome"] == "query_canceled"
							editionSpan = span.SpanId
						} else {
							other = attrs["ukinfo.edition"] == "other" && attrs["ukinfo.parade.stage"] == "other" && attrs["ukinfo.outcome"] == "error"
						}
					}
					if span.Name == "db.query" {
						childParent = span.ParentSpanId
					}
				}
			}
		}
		if !found || !other || string(childParent) != string(editionSpan) {
			t.Fatal("edition context or DB parenting lost")
		}
	case <-flush.Done():
		t.Fatal("no OTLP export")
	}
}
