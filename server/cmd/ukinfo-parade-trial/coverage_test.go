package main

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/abedegno/ukinfo-reader/server/internal/catalog"
)

func TestCoverageSuiteIsSeparateDeterministicAndPrivate(t *testing.T) {
	parents := []catalog.ParadeTrialCase{
		{Path: "/api/search", Query: "dataset=PRIVATE_EDITION&surname=PRIVATE_SURNAME&first_name=PRIVATE_FIRST&street=PRIVATE_STREET&town=PRIVATE_TOWN&county=PRIVATE_COUNTY&postcode=AA11AA&near=AA11AA&radius_m=1000"},
		{Path: "/api/search", Query: "dataset=PRIVATE_EDITION&kind=business&organisation=PRIVATE_BUSINESS&postcode=AA11AA"},
	}
	before, _ := json.Marshal(parents)
	cases, shapes, err := coverageCases(parents)
	if err != nil || len(cases) != 12 || len(shapes) != 12 {
		t.Fatal("incomplete fixed coverage suite", err)
	}
	again, againShapes, err := coverageCases(parents)
	if err != nil || !reflect.DeepEqual(cases, again) || !reflect.DeepEqual(shapes, againShapes) {
		t.Fatal("coverage is not reproducible")
	}
	after, _ := json.Marshal(parents)
	if string(before) != string(after) {
		t.Fatal("frozen parent workload was modified")
	}
	for i, shape := range shapes {
		v, _ := url.ParseQuery(cases[i].Query)
		if cases[i].DigestMode != "cursor-neutral-v1" || cases[i].Path != "/api/search" {
			t.Fatal("incorrect digest or route")
		}
		if (v.Get("dataset") == "all") != (shape.Scope == "all") {
			t.Fatal("wrong scope")
		}
		switch shape.Family {
		case "street_only", "county_only":
			for _, k := range []string{"surname", "first_name", "postcode", "town", "near"} {
				if v.Get(k) != "" {
					t.Fatal("broad address scenario was accidentally narrowed", k)
				}
			}
		case "nearby_max", "nearby_district", "nearby_sector":
			if v.Get("radius_m") != "50000" || v.Get("surname") != "" {
				t.Fatal("wide radius was narrowed")
			}
			if shape.Family == "nearby_sector" && v.Get("near") != "AA1 1" {
				t.Fatal("district/sector boundary lost")
			}
		case "dedupe", "dedupe_ordered":
			if v.Get("group") != "listing" || v.Get("limit") != "3" {
				t.Fatal("missing dedupe continuation")
			}
		}
	}
	directory := t.TempDir()
	input, output, report := filepath.Join(directory, "input.json"), filepath.Join(directory, "coverage.private.json"), filepath.Join(directory, "report.json")
	if err := os.WriteFile(input, before, 0600); err != nil {
		t.Fatal(err)
	}
	if err := extendCoverage(input, output, report); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(report)
	if err != nil || strings.Contains(string(raw), "PRIVATE_") || strings.Contains(string(raw), "AA11AA") || strings.Contains(string(raw), `"query"`) {
		t.Fatal("private inputs in aggregate report", err)
	}
	for _, path := range []string{output, report} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("private evidence permissions", err)
		}
	}
	if err := extendCoverage(input, output, report); err == nil {
		t.Fatal("existing evidence overwritten")
	}
	if _, _, err := coverageCases(parents[:1]); err == nil {
		t.Fatal("missing scenario silently discarded")
	}
}
