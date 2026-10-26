package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotComparisonKeepsFailuresAndExcludesDigests(t *testing.T) {
	root := t.TempDir()
	a := privateSnapshot{Cases: 4, Complete: true, Side: "native"}
	b := privateSnapshot{Cases: 4, Complete: true, Side: "candidate"}
	hash := strings.Repeat("ab", 32)
	for i := 0; i < 4; i++ {
		x := privateSnapshotRow{snapshotObservation: snapshotObservation{Case: i, Status: 200, Success: true}, Digest: hash}
		y := x
		if i == 1 || i == 3 {
			x.Success = false
			x.Status = 503
			x.Code = "query_timeout"
			x.Digest = ""
		}
		if i == 2 || i == 3 {
			y.Success = false
			y.Status = 503
			y.Code = "query_timeout"
			y.Digest = ""
		}
		a.Rows = append(a.Rows, x)
		b.Rows = append([]privateSnapshotRow{y}, b.Rows...)
	}
	pa, pb, out := filepath.Join(root, "native.private.json"), filepath.Join(root, "candidate.private.json"), filepath.Join(root, "report.json")
	if e := save(pa, a); e != nil {
		t.Fatal(e)
	}
	if e := save(pb, b); e != nil {
		t.Fatal(e)
	}
	if e := compareSnapshots(pa, pb, out); e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile(out)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(data), hash) || strings.Contains(string(data), "sha256") {
		t.Fatal("private digest exported")
	}
	var report struct {
		Complete bool
		Outcomes map[string]int
		Requests int `json:"requests_per_backend"`
	}
	if json.Unmarshal(data, &report) != nil || !report.Complete || report.Requests != 4 {
		t.Fatal("incomplete cohort")
	}
	for _, key := range []string{"both_ok_same", "before_only_ok", "after_only_ok", "both_failed"} {
		if report.Outcomes[key] != 1 {
			t.Fatal("failures or successes omitted", key)
		}
	}
	b.Rows[0].Case = b.Rows[1].Case
	bad := filepath.Join(root, "bad.private.json")
	if e = save(bad, b); e != nil {
		t.Fatal(e)
	}
	if _, e = readSnapshot(bad); e == nil {
		t.Fatal("duplicate case accepted")
	}
}
