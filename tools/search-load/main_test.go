package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func workload(t *testing.T, cases []queryCase) (string, string) {
	t.Helper()
	root := t.TempDir()
	data, _ := json.Marshal(cases)
	p := filepath.Join(root, "private.json")
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}
	return p, filepath.Join(root, "report.json")
}
func digest(body string) string { x := sha256.Sum256([]byte(body)); return hex.EncodeToString(x[:]) }
func report(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("SECRET")) {
		t.Fatal("private value leaked into report")
	}
	var r map[string]json.RawMessage
	if err = json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return r
}
func TestProbeReportsFailuresAndContinues(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("surname") == "SECRET_BAD" {
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"code":"query_timeout","error":"SECRET_SERVER_ERROR"}`))
			return
		}
		_, _ = w.Write([]byte(`{"items":[],"private":"SECRET_BODY"}`))
	}))
	defer s.Close()
	cases, out := workload(t, []queryCase{{Query: "dataset=all&surname=SECRET_BAD"}, {Query: "dataset=SECRET_EDITION&town=SECRET_TOWN"}})
	var log bytes.Buffer
	err := run([]string{"-url", s.URL, "-cases", cases, "-output", out, "-probe-only", "-repeats", "2"}, &log)
	if err == nil || calls.Load() != 4 {
		t.Fatalf("failed observation stopped workload: calls=%d err=%v", calls.Load(), err)
	}
	r := report(t, out)
	var obs []observation
	if json.Unmarshal(r["initial_observations"], &obs) != nil || len(obs) != 4 || obs[0].Code != "query_timeout" || obs[1].Outcome != "unverified" || obs[1].Family != "address" {
		t.Fatal("incorrect phase report")
	}
	if strings.Contains(log.String(), "SECRET") {
		t.Fatal("private value leaked to stdout")
	}
}
func TestLoadChecksHashesAndPerCaseErrors(t *testing.T) {
	body := `{"items":[]}`
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
	defer s.Close()
	cases, out := workload(t, []queryCase{{Query: "surname=SECRET", SHA256: digest(body)}, {Query: "postcode=SECRET", SHA256: strings.Repeat("0", 64)}})
	var log bytes.Buffer
	err := run([]string{"-url", s.URL, "-cases", cases, "-output", out, "-duration", "30ms", "-concurrency", "1,2", "-rate", "0", "-max-requests", "10000"}, &log)
	if err == nil {
		t.Fatal("hash mismatch returned success")
	}
	r := report(t, out)
	var stages map[string]stageReport
	if json.Unmarshal(r["closed_loop"], &stages) != nil {
		t.Fatal("invalid stages")
	}
	for _, clients := range []string{"1", "2"} {
		s := stages[clients]
		if s.Errors == 0 || s.Cases[1].Errors == 0 || s.Cases[0].Errors != 0 || s.Outcomes["hash_mismatch"] == 0 {
			t.Fatal("stage hid per-case failures", clients)
		}
	}
}
func TestRequestBudgetBoundsScheduledLoad(t *testing.T) {
	var calls atomic.Int32
	body := `{"items":[]}`
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); _, _ = w.Write([]byte(body)) }))
	defer s.Close()
	cases, out := workload(t, []queryCase{{Query: "surname=SECRET", SHA256: digest(body)}})
	var log bytes.Buffer
	err := run([]string{"-url", s.URL, "-cases", cases, "-output", out, "-duration", "30ms", "-concurrency", "4", "-rate", "100", "-arrival-duration", "30ms", "-max-requests", "5"}, &log)
	if err == nil || calls.Load() > 5 {
		t.Fatalf("request budget exceeded: %d %v", calls.Load(), err)
	}
	r := report(t, out)
	if string(r["budget_exhausted"]) != "true" {
		t.Fatal("incomplete workload reported as complete")
	}
}
func TestLoadRequiresHashesAndRejectsRedirects(t *testing.T) {
	var destination atomic.Int32
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { destination.Add(1) }))
	defer sink.Close()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, sink.URL, http.StatusFound) }))
	defer s.Close()
	cases, out := workload(t, []queryCase{{Query: "surname=SECRET"}})
	var log bytes.Buffer
	if run([]string{"-url", s.URL, "-cases", cases, "-output", out}, &log) == nil {
		t.Fatal("unverified workload entered load stages")
	}
	if run([]string{"-url", s.URL, "-cases", cases, "-output", out, "-probe-only"}, &log) == nil || destination.Load() != 0 {
		t.Fatal("redirect followed or accepted")
	}
	report(t, out)
}

func TestCursorNeutralDigestPreservesResultsAndPageAvailability(t *testing.T) {
	a := []byte(`{"items":[{"id":1,"record":{"name":"SECRET"}}],"next_cursor":"physical-one"}`)
	b := []byte(`{"next_cursor":"physical-two","items":[{"id":1,"record":{"name":"SECRET"}}]}`)
	x, err := responseDigest(a, "cursor-neutral-v1")
	if err != nil {
		t.Fatal(err)
	}
	y, err := responseDigest(b, "cursor-neutral-v1")
	if err != nil || x != y {
		t.Fatal("opaque cursor changed result digest")
	}
	for _, body := range []string{`{"items":[{"id":2,"record":{"name":"SECRET"}}],"next_cursor":"physical-one"}`, `{"items":[{"id":1,"record":{"name":"SECRET"}}],"next_cursor":""}`} {
		z, e := responseDigest([]byte(body), "cursor-neutral-v1")
		if e != nil || z == x {
			t.Fatal("content or page-availability difference ignored")
		}
	}
	if _, e := responseDigest([]byte(`{"next_cursor":42}`), "cursor-neutral-v1"); e == nil {
		t.Fatal("malformed cursor accepted")
	}
	rawA, _ := responseDigest(a, "")
	rawB, _ := responseDigest(b, "")
	if rawA == rawB {
		t.Fatal("legacy body hash changed semantics")
	}
}

func TestNDJSONExportsRemainValid(t *testing.T) {
	if !validResponse([]byte("{\"record\":1}\n{\"record\":2}\n"), "application/x-ndjson") || !validResponse(nil, "application/x-ndjson") {
		t.Fatal("valid export rejected")
	}
	if validResponse([]byte("{\"record\":1}\nNOT_JSON\n"), "application/x-ndjson") {
		t.Fatal("invalid export accepted")
	}
}
