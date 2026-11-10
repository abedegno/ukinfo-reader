package telemetry

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgconn"
	"testing"
)

func TestEditionLabelsAndOutcomes(t *testing.T) {
	for _, key := range []string{"se-pcf78-1998-01", "se4-pcf94-1999-04", "se2003-dpcs0104", "se2004-pcp218-2004-07", "se2005-pcf175-2005-06", "v14-2008-03:co1-excluded"} {
		if EditionKey(key) != key {
			t.Fatal("known edition lost")
		}
	}
	if EditionKey("PRIVATE_NAME/postcode") != "other" || EditionStage("PRIVATE_STAGE") != "other" {
		t.Fatal("unbounded labels")
	}
	tests := []struct {
		err  error
		want string
	}{{nil, "ok"}, {context.DeadlineExceeded, "deadline"}, {fmt.Errorf("PRIVATE: %w", context.Canceled), "canceled"}, {&pgconn.PgError{Code: "57014", Message: "PRIVATE"}, "query_canceled"}, {&pgconn.PgError{Code: "42P01", Message: "PRIVATE"}, "error"}, {errors.New("PRIVATE"), "error"}}
	for _, tt := range tests {
		if QueryOutcome(tt.err) != tt.want {
			t.Fatal("wrong bounded error outcome")
		}
	}
}
