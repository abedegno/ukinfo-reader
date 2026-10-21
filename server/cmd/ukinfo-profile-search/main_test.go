package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPlansExcludeQueryValues(t *testing.T) {
	raw := map[string]any{"Node Type": "Hash Join", "Total Cost": 12.0, "Hash Cond": "name='SECRET_NAME'", "Output": []any{"SECRET_OUTPUT"}, "Plans": []any{map[string]any{"Node Type": "Index Scan", "Relation Name": "terms", "Index Name": "terms_pkey", "Index Cond": "id='SECRET_ID'", "Filter": "key='SECRET_KEY'", "Plan Rows": 2.0}}}
	b, err := json.Marshal(cleanPlan(raw))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "SECRET") || !strings.Contains(string(b), "terms_pkey") {
		t.Fatal("plan privacy or useful index metadata lost")
	}
}
