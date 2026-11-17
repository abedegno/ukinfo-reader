package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func inventedPlan() []byte {
	return []byte(`[{"Plan":{"Node Type":"Limit","Actual Rows":51,"Actual Loops":1,"Shared Hit Blocks":12,"Shared Read Blocks":3,"Temp Read Blocks":0,"Temp Written Blocks":0,"Plans":[{"Node Type":"Sort","Actual Rows":51,"Actual Loops":1,"Sort Method":"top-N heapsort","Sort Space Used":31,"Sort Space Type":"Memory","Plans":[{"Node Type":"Index Only Scan","Index Name":"p8_pc_d1_aaaaaaaaaaaaaaaaaaaaaaaa_cover","Actual Rows":400,"Actual Loops":1,"Heap Fetches":7,"Index Cond":"PRIVATE QUERY","Output":["SECRET"]}]}]},"Planning Time":0.3,"Execution Time":2.5}]`)
}
func TestExecutedPlanSummaryIsClosedAndDoesNotExportPredicates(t *testing.T) {
	p, err := summarizeExplain(inventedPlan(), "p8_pc_d1_aaaaaaaaaaaaaaaaaaaaaaaa_cover")
	if err != nil || !p.ExpectedIndexUsed || p.IndexOnlyScans != 1 || !p.HeapFetchesAvailable || p.HeapFetches != 7 || p.ScanRows != 400 || p.RootRows != 51 || p.SharedReadBlocks != 3 || p.MaxSortKB != 31 {
		t.Fatal("execution summary incomplete")
	}
	raw, _ := json.Marshal(p)
	for _, private := range []string{"PRIVATE", "SECRET", "Index Name", "d1_", "Cond", "Output"} {
		if strings.Contains(string(raw), private) {
			t.Fatal("private plan escaped")
		}
	}
	p, err = summarizeExplain([]byte(strings.ReplaceAll(string(inventedPlan()), "p8_pc_d1_aaaaaaaaaaaaaaaaaaaaaaaa_cover", "foreign_private_index")), "p8_pc_d1_aaaaaaaaaaaaaaaaaaaaaaaa_cover")
	if err != nil || p.ExpectedIndexUsed {
		t.Fatal("other index became expected index")
	}
}
func TestPlanSummaryRejectsMalformedMissingAndNonfiniteExecutionData(t *testing.T) {
	base := string(inventedPlan())
	for _, raw := range []string{"null", "[]", "{}", base + base, strings.Replace(base, `"Actual Rows":51`, `"Actual Rows":-1`, 1), strings.Replace(base, `"Actual Loops":1`, `"Actual Loops":"SECRET"`, 1), strings.Replace(base, `"Heap Fetches":7`, `"Heap Fetches":null`, 1), strings.Replace(base, `"Node Type":"Sort"`, `"Node Type":"SECRET"`, 1), strings.Replace(base, `"Execution Time":2.5`, `"Execution Time":1e999`, 1), strings.Replace(base, `"Shared Read Blocks":3,`, "", 1), strings.Repeat(" ", 1<<20) + base} {
		if _, err := summarizeExplain([]byte(raw), "p8_pc_d1_aaaaaaaaaaaaaaaaaaaaaaaa_cover"); err == nil {
			t.Fatal("malformed plan accepted")
		}
	}
}

type probeRow struct {
	raw             []byte
	generic, custom int64
	err             error
}

func (r probeRow) Scan(out ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(out) == 1 {
		*out[0].(*[]byte) = r.raw
	} else {
		*out[0].(*int64) = r.generic
		*out[1].(*int64) = r.custom
	}
	return nil
}

type probeConnection struct {
	t               *testing.T
	calls           []string
	raw             []byte
	generic, custom int64
	failure         string
	cleanup         bool
}

