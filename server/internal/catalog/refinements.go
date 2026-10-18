package catalog

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"unicode"
)

func isLetter(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return s != ""
}
func (q query) hasRefinements() bool {
	return q.House != "" || q.Building != "" || q.Flat != "" || q.Middle != "" || (q.First != "" && q.FirstMatch == "given" && !strings.HasSuffix(q.First, "*"))
}
func textMatches(value, filter string) bool {
	value = Normalize(value)
	if strings.HasSuffix(filter, "*") {
		return strings.HasPrefix(value, strings.TrimSuffix(filter, "*"))
	}
	return value == filter
}

// Refine only displayed fields: raw references and hidden postcode precision are
// never used to supply missing house/building/flat or name information.
func (q query) accepts(raw json.RawMessage) (bool, error) {
	var row map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&row); err != nil {
		return false, err
	}
	for _, filter := range []struct{ field, value string }{{"houseno", q.House}, {"building", q.Building}, {"subbuilding", q.Flat}} {
		if filter.value == "" {
			continue
		}
		value := ""
		switch v := row[filter.field].(type) {
		case string:
			value = v
		case json.Number:
			if filter.field == "houseno" {
				value = v.String()
				if number, err := v.Float64(); err == nil && number == 0 {
					value = ""
				}
			}
		}
		if filter.field == "houseno" && value == "0" {
			value = ""
		}
		if value == "" || !textMatches(value, filter.value) {
			return false, nil
		}
	}
	first := Normalize(Text(row, "firstName"))
	if q.First != "" && q.FirstMatch == "given" && !strings.HasSuffix(q.First, "*") && first != q.First && !strings.HasPrefix(first, q.First+" ") {
		return false, nil
	}
	if q.Middle != "" {
		parts := strings.Fields(strings.ReplaceAll(first, ".", " "))
		if len(parts) < 2 || !strings.HasPrefix(parts[1], q.Middle) {
			return false, nil
		}
	}
	return true, nil
}

// Search complete candidate pages until the requested filtered page plus lookahead
// is known. The shared request deadline bounds all batches; no candidate-count cap
// can silently turn a sparse match into a false end of results.
func (a *API) searchResults(ctx context.Context, q query) (searchResult, int, error) {
	if q.Group != "" || q.Sort != "" {
		return a.orderedResults(ctx, q)
	}
	if !q.hasRefinements() {
		return a.searchCandidates(ctx, q)
	}
	limit := q.Limit
	q.Limit = 100
	output := searchResult{Items: []result{}}
	var last cursor
	for {
		batch, status, err := a.searchCandidates(ctx, q)
		if err != nil {
			return searchResult{}, status, err
		}
		output.Geography = batch.Geography
		for i, item := range batch.Items {
			accepted, err := q.accepts(item.Record)
			if err != nil {
				return searchResult{}, 503, err
			}
			if !accepted {
				continue
			}
			if len(output.Items) == limit {
				raw, _ := json.Marshal(last)
				output.Next = base64.RawURLEncoding.EncodeToString(raw)
				return output, 200, nil
			}
			output.Items = append(output.Items, item)
			last = batch.positions[i]
		}
		if batch.Next == "" {
			return output, 200, nil
		}
		q.Cursor = batch.Next
	}
}

func (q query) hasBusinessFilters() bool { return q.SIC != "" || q.Industry != "" }
func (q query) needsResearch() bool      { return q.Group != "" || q.hasBusinessFilters() }
