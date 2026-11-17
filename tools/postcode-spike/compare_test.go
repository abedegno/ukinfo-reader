package main

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func twoStatements() []statement {
	args := []any{int16(0), "ZZ9", "ZZ9\U0010ffff", 6, "ZZ9", "ZZ9(.{3})?", 51}
	return []statement{{testSQL, args}, {testSQL, append([]any(nil), args...)}}
}

func TestCounterbalancedScheduleChecksAllOrderedRowsWithoutPrivateOutput(t *testing.T) {
	seen := []string{}
	report, err := compare(context.Background(), twoStatements(), func(_ context.Context, s statement) reading {
		scalar := strings.Contains(s.SQL, "::text IS NOT NULL")
		if scalar {
			seen = append(seen, "scalar")
		} else {
			seen = append(seen, "postcode_prefilter")
		}
		return reading{Rows: 51, Digest: [32]byte{9}, Milliseconds: 12}
	})
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"postcode_prefilter", "scalar", "scalar", "postcode_prefilter", "scalar", "postcode_prefilter", "postcode_prefilter", "scalar"}
	if !reflect.DeepEqual(seen, expected) {
		t.Fatal("comparison order is not counterbalanced")
	}
	if !report.Complete || !report.SQLRowsEqual || report.Requests != 8 || report.Errors != 0 {
		t.Fatal("complete equivalent comparison rejected")
	}
	raw, _ := json.Marshal(report)
	for _, private := range []string{"PRIVATE", "ZZ9", "Digest", "digest", "Args", testSQL} {
		if strings.Contains(string(raw), private) {
			t.Fatal("private information exported")
		}
	}
	if len(report.Observations) != 8 {
		t.Fatal("observation omitted")
	}
}

func TestChangedRowsAndErrorsNeverClaimEquivalence(t *testing.T) {
	for _, mode := range []string{"rows", "digest", "error", "nan"} {
		t.Run(mode, func(t *testing.T) {
			n := 0
			report, _ := compare(context.Background(), twoStatements(), func(context.Context, statement) reading {
				n++
				r := reading{Rows: 51, Digest: [32]byte{1}, Milliseconds: 1}
				if n == 2 {
					switch mode {
					case "rows":
						r.Rows = 50
					case "digest":
						r.Digest = [32]byte{2}
					case "error":
						r.Err = errors.New("SECRET_SQL_AND_ARGS")
					case "nan":
						r.Milliseconds = math.NaN()
					}
				}
				return r
			})
			if report.SQLRowsEqual {
				t.Fatal("changed or failed result accepted")
			}
			raw, err := json.Marshal(report)
			if err != nil || strings.Contains(string(raw), "SECRET") {
				t.Fatal("invalid or private report")
			}
		})
	}
}

func TestCancellationCannotBecomeCompleteComparison(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	n := 0
	report, _ := compare(ctx, twoStatements(), func(context.Context, statement) reading {
		n++
		if n == 2 {
			cancel()
		}
		return reading{Rows: 1, Milliseconds: 1}
	})
	if report.Complete || report.SQLRowsEqual || n > 2 {
		t.Fatal("cancelled workload accepted or continued")
	}
}

func TestOnlyClosedErrorsAreExported(t *testing.T) {
	for _, tc := range []struct {
		err   error
		label string
	}{{context.DeadlineExceeded, "timeout"}, {context.Canceled, "canceled"}, {&pgconn.PgError{Code: "57014", Message: "SECRET"}, "timeout"}, {errors.New("PRIVATE_DSN"), "database_error"}} {
		if classifyError(tc.err) != tc.label {
			t.Fatal("wrong sanitized outcome")
		}
	}
}
