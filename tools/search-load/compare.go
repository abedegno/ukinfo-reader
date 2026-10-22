package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"
)

type pairedObservation struct {
	Page        int         `json:"page"`
	Case        int         `json:"case"`
	Round       int         `json:"round"`
	BeforeFirst bool        `json:"before_first"`
	Before      observation `json:"before"`
	After       observation `json:"after"`
	Outcome     string      `json:"outcome"`
}

// Alternate which server goes first, without overlapping database work. Cache
// and background I/O still vary; pairing reduces confounding but is not isolation.
func (e *engine) compare(before string, rounds, pages int, path string, stdout io.Writer) error {
	after := e.base
	before = strings.TrimRight(before, "/")
	b, a := &recorder{}, &recorder{}
	pairs := []pairedObservation{}
	outcomes := map[string]int{}
	start := time.Now()
	completedCases := 0
	for round := 1; round <= rounds; round++ {
		for i := range e.cases {
			if e.ctx.Err() != nil || e.used.Load()+2 > e.max {
				break
			}
			original := e.cases[i]
			cursors := [2]string{}
			caseComplete := false
			for page := 1; page <= pages; page++ {
				if e.ctx.Err() != nil || e.used.Load()+2 > e.max {
					break
				}
				p := pairedObservation{Page: page, Case: i, Round: round, BeforeFirst: (i+round+page-1)%2 == 1}
				request := func(base string, side int) observation {
					e.base = base
					e.cases[i] = original
					if page > 1 {
						v, _ := url.ParseQuery(original.Query)
						v.Set("cursor", cursors[side])
						e.cases[i].Query = v.Encode()
						e.cases[i].SHA256 = ""
					}
					o, _ := e.request(i, round, time.Now())
					cursors[side] = o.next
					return o
				}
				if p.BeforeFirst {
					p.Before = request(before, 0)
					p.After = request(after, 1)
				} else {
					p.After = request(after, 1)
					p.Before = request(before, 0)
				}
				if p.Before.Outcome == "" || p.After.Outcome == "" {
					if p.Before.Outcome != "" {
						b.add(p.Before)
					}
					if p.After.Outcome != "" {
						a.add(p.After)
					}
					p.Outcome = "incomplete_pair"
					outcomes[p.Outcome]++
					pairs = append(pairs, p)
					break
				}
				b.add(p.Before)
				a.add(p.After)
				switch {
				case !p.Before.failed() && !p.After.failed():
					p.Outcome = "both_ok_same"
					if p.Before.digest != p.After.digest {
						p.Outcome = "both_ok_mismatch"
					}
				case p.Before.failed() && !p.After.failed():
					p.Outcome = "after_only_ok"
				case !p.Before.failed() && p.After.failed():
					p.Outcome = "before_only_ok"
				default:
					p.Outcome = "both_failed"
				}
				outcomes[p.Outcome]++
				pairs = append(pairs, p)
				if len(pairs)%25 == 0 {
					fmt.Fprintf(stdout, "pairs=%d after_errors=%d mismatches=%d\n", len(pairs), outcomes["before_only_ok"]+outcomes["both_failed"], outcomes["both_ok_mismatch"])
				}
				if p.Outcome != "both_ok_same" || p.Before.next == "" || p.After.next == "" || page == pages {
					caseComplete = true
					break
				}
			}
			e.cases[i] = original
			if caseComplete {
				completedCases++
			}
		}
	}
	wall := time.Since(start)
	exhausted := e.ctx.Err() != nil || completedCases != len(e.cases)*rounds || outcomes["incomplete_pair"] > 0
	bs, as := b.report(wall), a.report(wall)
	passed := !exhausted && as.Errors == 0 && outcomes["both_ok_mismatch"] == 0
	report := map[string]any{"schema_version": 3, "mode": "paired_api", "cases": len(e.cases), "rounds": rounds, "max_pages": pages, "pairs": pairs, "outcomes": outcomes, "before": bs, "after": as, "max_requests": e.max, "request_timeout_seconds": e.client.Timeout.Seconds(), "requests_attempted": e.used.Load(), "budget_exhausted": exhausted, "passed": passed, "notes": []string{"Baseline and candidate run sequentially per case; first server alternates by case/round.", "Cache state and maintenance can still vary; inspect before-only and after-only failures, not just aggregate latency.", "Both successful responses are compared using the case digest mode; opaque cursors can be neutralized explicitly.", "No server URLs, query values, response records or hashes are exported."}}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return errors.New("cannot encode paired report")
	}
	if err = os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		return errors.New("cannot write paired report")
	}
	fmt.Fprintf(stdout, "pairs=%d after_errors=%d mismatches=%d budget_exhausted=%t\n", len(pairs), as.Errors, outcomes["both_ok_mismatch"], exhausted)
	if !passed {
		return errors.New("paired workload failed; inspect aggregate report")
	}
	return nil
}
