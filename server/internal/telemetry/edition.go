package telemetry

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
)

// EditionKey deliberately excludes arbitrary imported keys, samples and paths.
func EditionKey(key string) string {
	switch key {
	case "se-pcf78-1998-01", "se4-pcf94-1999-04", "se2003-dpcs0104", "se2004-pcp218-2004-07", "se2005-pcf175-2005-06", "v14-2008-03:co1-excluded":
		return key
	default:
		return "other"
	}
}
func EditionStage(stage string) string {
	switch stage {
	case "positions", "pair_drain":
		return stage
	default:
		return "other"
	}
}

// SQLSTATE 57014 means server cancellation; it does not establish its cause.
func QueryOutcome(err error) string {
	if err == nil {
		return "ok"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg != nil && pg.Code == "57014" {
		return "query_canceled"
	}
	return "error"
}
func ParadeEdition(ctx context.Context, edition, stage string) (context.Context, func(error)) {
	ctx, span := Operation(ctx, "search.parade_edition")
	span.SetAttributes(attribute.String("ukinfo.backend", "paradedb"), attribute.String("ukinfo.edition", EditionKey(edition)), attribute.String("ukinfo.parade.stage", EditionStage(stage)))
	return ctx, func(err error) {
		span.SetAttributes(attribute.String("ukinfo.outcome", QueryOutcome(err)))
		End(span, err)
	}
}
