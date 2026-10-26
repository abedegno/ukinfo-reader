package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/abedegno/ukinfo-reader/server/internal/catalog"
)

type snapshotObservation struct {
	Case    int     `json:"case"`
	Status  int     `json:"status"`
	MS      float64 `json:"milliseconds"`
	Code    string  `json:"code,omitempty"`
	Success bool    `json:"success"`
	Next    bool    `json:"has_next"`
}
type privateSnapshotRow struct {
	snapshotObservation
	Digest string `json:"sha256,omitempty"`
}
type privateSnapshot struct {
	Cases    int                  `json:"cases"`
	Complete bool                 `json:"complete"`
	Side     string               `json:"side"`
	Rows     []privateSnapshotRow `json:"rows"`
}

func captureSnapshot(ctx context.Context, path, output, report, side string, reverse bool, ordinal int) error {
	if report == "" || output == report {
		return errors.New("require separate private snapshot and aggregate report")
	}
	index := -1
	switch side {
	case "native":
		index = 0
	case "candidate":
		index = 1
	default:
		return errors.New("invalid private backend")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		return errors.New("private cases unavailable")
	}
	var cases []catalog.ParadeTrialCase
	if json.Unmarshal(raw, &cases) != nil || len(cases) == 0 || len(cases) > 500 || ordinal < -1 || ordinal >= len(cases) {
		return errors.New("invalid bounded snapshot workload")
	}
	client := http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{Proxy: nil, MaxIdleConnsPerHost: 2}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	snap := privateSnapshot{Cases: len(cases), Side: side, Rows: []privateSnapshotRow{}}
	observations := []snapshotObservation{}
	failures := 0
	for i := range cases {
		n := i
		if reverse {
			n = len(cases) - 1 - i
		}
		if ordinal >= 0 && n != ordinal {
			continue
		}
		if ctx.Err() != nil {
			break
		}
		p := privateRead(ctx, &client, index, cases[n], "")
		o := snapshotObservation{Case: n, Status: p.status, MS: p.ms, Code: p.code, Success: p.hash != "", Next: p.next != ""}
		if !o.Success {
			failures++
		}
		observations = append(observations, o)
		snap.Rows = append(snap.Rows, privateSnapshotRow{snapshotObservation: o, Digest: p.hash})
	}
	expected := len(cases)
	if ordinal >= 0 {
		expected = 1
	}
	snap.Complete = ctx.Err() == nil && len(snap.Rows) == expected
	if e = save(output, snap); e != nil {
		return e
	}
	if e = save(report, map[string]any{"cases": snap.Cases, "requests": len(observations), "complete": snap.Complete, "errors": failures, "side": side, "reverse": reverse, "observations": observations}); e != nil {
		return e
	}
	if !snap.Complete {
		return errors.New("snapshot did not complete")
	}
	return nil
}
func readSnapshot(path string) (privateSnapshot, error) {
	var s privateSnapshot
	raw, e := os.ReadFile(path)
	if e != nil || json.Unmarshal(raw, &s) != nil || !s.Complete || s.Cases < 1 || s.Cases > 500 || len(s.Rows) == 0 || len(s.Rows) > s.Cases {
		return s, errors.New("invalid private snapshot")
	}
	seen := map[int]bool{}
	for _, r := range s.Rows {
		switch r.Code {
		case "", "query_timeout", "invalid_request", "not_found", "source_unavailable", "search_unavailable", "http_error":
		default:
			return s, errors.New("invalid snapshot error code")
		}
		if r.Case < 0 || r.Case >= s.Cases || seen[r.Case] {
			return s, errors.New("invalid snapshot ordinals")
		}
		seen[r.Case] = true
		hash, e := hex.DecodeString(r.Digest)
		if r.Success && (r.Status != 200 || e != nil || len(hash) != 32) || !r.Success && r.Digest != "" {
			return s, errors.New("invalid snapshot digest")
		}
	}
	return s, nil
}
func compareSnapshots(before, after, output string) error {
	a, e := readSnapshot(before)
	if e != nil {
		return e
	}
	b, e := readSnapshot(after)
	if e != nil {
		return e
	}
	if a.Side != "native" || b.Side != "candidate" || a.Cases != b.Cases || len(a.Rows) != len(b.Rows) {
		return errors.New("different snapshot populations")
	}
	other := map[int]privateSnapshotRow{}
	for _, r := range b.Rows {
		other[r.Case] = r
	}
	counts := map[string]int{}
	pairs := []map[string]any{}
	mismatches := 0
	sort.Slice(a.Rows, func(i, j int) bool { return a.Rows[i].Case < a.Rows[j].Case })
	for _, r := range a.Rows {
		s, ok := other[r.Case]
		if !ok {
			return errors.New("different snapshot ordinals")
		}
		outcome := "both_failed"
		switch {
		case r.Success && s.Success:
			outcome = "both_ok_same"
			if r.Digest != s.Digest {
				outcome = "both_ok_mismatch"
				mismatches++
			}
		case r.Success:
			outcome = "before_only_ok"
		case s.Success:
			outcome = "after_only_ok"
		}
		counts[outcome]++
		pairs = append(pairs, map[string]any{"case": r.Case, "outcome": outcome, "before": r.snapshotObservation, "after": s.snapshotObservation})
	}
	if e = save(output, map[string]any{"cases": a.Cases, "requests_per_backend": len(a.Rows), "complete": true, "outcomes": counts, "mismatches": mismatches, "pairs": pairs}); e != nil {
		return e
	}
	if mismatches > 0 {
		return errors.New("snapshot content mismatch; aggregate evidence retained")
	}
	return nil
}