func (c *probeConnection) Prepare(_ context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	c.calls = append(c.calls, "prepare")
	if strings.Contains(sql, "@@@") || name != "ukinfo_postcode_cover_probe" {
		c.t.Fatal("wrong prepared SQL")
	}
	if c.failure == "prepare" {
		return nil, errors.New("PRIVATE")
	}
	return &pgconn.StatementDescription{}, nil
}
func (c *probeConnection) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	if strings.HasPrefix(sql, "EXPLAIN") {
		c.calls = append(c.calls, "explain")
		if !strings.Contains(sql, "EXECUTE ukinfo_postcode_cover_probe($1,$2,$3,$4,$5,$6,$7)") || len(args) != 8 || args[0] != pgx.QueryExecModeSimpleProtocol {
			c.t.Fatal("unsafe literal encoding or wrong plan protocol")
		}
		if c.failure == "execute" {
			return probeRow{err: context.DeadlineExceeded}
		}
		return probeRow{raw: c.raw}
	}
	c.calls = append(c.calls, "generic_counters")
	if !strings.Contains(sql, "pg_prepared_statements") {
		c.t.Fatal("missing prepared generic-plan proof")
	}
	return probeRow{generic: c.generic, custom: c.custom}
}
func (c *probeConnection) Deallocate(context.Context, string) error {
	c.cleanup = true
	if c.failure == "cleanup" {
		return errors.New("PRIVATE")
	}
	return nil
}
func TestProbeExecutesActualGenericPreparedSQLAndAlwaysCleansUp(t *testing.T) {
	for _, failure := range []string{"", "prepare", "execute", "custom", "cleanup"} {
		c := &probeConnection{t: t, raw: inventedPlan(), generic: 1, failure: failure}
		if failure == "custom" {
			c.custom = 1
		}
		_, err := probePreparedPlan(context.Background(), c, twoStatements()[0], "p8_pc_d1_aaaaaaaaaaaaaaaaaaaaaaaa_cover")
		if (err != nil) != (failure != "") {
			t.Fatal("wrong probe result")
		}
		if failure != "prepare" && !c.cleanup {
			t.Fatal("prepared statement not deallocated")
		}
		if failure == "" && strings.Join(c.calls, ",") != "prepare,explain,generic_counters" {
			t.Fatal("query was not actually prepared and explained")
		}
	}
}

func TestUnexecutedExpectedIndexCannotClaimObservedIndexUse(t *testing.T) {
	raw := strings.Replace(string(inventedPlan()), `"Actual Rows":400,"Actual Loops":1,"Heap Fetches":7`, `"Actual Rows":0,"Actual Loops":0,"Heap Fetches":0`, 1)
	p, err := summarizeExplain([]byte(raw), "p8_pc_d1_aaaaaaaaaaaaaaaaaaaaaaaa_cover")
	if err != nil || p.ExpectedIndexUsed || p.HeapFetchesAvailable || p.IndexOnlyScans != 0 {
		t.Fatal("unexecuted index reported as observed execution")
	}
}

func TestParallelWorkerSortSpillAndMemoryAreIncluded(t *testing.T) {
	raw := strings.Replace(string(inventedPlan()), `"Sort Space Type":"Memory",`, `"Sort Space Type":"Memory","Workers":[{"Worker Number":0,"Sort Method":"external merge","Sort Space Used":128,"Sort Space Type":"Disk"}],`, 1)
	p, err := summarizeExplain([]byte(raw), "p8_pc_d1_aaaaaaaaaaaaaaaaaaaaaaaa_cover")
	if err != nil || p.ExternalSorts != 1 || p.MaxSortKB != 128 {
		t.Fatal("parallel worker sort work disappeared")
	}
}

func TestPostgres18DecimalRowsAndPerLoopAverages(t *testing.T) {
	raw := strings.Replace(string(inventedPlan()), `"Actual Rows":51`, `"Actual Rows":51.00`, 1)
	raw = strings.Replace(raw, `"Actual Rows":400,"Actual Loops":1`, `"Actual Rows":400.33,"Actual Loops":3`, 1)
	p, err := summarizeExplain([]byte(raw), "p8_pc_d1_aaaaaaaaaaaaaaaaaaaaaaaa_cover")
	if err != nil || p.RootRows != 51 || float64(p.ScanRows) < 1200.98 || float64(p.ScanRows) > 1201.00 {
		t.Fatal("PostgreSQL18 decimal or per-loop row format rejected")
	}
	for _, bad := range []string{`"Actual Rows":-0.01`, `"Actual Rows":1e999`, `"Actual Rows":null`, `"Actual Rows":"51.00"`, `"Actual Rows":51.01`} {
		if _, err := summarizeExplain([]byte(strings.Replace(raw, `"Actual Rows":51.00`, bad, 1)), "p8_pc_d1_aaaaaaaaaaaaaaaaaaaaaaaa_cover"); err == nil {
			t.Fatal("invalid root row count accepted")
		}
	}
}

func TestProbeFailureStageIsClosedWithoutPrivateErrorText(t *testing.T) {
	for _, stage := range []string{"prepare", "execute", "summary", "generic_counters"} {
		c := &probeConnection{t: t, raw: inventedPlan(), generic: 1, failure: stage}
		if stage == "summary" {
			c.raw = []byte(`{"PRIVATE":"RAW"}`)
		}
		if stage == "generic_counters" {
			c.generic = 0
		}
		_, err := probePreparedPlan(context.Background(), c, twoStatements()[0], "p8_pc_d1_aaaaaaaaaaaaaaaaaaaaaaaa_cover")
		var boundary *planProbeError
		if !errors.As(err, &boundary) || boundary.stage != stage || strings.Contains(err.Error(), "PRIVATE") {
			t.Fatal("probe failure boundary missing or private")
		}
	}
}
