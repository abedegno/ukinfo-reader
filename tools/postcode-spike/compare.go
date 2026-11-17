package main

import (
	"context"
	"errors"
	"math"

	"github.com/jackc/pgx/v5/pgconn"
)

type reading struct {
	Rows         int
	Digest       [32]byte
	Milliseconds float64
	Err          error
}
type observation struct {
	Strategy     string  `json:"strategy"`
	Outcome      string  `json:"outcome"`
	Milliseconds float64 `json:"milliseconds"`
	Rows         int     `json:"rows"`
}
type aggregate struct {
	ExecutionOnly     bool            `json:"execution_only,omitempty"`
	Plan              *planSummary    `json:"plan,omitempty"`
	Mode              string          `json:"mode,omitempty"`
	IndexValidated    bool            `json:"index_validated,omitempty"`
	CaptureFailure    *captureFailure `json:"capture_failure,omitempty"`
	SetupMilliseconds float64         `json:"connection_setup_ms"`
	Stage             string          `json:"stage"`
	Captures          int             `json:"cancelled_capture_requests"`
	SchemaVersion     int             `json:"schema_version"`
	DiagnosticOnly    bool            `json:"diagnostic_only"`
	Complete          bool            `json:"complete"`
	Requests          int             `json:"requests"`
	Errors            int             `json:"errors"`
	SQLRowsEqual      bool            `json:"sql_rows_equal"`
	PlanMode          string          `json:"plan_mode"`
	SQLSeconds        int             `json:"sql_seconds"`
	CacheConditions   string          `json:"cache_conditions"`
	Observations      []observation   `json:"observations"`
}

func classifyError(err error) string {
	if err == nil {
		return "ok"
	}
	var pg *pgconn.PgError
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &pg) && pg.Code == "57014" {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	return "database_error"
}

// Each case gets both strategies in each early/late half. This controls the
// fixed ordering bias of the old plans utility; caches remain uncontrolled.
func compare(ctx context.Context, statements []statement, run func(context.Context, statement) reading) (aggregate, error) {
	return compareStrategies(ctx, statements, run, func(s statement) (string, string, error) {
		a, err := prefilterVariant(s.SQL, s.Args)
		if err != nil {
			return "", "", err
		}
		b, err := scalarVariant(s.SQL, s.Args)
		return a, b, err
	}, [2]string{"postcode_prefilter", "scalar"}, false)
}

func compareStrategies(ctx context.Context, statements []statement, run func(context.Context, statement) reading, variants func(statement) (string, string, error), labels [2]string, sharedBaseline bool) (aggregate, error) {
	result := newAggregate()
	if len(statements) != 2 {
		return result, errors.New("two captured statements required")
	}
	equal, valid := true, true
	schedules := [][]bool{{false, true, true, false}, {true, false, false, true}}
	var baseline reading
	haveBaseline := false
	for i, original := range statements {
		first, second, err := variants(original)
		if err != nil {
			return result, errors.New("invalid captured statement")
		}
		if !sharedBaseline {
			haveBaseline = false
		}
		for _, useScalar := range schedules[i] {
			if ctx.Err() != nil {
				return result, errors.New("comparison budget exhausted")
			}
			query := original
			query.SQL = first
			label := labels[0]
			if useScalar {
				query.SQL = second
				label = labels[1]
			}
			sample := run(ctx, query)
			if sample.Err == nil && ctx.Err() != nil {
				sample.Err = ctx.Err()
			}
			outcome := classifyError(sample.Err)
			if sample.Rows < 0 || sample.Rows > 129 || math.IsNaN(sample.Milliseconds) || math.IsInf(sample.Milliseconds, 0) || sample.Milliseconds < 0 {
				outcome = "invalid_measurement"
				sample.Rows = 0
				sample.Milliseconds = 0
				valid = false
			}
			result.Requests++
			result.Observations = append(result.Observations, observation{label, outcome, sample.Milliseconds, sample.Rows})
			if outcome != "ok" {
				result.Errors++
				equal = false
				continue
			}
			if !haveBaseline {
				baseline = sample
				haveBaseline = true
			} else if sample.Rows != baseline.Rows || sample.Digest != baseline.Digest {
				equal = false
			}
		}
	}
	result.Complete = result.Requests == 8 && ctx.Err() == nil && valid
	result.SQLRowsEqual = result.Complete && result.Errors == 0 && equal
	if !result.Complete {
		return result, errors.New("comparison incomplete")
	}
	return result, nil
}

func newAggregate() aggregate {
	return aggregate{SchemaVersion: 1, DiagnosticOnly: true, PlanMode: "force_generic_plan", SQLSeconds: 4, CacheConditions: "shared, uncontrolled; no eviction", Observations: []observation{}}
}
