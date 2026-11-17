package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/abedegno/ukinfo-reader/server/internal/catalog"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func poolConfig(uri string, target bool) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(uri)
	if err != nil || uri == "" {
		return nil, errors.New("explicit private reader required")
	}
	user, host, db, timeout := "ukinfo_v14_verify", "ukinfo-db", "ukinfo", strconv.FormatInt(catalog.SourceReadTimeout.Milliseconds(), 10)
	cfg.MaxConns = 8
	cfg.MinConns = 1
	if target {
		user, host, db, timeout = "ukinfo_parade_search", "ukinfo-parade-preview-db", "ukinfo_search", "4000"
		cfg.MaxConns = 4
		cfg.MinConns = 0
	}
	if cfg.ConnConfig.User != user || cfg.ConnConfig.Host != host || cfg.ConnConfig.Database != db || cfg.ConnConfig.Port != 5432 || cfg.ConnConfig.Password == "" {
		return nil, errors.New("unexpected private reader identity")
	}
	for _, fallback := range cfg.ConnConfig.Fallbacks {
		if fallback == nil || fallback.Host != host || fallback.Port != 5432 {
			return nil, errors.New("unexpected fallback reader identity")
		}
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheStatement
	cfg.ConnConfig.StatementCacheCapacity = 512
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = timeout
	if target {
		cfg.ConnConfig.RuntimeParams["plan_cache_mode"] = "force_generic_plan"
	}
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		command := "SET default_transaction_read_only=on; SET statement_timeout='" + timeout + "'"
		if target {
			command += "; SET plan_cache_mode=force_generic_plan"
		}
		_, err := conn.Exec(ctx, command)
		if err != nil {
			return errors.New("reader session settings unavailable")
		}
		return nil
	}
	return cfg, nil
}

func captureStatements(ctx context.Context, cases []workloadCase) ([]statement, error) {
	source, err := poolConfig(os.Getenv("DATABASE_URL"), false)
	if err != nil {
		return nil, err
	}
	src, err := pgxpool.NewWithConfig(ctx, source)
	if err != nil {
		return nil, errors.New("source reader unavailable")
	}
	defer src.Close()
	target, err := poolConfig(os.Getenv("TARGET_DATABASE_URL"), true)
	if err != nil {
		return nil, err
	}
	tracer := &capture{}
	target.ConnConfig.Tracer = tracer
	dst, err := pgxpool.NewWithConfig(ctx, target)
	if err != nil {
		return nil, errors.New("capture target unavailable")
	}
	defer dst.Close()
	api := catalog.NewAPI(src)
	api.ParadeDB = dst
	handler := api.Handler()
	statements := []statement{}
	for _, c := range cases {
		tracer.reset()
		req, err := privateRequest(ctx, c)
		if err != nil {
			return statements, err
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		item, err := tracer.only()
		if err != nil || response.Code != 503 {
			return statements, tracer.failure(response.Code)
		}
		statements = append(statements, item)
	}
	// All capture connections close before the separate measurement pool opens:
	// at no time do two target pools exceed the serving four-connection bound.
	return statements, nil
}

func measure(ctx context.Context, statements []statement) (aggregate, error) {
	return measureMode(ctx, statements, false, false)
}
func measureCovering(ctx context.Context, statements []statement) (aggregate, error) {
	return measureMode(ctx, statements, true, false)
}

func measureCoveringPlan(ctx context.Context, statements []statement) (aggregate, error) {
	return measureMode(ctx, statements, true, true)
}

func measureMode(ctx context.Context, statements []statement, covering bool, planMode bool) (aggregate, error) {
	cfg, err := poolConfig(os.Getenv("TARGET_DATABASE_URL"), true)
	if err != nil {
		return newAggregate(), err
	}
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return newAggregate(), errors.New("measurement target unavailable")
	}
	defer db.Close()
	setupStart := time.Now()
	conn, err := db.Acquire(ctx)
	setupMS := float64(time.Since(setupStart)) / float64(time.Millisecond)
	if err != nil {
		report := newAggregate()
		report.SetupMilliseconds = setupMS
		return report, errors.New("measurement connection unavailable")
	}
	defer conn.Release()
	compareFn := compare
	var before coverIndex
	var indexTable string
	readIndex := func() (coverIndex, error) {
		value := coverIndex{}
		dataset, err := strconv.Atoi(strings.Split(indexTable[1:], "_")[0])
		if err != nil {
			return value, errors.New("canonical table required")
		}
		var raw []byte
		err = conn.QueryRow(ctx, coverIndexSQL, "p8_pc_"+indexTable+"_cover", indexTable, dataset).Scan(&raw)
		if err != nil || json.Unmarshal(raw, &value) != nil || validateCoverIndex(value, indexTable) != nil {
			return value, errors.New("reviewed covering index unavailable")
		}
		return value, nil
	}
	if covering {
		district, err := districtStatement(statements)
		if err != nil {
			return newAggregate(), err
		}
		indexTable = candidateSQL.FindStringSubmatch(district.SQL)[1]
		before, err = readIndex()
		if err != nil {
			return newAggregate(), err
		}
		compareFn = compareCovering
	}
	report := newAggregate()
	var comparisonErr error
	if planMode {
		district, _ := districtStatement(statements) // validated before metadata admission
		report.ExecutionOnly = true
		report.Requests = 1
		started := time.Now()
		summary, err := probePreparedPlan(ctx, conn.Conn(), district, "p8_pc_"+indexTable+"_cover")
		if err == nil && ctx.Err() != nil {
			err = ctx.Err()
		}
		comparisonErr = err
		var boundary *planProbeError
		if errors.As(err, &boundary) {
			report.PlanFailureStage = boundary.stage
		}
		report.Observations = []observation{{"native_scalar", classifyError(err), float64(time.Since(started)) / float64(time.Millisecond), int(summary.RootRows)}}
		if err == nil {
			report.Plan = &summary
			report.Complete = true
		} else {
			report.Errors = 1
		}
	} else {
		report, comparisonErr = compareFn(ctx, statements, func(ctx context.Context, s statement) reading {
			start := time.Now()
			rows, err := conn.Query(ctx, s.SQL, s.Args...)
			out := reading{}
			if err == nil {
				out.Rows, out.Digest, err = digestRows(rows)
			}
			out.Milliseconds = float64(time.Since(start)) / float64(time.Millisecond)
			out.Err = err
			return out
		})
	}
	if covering {
		after, err := readIndex()
		if err != nil || !reflect.DeepEqual(before, after) {
			report.Complete = false
			report.SQLRowsEqual = false
			comparisonErr = errors.New("covering index changed during comparison")
		} else {
			report.IndexValidated = true
		}
	}
	report.SetupMilliseconds = setupMS
	return report, comparisonErr
}

func privateRequest(ctx context.Context, c workloadCase) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://localhost"+c.Path+"?"+c.Query, nil)
	if err != nil {
		return nil, errors.New("private request unavailable")
	}
	return req, nil
}
