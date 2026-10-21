package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestPairedAlternatesOrderAndKeepsValuesPrivate(t *testing.T) {
	var mu sync.Mutex
	order := []string{}
	server := func(label, cursor string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			order = append(order, label)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []int{}, "next_cursor": cursor, "private": "SECRET_BODY"})
		}))
	}
	before, after := server("b", "SECRET_BEFORE"), server("a", "SECRET_AFTER")
	defer before.Close()
	defer after.Close()
	cases, out := workload(t, []queryCase{{Query: "surname=SECRET_ONE", DigestMode: "cursor-neutral-v1"}, {Query: "surname=SECRET_TWO", DigestMode: "cursor-neutral-v1"}})
	var log bytes.Buffer
	if err := run([]string{"--url", after.URL, "--compare-url", before.URL, "--probe-only", "--repeats", "2", "--cases", cases, "--output", out}, &log); err != nil {
		t.Fatal(err)
	}
	r := report(t, out)
	var outcomes map[string]int
	if json.Unmarshal(r["outcomes"], &outcomes) != nil || outcomes["both_ok_same"] != 4 {
		t.Fatal("paired results", outcomes)
	}
	mu.Lock()
	got := strings.Join(order, ",")
	mu.Unlock()
	if got != "b,a,a,b,a,b,b,a" {
		t.Fatal("unbalanced request order", got)
	}
	if strings.Contains(log.String(), "SECRET") {
		t.Fatal("private output")
	}
}
func TestPairedDetectsMismatchAndCandidateFailure(t *testing.T) {
	before := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[1]}`))
	}))
	defer before.Close()
	after := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("surname") == "SECRET_FAIL" {
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"code":"query_timeout"}`))
			return
		}
		_, _ = w.Write([]byte(`{"items":[2]}`))
	}))
	defer after.Close()
	cases, out := workload(t, []queryCase{{Query: "surname=SECRET_DIFFERENT"}, {Query: "surname=SECRET_FAIL"}})
	if err := run([]string{"--url", after.URL, "--compare-url", before.URL, "--probe-only", "--cases", cases, "--output", out}, &bytes.Buffer{}); err == nil {
		t.Fatal("candidate regression accepted")
	}
	var outcomes map[string]int
	if json.Unmarshal(report(t, out)["outcomes"], &outcomes) != nil || outcomes["both_ok_mismatch"] != 1 || outcomes["before_only_ok"] != 1 {
		t.Fatal("regression outcomes", outcomes)
	}
}
func TestPairedRetainsIncompleteRequest(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer s.Close()
	cases, out := workload(t, []queryCase{{Query: "surname=SECRET_WAIT"}})
	if err := run([]string{"--url", s.URL, "--compare-url", s.URL, "--probe-only", "--cases", cases, "--output", out, "--max-duration", "20ms"}, &bytes.Buffer{}); err == nil {
		t.Fatal("incomplete pair accepted")
	}
	r := report(t, out)
	var outcomes map[string]int
	_ = json.Unmarshal(r["outcomes"], &outcomes)
	if outcomes["incomplete_pair"] != 1 || string(r["budget_exhausted"]) != "true" || string(r["requests_attempted"]) != "1" {
		t.Fatal("lost incomplete attempt")
	}
}
