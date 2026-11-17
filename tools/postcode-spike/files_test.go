package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateReportIsExclusiveAndContainsOnlyAggregate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")
	value := aggregate{DiagnosticOnly: true, Complete: false, Requests: 1, Errors: 1, Observations: []observation{{Strategy: "scalar", Outcome: "database_error"}}}
	if err := writeReport(path, value); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("private file mode missing")
	}
	body, _ := os.ReadFile(path)
	var decoded map[string]any
	if json.Unmarshal(body, &decoded) != nil {
		t.Fatal("invalid aggregate")
	}
	for _, key := range []string{"sql", "args", "digest", "case", "query", "rows_data"} {
		if _, ok := decoded[key]; ok {
			t.Fatal("private report field")
		}
	}
	if err := writeReport(path, aggregate{}); err == nil {
		t.Fatal("existing evidence overwritten")
	}
	again, _ := os.ReadFile(path)
	if string(again) != string(body) {
		t.Fatal("evidence changed")
	}
}

func TestInputIsBoundedPrivateAndValidated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cases.private.json")
	body := `[{"path":"/api/search","query":"dataset=invented-a&postcode=AA1&sort=name"},{"path":"/api/search","query":"dataset=invented-b&postcode=BB2&sort=name"}]`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readCases(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("PRIVATE", 700000)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readCases(path); err == nil || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal("unbounded or leaked input")
	}
}

func TestExistingEvidenceStopsBeforeAnyDatabaseWork(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "cases.private.json")
	output := filepath.Join(dir, "report.json")
	cases := `[{"path":"/api/search","query":"dataset=invented-a&postcode=AA1&sort=name"},{"path":"/api/search","query":"dataset=invented-b&postcode=BB2&sort=name"}]`
	os.WriteFile(input, []byte(cases), 0600)
	os.WriteFile(output, []byte("existing"), 0600)
	called := false
	exec := execution{capture: func(context.Context, []workloadCase) ([]statement, error) { called = true; return nil, nil }}
	if runWith(context.Background(), []string{"--cases", input, "--output", output}, exec) == nil || called {
		t.Fatal("existing evidence did not prevent database work")
	}
}

func TestCaptureFailureStillWritesPrivateAggregateWithoutErrorValues(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "cases.private.json")
	output := filepath.Join(dir, "report.json")
	cases := `[{"path":"/api/search","query":"dataset=invented-a&postcode=AA1&sort=name"},{"path":"/api/search","query":"dataset=invented-b&postcode=BB2&sort=name"}]`
	os.WriteFile(input, []byte(cases), 0600)
	exec := execution{capture: func(context.Context, []workloadCase) ([]statement, error) {
		return []statement{{SQL: "SECRET_SQL"}}, errors.New("SECRET_DSN")
	}}
	if runWith(context.Background(), []string{"--cases", input, "--output", output}, exec) == nil {
		t.Fatal("capture failure accepted")
	}
	body, err := os.ReadFile(output)
	if err != nil || strings.Contains(string(body), "SECRET") {
		t.Fatal("capture failure not safely recorded")
	}
	var report aggregate
	if json.Unmarshal(body, &report) != nil || report.Complete || report.Stage != "capture" || report.Captures != 1 {
		t.Fatal("capture failure relabelled")
	}
}

func TestCatalogStandardLoggerCannotBypassSanitization(t *testing.T) {
	var logged bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logged)
	defer log.SetOutput(previous)
	dir := t.TempDir()
	input := filepath.Join(dir, "cases.private.json")
	output := filepath.Join(dir, "report.json")
	os.WriteFile(input, []byte(`[{"path":"/api/search","query":"dataset=invented-a&postcode=AA1&sort=name"},{"path":"/api/search","query":"dataset=invented-b&postcode=BB2&sort=name"}]`), 0600)
	exec := execution{capture: func(context.Context, []workloadCase) ([]statement, error) {
		log.Print("SECRET_DATABASE_ERROR")
		return nil, errors.New("SECRET")
	}}
	if runWith(context.Background(), []string{"--cases", input, "--output", output}, exec) == nil {
		t.Fatal("capture failure accepted")
	}
	if strings.Contains(logged.String(), "SECRET") {
		t.Fatal("catalog logger leaked private error")
	}
	log.Print("restored")
	if !strings.Contains(logged.String(), "restored") {
		t.Fatal("logger output not restored")
	}
}
