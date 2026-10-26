package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/abedegno/ukinfo-reader/server/internal/catalog"
)

var privateBases = []string{"http://ukinfo-parade-preview-native:8080", "http://ukinfo-parade-preview-candidate:8080"}

type privatePage struct {
	status     int
	hash, next string
	ms         float64
	code       string
}

func privateRead(ctx context.Context, client *http.Client, side int, c catalog.ParadeTrialCase, cursor string) (p privatePage) {
	return privateReadTarget(ctx, client, side, c, cursor, false)
}

func privateReadTarget(ctx context.Context, client *http.Client, side int, c catalog.ParadeTrialCase, cursor string, hosted bool) (p privatePage) {
	started := time.Now()
	defer func() { p.ms = float64(time.Since(started).Microseconds()) / 1000 }()
	if side < 0 || side >= len(privateBases) {
		return p
	}
	switch c.Path {
	case "/api/search", "/api/address", "/api/group", "/api/record", "/api/export":
	default:
		return p
	}
	values, e := url.ParseQuery(c.Query)
	if e != nil {
		return p
	}
	if cursor != "" {
		values.Set("cursor", cursor)
	}
	base := privateBases[side]
	if hosted {
		base = "http://ukinfo:8080"
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, base+c.Path+"?"+values.Encode(), nil)
	if e != nil {
		return p
	}
	if hosted {
		req.Host = "ukinfo.6153112.xyz"
	}
	r, e := client.Do(req)
	if e != nil {
		return p
	}
	defer r.Body.Close()
	p.status = r.StatusCode
	body, e := io.ReadAll(io.LimitReader(r.Body, (8<<20)+1))
	if e != nil || len(body) > 8<<20 {
		return p
	}
	if r.StatusCode != 200 {
		var failure struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(body, &failure)
		switch failure.Code {
		case "query_timeout", "invalid_request", "not_found", "source_unavailable", "search_unavailable":
			p.code = failure.Code
		default:
			p.code = "http_error"
		}
		return p
	}
	p.hash, e = digest(body, c.DigestMode)
	if e != nil {
		return p
	}
	var fields struct {
		Next string `json:"next_cursor"`
	}
	_ = json.Unmarshal(body, &fields)
	p.next = fields.Next
	return p
}

type cursorCheck struct {
	Case    int    `json:"case"`
	Outcome string `json:"outcome"`
}
type cursorReport struct {
	Complete        bool          `json:"complete"`
	Cases           int           `json:"cases"`
	FirstMatches    int           `json:"first_matches"`
	Continuations   int           `json:"continuations"`
	NativeErrors    int           `json:"native_errors"`
	CandidateErrors int           `json:"candidate_errors"`
	Mismatches      int           `json:"mismatches"`
	BindingChecks   int           `json:"binding_checks"`
	BindingRejected int           `json:"binding_rejected"`
	Observations    []cursorCheck `json:"observations"`
}

func checkCursors(ctx context.Context, path, output string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return errors.New("private cases unavailable")
	}
	var cases []catalog.ParadeTrialCase
	if json.Unmarshal(raw, &cases) != nil || len(cases) == 0 || len(cases) > 500 {
		return errors.New("invalid bounded workload")
	}
	client := http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{Proxy: nil, MaxIdleConnsPerHost: 2}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	report := cursorReport{Cases: len(cases), Observations: []cursorCheck{}}
	account := func(p privatePage, side int) bool {
		if p.hash != "" {
			return true
		}
		if side == 0 {
			report.NativeErrors++
		} else {
			report.CandidateErrors++
		}
		return false
	}
	for i, c := range cases {
		if c.Path != "/api/search" && c.Path != "/api/address" && c.Path != "/api/group" {
			continue
		}
		obs := cursorCheck{Case: i, Outcome: "first_error"}
		first := [2]privatePage{}
		for k := 0; k < 2; k++ {
			side := (i + k) % 2
			first[side] = privateRead(ctx, &client, side, c, "")
		}
		ok0, ok1 := account(first[0], 0), account(first[1], 1)
		if ok0 && ok1 {
			switch {
			case first[0].hash != first[1].hash:
				obs.Outcome = "first_mismatch"
				report.Mismatches++
			case first[0].next == "" || first[1].next == "":
				obs.Outcome = "no_next"
				report.FirstMatches++
			default:
				report.FirstMatches++
				obs.Outcome = "continuation_error"
				own := [2]privatePage{}
				cross := [2]privatePage{}
				valid := true
				for side := 0; side < 2; side++ {
					own[side] = privateRead(ctx, &client, side, c, first[side].next)
					cross[side] = privateRead(ctx, &client, side, c, first[1-side].next)
					if !account(own[side], side) {
						valid = false
					}
					if !account(cross[side], side) {
						valid = false
					}
				}
				if valid {
					if own[0].hash != own[1].hash || own[0].hash != cross[0].hash || own[1].hash != cross[1].hash {
						obs.Outcome = "continuation_mismatch"
						report.Mismatches++
					} else {
						obs.Outcome = "match"
						report.Continuations++
					}
				}
				// A displayed-field refinement changes the fingerprint without changing the
				// initial candidate set, so native missing-vocabulary shortcuts cannot mask
				// the query-binding check. Run one bounded positive pair only.
				if c.Path == "/api/search" && report.BindingChecks == 0 {
					v, _ := url.ParseQuery(c.Query)
					v.Set("house", "ukinfo trial cursor guard 20261026")
					changed := c
					changed.Query = v.Encode()
					for side := 0; side < 2; side++ {
						p := privateRead(ctx, &client, side, changed, first[side].next)
						report.BindingChecks++
						if p.status == 400 {
							report.BindingRejected++
						}
					}
				}
			}
		}
		report.Observations = append(report.Observations, obs)
		if ctx.Err() != nil {
			break
		}
	}
	report.Complete = ctx.Err() == nil
	if err = save(output, report); err != nil {
		return err
	}
	if !report.Complete || report.Mismatches > 0 || report.Continuations == 0 || report.BindingRejected != report.BindingChecks || report.BindingChecks == 0 {
		return errors.New("cursor portability gate failed; aggregate evidence retained")
	}
	return nil
}
