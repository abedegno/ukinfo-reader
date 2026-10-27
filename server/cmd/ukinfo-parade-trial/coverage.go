package main

import (
	"errors"
	"net/url"
	"strings"

	"github.com/abedegno/ukinfo-reader/server/internal/catalog"
)

type coverageShape struct {
	Case    int    `json:"case"`
	Parent  int    `json:"parent_case"`
	Family  string `json:"family"`
	Scope   string `json:"scope"`
	Backend string `json:"expected_backend"`
}

// Build a separate suite from frozen inputs, never from successful responses to
// the new searches. A failed scenario stays in the suite; it is not calibrated
// away. Queries remain private; the companion report contains fixed shapes only.
func extendCoverage(path, output, report string) error {
	if output == report || report == "" {
		return errors.New("separate aggregate coverage report required")
	}
	parents, err := referenceCases(path)
	if err != nil {
		return err
	}
	cases, shapes, err := coverageCases(parents)
	if err != nil {
		return err
	}
	if err = save(output, cases); err != nil {
		return err
	}
	return save(report, struct {
		Cases   int             `json:"cases"`
		Parents int             `json:"parent_cases"`
		Shapes  []coverageShape `json:"shapes"`
	}{len(cases), len(parents), shapes})
}

func coverageCases(parents []catalog.ParadeTrialCase) ([]catalog.ParadeTrialCase, []coverageShape, error) {
	type specification struct{ family, scope string }
	specs := []specification{
		{"wildcard_surname", "all"}, {"first_prefix", "edition"}, {"wildcard_business", "all"},
		{"street_only", "edition"}, {"street_only", "all"},
		{"county_only", "edition"}, {"county_only", "all"},
		{"nearby_max", "all"}, {"nearby_district", "edition"}, {"nearby_sector", "all"},
		{"dedupe", "all"}, {"dedupe_ordered", "all"},
	}
	seeds := []url.Values{}
	for _, c := range parents {
		if c.Path != "/api/search" {
			seeds = append(seeds, nil)
			continue
		}
		v, err := url.ParseQuery(c.Query)
		if err != nil {
			return nil, nil, errors.New("invalid coverage seed")
		}
		seeds = append(seeds, v)
	}
	cases, shapes := []catalog.ParadeTrialCase{}, []coverageShape{}
	for _, spec := range specs {
		found := false
		for parent, seed := range seeds {
			if seed == nil || seed.Get("dataset") == "" || seed.Get("dataset") == "all" {
				continue
			}
			v := url.Values{"dataset": {seed.Get("dataset")}, "kind": {"person"}, "limit": {"7"}}
			if spec.scope == "all" {
				v.Set("dataset", "all")
			}
			if !coverageScenario(spec.family, seed, v) {
				continue
			}
			c := catalog.ParadeTrialCase{Path: "/api/search", Query: v.Encode(), DigestMode: "cursor-neutral-v1"}
			backend, err := catalog.ParadeTrialBackend(c)
			if err != nil {
				continue
			} // Syntax-only seed suitability, no query execution.
			shapes = append(shapes, coverageShape{len(cases), parent, spec.family, spec.scope, backend})
			cases = append(cases, c)
			found = true
			break
		}
		if !found {
			return nil, nil, errors.New("missing seed for coverage family: " + spec.family)
		}
	}
	return cases, shapes, nil
}

func coverageScenario(family string, seed, v url.Values) bool {
	person := seed.Get("kind") != "business"
	exact := func(value string) bool { return value != "" && !strings.ContainsAny(value, "*?%") }
	prefix := func(value string) string {
		r := []rune(value)
		if len(r) < 4 || !exact(value) {
			return ""
		}
		return string(r[:len(r)-1]) + "*"
	}
	surname, first := seed.Get("surname"), seed.Get("first_name")
	given := strings.Fields(first)
	switch family {
	case "wildcard_surname", "first_prefix":
		if !person || !exact(surname) || len(given) == 0 || !exact(first) {
			return false
		}
		v.Set("surname", surname)
		v.Set("first_name", given[0])
		v.Set("first_match", "given")
		if family == "wildcard_surname" {
			p := prefix(surname)
			if p == "" {
				return false
			}
			v.Set("surname", p)
		} else {
			p := prefix(given[0])
			if p == "" {
				return false
			}
			v.Set("first_name", p)
			v.Set("first_match", "prefix")
		}
	case "wildcard_business":
		p := prefix(seed.Get("organisation"))
		if person || p == "" || !exact(seed.Get("postcode")) {
			return false
		}
		v.Set("kind", "business")
		v.Set("organisation", p)
		v.Set("postcode", seed.Get("postcode"))
		v.Set("sort", "name")
	case "street_only", "county_only":
		field := "street"
		if family == "county_only" {
			field = "county"
		}
		if !person || !exact(seed.Get(field)) {
			return false
		}
		v.Set(field, seed.Get(field))
	case "nearby_max", "nearby_district", "nearby_sector":
		near := catalog.Postcode(seed.Get("near"))
		if !person || len(near) < 5 || len(near) > 7 || !exact(near) {
			return false
		}
		// Require a complete unit postcode: partial inputs are deliberately built
		// below, preserving the district/sector boundary with an explicit space.
		n := len(near)
		if near[n-3] < '0' || near[n-3] > '9' || near[n-2] < 'A' || near[n-2] > 'Z' || near[n-1] < 'A' || near[n-1] > 'Z' {
			return false
		}
		field := "street"
		if !exact(seed.Get(field)) {
			field = "town"
		}
		if !exact(seed.Get(field)) {
			return false
		}
		v.Set(field, seed.Get(field))
		v.Set("near", near)
		v.Set("radius_m", "50000")
		if family == "nearby_district" {
			v.Set("near", near[:n-3])
		}
		if family == "nearby_sector" {
			v.Set("near", near[:n-3]+" "+near[n-3:n-2])
		}
	case "dedupe", "dedupe_ordered":
		if !person || !exact(surname) || !exact(first) {
			return false
		}
		field := "town"
		if !exact(seed.Get(field)) {
			field = "postcode"
		}
		if !exact(seed.Get(field)) {
			return false
		}
		v.Set("surname", surname)
		v.Set("first_name", first)
		v.Set("first_match", "given")
		v.Set(field, seed.Get(field))
		v.Set("group", "listing")
		v.Set("limit", "3")
		if family == "dedupe_ordered" {
			v.Set("sort", "name")
		}
	default:
		return false
	}
	return true
}
