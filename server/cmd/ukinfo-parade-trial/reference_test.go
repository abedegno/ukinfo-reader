package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotSourceReferenceGate(t *testing.T) {
	for _, tc := range []struct {
		name, digest             string
		success                  bool
		wantErrors, wantMismatch int
	}{
		{"match", strings.Repeat("a", 64), true, 0, 0},
		{"different", strings.Repeat("b", 64), true, 0, 1},
		{"timeout", "", false, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			reference, snapshot, output := filepath.Join(dir, "reference"), filepath.Join(dir, "snapshot"), filepath.Join(dir, "report")
			write := func(path string, v any) {
				t.Helper()
				b, e := json.Marshal(v)
				if e != nil {
					t.Fatal(e)
				}
				if e = os.WriteFile(path, b, 0600); e != nil {
					t.Fatal(e)
				}
			}
			privateDigest := strings.Repeat("a", 64)
			write(reference, nativeReferenceFile{Cases: 1, Rows: []referenceRow{{Case: 0, Page: 1, Digest: privateDigest}}})
			status := 200
			if !tc.success {
				status = 503
			}
			write(snapshot, privateSnapshot{Cases: 1, Complete: true, Side: "candidate", Rows: []privateSnapshotRow{{snapshotObservation: snapshotObservation{Case: 0, Status: status, Success: tc.success}, Digest: tc.digest}}})
			e := compareSnapshotReference(reference, snapshot, output)
			if (e != nil) != (tc.wantErrors+tc.wantMismatch > 0) {
				t.Fatal("unexpected gate result", e)
			}
			raw, e := os.ReadFile(output)
			if e != nil {
				t.Fatal(e)
			}
			var report struct {
				Complete                     bool `json:"complete"`
				Requests, Errors, Mismatches int
			}
			if e = json.Unmarshal(raw, &report); e != nil || !report.Complete || report.Requests != 1 || report.Errors != tc.wantErrors || report.Mismatches != tc.wantMismatch {
				t.Fatal("invalid aggregate report", report, e)
			}
			if strings.Contains(string(raw), privateDigest) || strings.Contains(string(raw), "sha256") || strings.Contains(string(raw), "next_cursor") {
				t.Fatal("private field escaped into aggregate report")
			}
			if e = compareSnapshotReference(reference, snapshot, output); e == nil {
				t.Fatal("existing evidence overwritten")
			}
		})
	}
}
