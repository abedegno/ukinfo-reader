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
