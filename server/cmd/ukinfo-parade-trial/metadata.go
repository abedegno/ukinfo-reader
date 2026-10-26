package main

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"

	"github.com/abedegno/ukinfo-reader/server/internal/catalog"
)

type caseMetadata struct {
	Name           bool   `json:"name"`
	First          bool   `json:"first"`
	Case           int    `json:"case"`
	Route          string `json:"route"`
	Scope          string `json:"scope"`
	EditionOrdinal int    `json:"edition_ordinal"`
	Kind           string `json:"kind"`
	Backend        string `json:"expected_backend"`
	Sort           string `json:"sort"`
	Postcode       bool   `json:"postcode"`
	Address        bool   `json:"address"`
	Refinements    bool   `json:"refinements"`
}

func metadata(path, output string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return errors.New("private cases unavailable")
	}
	var cases []catalog.ParadeTrialCase
	if json.Unmarshal(raw, &cases) != nil || len(cases) == 0 || len(cases) > 500 {
		return errors.New("invalid bounded workload")
	}
	values := []caseMetadata{}
	editions := map[string]int{}
	for i, c := range cases {
		v, e := url.ParseQuery(c.Query)
		if e != nil {
			return errors.New("invalid private query")
		}
		row := caseMetadata{Case: i, Route: c.Path, Scope: "edition", Kind: "other", Backend: "native", Sort: "catalogue"}
		switch c.Path {
		case "/api/search", "/api/export", "/api/record", "/api/address", "/api/group":
		default:
			return errors.New("unsupported private route")
		}
		if v.Get("dataset") == "all" {
			row.Scope = "all"
		} else {
			key := v.Get("dataset")
			if editions[key] == 0 {
				editions[key] = len(editions) + 1
			}
			row.EditionOrdinal = editions[key]
		}
		if c.Path == "/api/search" || c.Path == "/api/export" {
			row.Kind = "person"
			if v.Get("kind") == "business" {
				row.Kind = "business"
			}
			switch v.Get("sort") {
			case "", "catalogue":
			case "name", "edition", "nearest":
				row.Sort = v.Get("sort")
			default:
				return errors.New("unsupported private sort")
			}
			row.Backend, e = catalog.ParadeTrialBackend(c)
			if e != nil {
				return e
			}
			row.Name = v.Get("surname") != "" || v.Get("organisation") != ""
			row.First = v.Get("first_name") != ""
			row.Postcode = v.Get("postcode") != ""
			for _, key := range []string{"street", "locality", "town", "county"} {
				row.Address = row.Address || v.Get(key) != ""
			}
			for _, key := range []string{"house", "building", "flat", "middle_initial"} {
				row.Refinements = row.Refinements || v.Get(key) != ""
			}
		}
		values = append(values, row)
	}
	return save(output, struct {
		Cases    int            `json:"cases"`
		Editions int            `json:"editions"`
		Values   []caseMetadata `json:"metadata"`
	}{len(cases), len(editions), values})
}

// subset writes only a private pilot workload; values never enter its log.
func subset(path, dataset, output string) error {
	if dataset == "" || dataset == "all" {
		return errors.New("choose one visible pilot edition")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return errors.New("private workload unavailable")
	}
	var cases []catalog.ParadeTrialCase
	if json.Unmarshal(raw, &cases) != nil || len(cases) == 0 || len(cases) > 500 {
		return errors.New("invalid private workload")
	}
	selected := []catalog.ParadeTrialCase{}
	for _, c := range cases {
		v, e := url.ParseQuery(c.Query)
		if e != nil {
			return errors.New("invalid private query")
		}
		if v.Get("dataset") == dataset {
			selected = append(selected, c)
		}
	}
	if len(selected) == 0 {
		return errors.New("pilot edition has no cases")
	}
	return save(output, selected)
}

// merge keeps the existing regression searches on the NAS and normalizes only
// workload serialization. It never substitutes raw-cursor hashing for JSON pages.
func merge(path, seeds, output string) error {
	out := []catalog.ParadeTrialCase{}
	seen := map[string]bool{}
	for _, file := range []string{path, seeds} {
		raw, err := os.ReadFile(file)
		if err != nil {
			return errors.New("private workload input unavailable")
		}
		var cases []catalog.ParadeTrialCase
		if json.Unmarshal(raw, &cases) != nil || len(cases) == 0 || len(cases) > 500 {
			return errors.New("invalid bounded private workload")
		}
		for _, c := range cases {
			if c.Path == "" {
				c.Path = "/api/search"
			}
			switch c.Path {
			case "/api/search", "/api/export", "/api/record", "/api/address", "/api/group":
			default:
				return errors.New("unsupported private route")
			}
			v, e := url.ParseQuery(c.Query)
			if e != nil {
				return errors.New("invalid private query")
			}
			c.Query = v.Encode()
			if c.Path == "/api/export" {
				c.DigestMode = ""
			} else {
				c.DigestMode = "cursor-neutral-v1"
			}
			key := c.Path + "?" + c.Query
			if !seen[key] {
				out = append(out, c)
				seen[key] = true
			}
		}
	}
	if len(out) > 500 {
		return errors.New("merged workload exceeds bounded cohort")
	}
	return save(output, out)
}
