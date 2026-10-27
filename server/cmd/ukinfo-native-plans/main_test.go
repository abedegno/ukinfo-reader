package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPlanSummaryOmitsPredicatesAndNames(t *testing.T) {
	root := map[string]any{"Node Type": "Sort", "Actual Rows": float64(51), "Sort Key": []any{"private surname"}, "Filter": "private address", "Relation Name": "private table", "Plans": []any{map[string]any{"Node Type": "Index Scan", "Index Cond": "private postcode", "Shared Read Blocks": float64(23)}}}
	raw, e := json.Marshal(summarize(root))
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(raw), "private") {
		t.Fatal("plan details leaked")
	}
	if !strings.Contains(string(raw), `"read_blocks":23`) || !strings.Contains(string(raw), `"rows":51`) {
		t.Fatal("structural counters missing")
	}
}

func TestDiagnosticPhase(t *testing.T) {
	for _, tc := range []struct{ sql, want string }{
		{"SELECT id,key,manifest,entries FROM datasets", "inventory"},
		{"SELECT id,key FROM terms WHERE key=$1", "vocabulary"},
		{"SELECT group_key,count(*) FROM research_keys GROUP BY group_key", "group_counts"},
		{"SELECT wanted.group_key,found.id FROM unnest($2::bytea[]) wanted(group_key)", "group_representatives"},
		{"SELECT id,group_key FROM research_keys WHERE dataset_id=$1", "group_keys"},
		{"SELECT payload FROM record_blocks WHERE id=$1", "records"},
		{"WITH ordered_candidates AS (SELECT * FROM entries) SELECT * FROM ordered_candidates", "ordered"},
		{"SELECT id FROM entries WHERE dataset_id=$1", "source_candidates"},
		{"SELECT t.id FROM postcode_locations p JOIN terms t ON t.key=p.postcode", "geography"},
	} {
		if got := diagnosticPhase(tc.sql); got != tc.want {
			t.Fatalf("phase=%s want=%s", got, tc.want)
		}
	}
}
