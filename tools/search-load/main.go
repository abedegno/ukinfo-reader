// Bounded HTTP workload measurements. Query values and response records stay private.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type measures struct {
	Requests int     `json:"requests"`
	Errors   int     `json:"errors"`
	Median   float64 `json:"median_ms"`
	P95      float64 `json:"p95_ms"`
	P99      float64 `json:"p99_ms"`
	Max      float64 `json:"max_ms"`
	Wall     float64 `json:"wall_seconds"`
	QPS      float64 `json:"requests_per_second"`
}

func summarize(latencies []float64, failures int, wall time.Duration) measures {
	sort.Float64s(latencies)
	n := len(latencies)
	if n == 0 {
		return measures{Errors: failures, Wall: wall.Seconds()}
	}
	qps := 0.0
	if wall > 0 {
		qps = float64(n) / wall.Seconds()
	}
	return measures{n, failures, latencies[n/2], latencies[int(math.Ceil(.95*float64(n)))-1], latencies[int(math.Ceil(.99*float64(n)))-1], latencies[n-1], wall.Seconds(), qps}
}

type queryCase struct {
	Path       string `json:"path,omitempty"`
	Query      string `json:"query"`
	SHA256     string `json:"sha256,omitempty"`
	DigestMode string `json:"digest_mode,omitempty"`
}
type observation struct {
	Case         int     `json:"case"`
	Round        int     `json:"round,omitempty"`
	Family       string  `json:"family"`
	Scope        string  `json:"scope"`
	Milliseconds float64 `json:"milliseconds"`
	Status       int     `json:"status,omitempty"`
	Outcome      string  `json:"outcome"`
	Code         string  `json:"code,omitempty"`
}

func validResponse(body []byte, contentType string) bool {
	if strings.HasPrefix(contentType, "application/x-ndjson") {
		for _, line := range bytes.Split(body, []byte{'\n'}) {
			line = bytes.TrimSpace(line)
			if len(line) > 0 && !json.Valid(line) {
				return false
			}
		}
		return true
	}
	return json.Valid(body)
}

// cursor-neutral-v1 preserves the complete response except that an opaque
// next-page token becomes a boolean. Physical dataset IDs inside cursors differ
// after an import; item identities/content and page availability must still match.
func responseDigest(body []byte, mode string) (string, error) {
	if mode == "cursor-neutral-v1" {
		var fields map[string]json.RawMessage
		if json.Unmarshal(body, &fields) != nil || fields == nil {
			return "", errors.New("invalid response object")
		}
		var cursor string
		if token, ok := fields["next_cursor"]; ok {
			if json.Unmarshal(token, &cursor) != nil {
				return "", errors.New("invalid pagination token")
			}
		}
		if cursor != "" {
			fields["next_cursor"] = json.RawMessage("true")
		} else {
			fields["next_cursor"] = json.RawMessage("false")
		}
		var err error
		body, err = json.Marshal(fields)
		if err != nil {
			return "", errors.New("invalid response object")
		}
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

func (o observation) failed() bool { return o.Outcome != "ok" && o.Outcome != "unverified" }
func classify(c queryCase) (string, string) {
	q, _ := url.ParseQuery(c.Query)
	scope := "edition"
	if q.Get("dataset") == "all" {
		scope = "all"
	}
	if c.Path != "/api/search" {
		return strings.TrimPrefix(c.Path, "/api/"), scope
	}
	switch {
	case q.Get("group") != "":
		return "grouped", scope
	case q.Get("sort") != "" && q.Get("sort") != "catalogue":
		return "ordered", scope
	case q.Get("near") != "":
		return "nearby", scope
	case q.Get("street") != "" || q.Get("town") != "" || q.Get("county") != "" || q.Get("locality") != "":
		return "address", scope
	case q.Get("postcode") != "":
		return "postcode", scope
	case q.Get("kind") == "business":
		return "business", scope
	default:
		return "name", scope
	}
}

type stageReport struct {
	measures
	Outcomes map[string]int      `json:"outcomes"`
	Statuses map[string]int      `json:"statuses"`
	Families map[string]measures `json:"families"`
	Cases    map[int]measures    `json:"per_case"`
}
type recorder struct {
	mu   sync.Mutex
	rows []observation
}

func (r *recorder) add(o observation) { r.mu.Lock(); r.rows = append(r.rows, o); r.mu.Unlock() }
func (r *recorder) report(wall time.Duration) stageReport {
	s := stageReport{Outcomes: map[string]int{}, Statuses: map[string]int{}, Families: map[string]measures{}, Cases: map[int]measures{}}
	all := []float64{}
	failed := 0
	byFamily := map[string][]float64{}
	familyErrors := map[string]int{}
	byCase := map[int][]float64{}
	caseErrors := map[int]int{}
	for _, o := range r.rows {
		all = append(all, o.Milliseconds)
		s.Outcomes[o.Outcome]++
		if o.Status > 0 {
			s.Statuses[strconv.Itoa(o.Status)]++
		}
		byFamily[o.Family] = append(byFamily[o.Family], o.Milliseconds)
		byCase[o.Case] = append(byCase[o.Case], o.Milliseconds)
		if o.failed() {
			failed++
			familyErrors[o.Family]++
			caseErrors[o.Case]++
		}
	}
	s.measures = summarize(all, failed, wall)
	for k, v := range byFamily {
		s.Families[k] = summarize(v, familyErrors[k], wall)
	}
	for k, v := range byCase {
		s.Cases[k] = summarize(v, caseErrors[k], wall)
	}
	return s
}

type engine struct {
	ctx    context.Context
	client *http.Client
	base   string
	cases  []queryCase
	used   atomic.Int64
	max    int64
}

func (e *engine) request(index, round int, at time.Time) (observation, bool) {
	for {
		n := e.used.Load()
		if n >= e.max || e.ctx.Err() != nil {
			return observation{}, false
		}
		if e.used.CompareAndSwap(n, n+1) {
			break
		}
	}
	family, scope := classify(e.cases[index])
	o := observation{Case: index, Round: round, Family: family, Scope: scope, Outcome: "transport_error"}
	req, err := http.NewRequestWithContext(e.ctx, http.MethodGet, e.base+e.cases[index].Path+"?"+e.cases[index].Query, nil)
	if err != nil {
		o.Milliseconds = float64(time.Since(at)) / float64(time.Millisecond)
		return o, true
	}
	req.Header.Set("User-Agent", "UKInfo-Workload-Benchmark/2.0")
	req.Header.Set("Accept", "application/json")
	response, err := e.client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			o.Outcome = "timeout"
		}
		o.Milliseconds = float64(time.Since(at)) / float64(time.Millisecond)
		return o, true
	}
	o.Status = response.StatusCode
	body, err := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
	response.Body.Close()
	switch {
	case err != nil:
		if errors.Is(err, context.DeadlineExceeded) {
			o.Outcome = "timeout"
		}
	case len(body) > 8<<20:
		o.Outcome = "body_too_large"
	case o.Status != 200:
		o.Outcome = "http_error"
		var b struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(body, &b)
		switch b.Code {
		case "query_timeout", "rate_limited", "invalid_request", "unavailable", "catalogue_integrity":
			o.Code = b.Code
		}
	case !validResponse(body, response.Header.Get("Content-Type")):
		o.Outcome = "invalid_json"
	case e.cases[index].SHA256 == "":
		o.Outcome = "unverified"
	default:
		digest, err := responseDigest(body, e.cases[index].DigestMode)
		if err != nil {
			o.Outcome = "invalid_json"
		} else if digest == strings.ToLower(e.cases[index].SHA256) {
			o.Outcome = "ok"
		} else {
			o.Outcome = "hash_mismatch"
		}
	}
	o.Milliseconds = float64(time.Since(at)) / float64(time.Millisecond)
	return o, true
}
func parseConcurrency(raw string) ([]int, error) {
	out := []int{}
	seen := map[int]bool{}
	for _, s := range strings.Split(raw, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || n < 1 || n > 64 || seen[n] {
			return nil, errors.New("concurrency must contain distinct integers from 1 to 64")
		}
		seen[n] = true
		out = append(out, n)
	}
	return out, nil
}
func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string, stdout io.Writer) error {
	f := flag.NewFlagSet("search-load", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	host := f.String("url", "http://127.0.0.1:8080", "base API URL")
	duration := f.Duration("duration", 10*time.Second, "duration per closed-loop stage, at most five minutes")
	rate := f.Int("rate", 100, "scheduled requests per second; zero skips this stage")
	timeout := f.Duration("request-timeout", 8*time.Second, "per-request HTTP timeout")
	arrivals := f.Duration("arrival-duration", 30*time.Second, "scheduled-arrival duration")
	casesPath := f.String("cases", "", "private cases JSON")
	output := f.String("output", "", "aggregate report JSON")
	concurrency := f.String("concurrency", "1,8,32", "closed-loop client counts")
	probe := f.Bool("probe-only", false, "initial and repeated sequential observations only; permits missing reference hashes")
	rounds := f.Int("repeats", 1, "sequential observation rounds; first round is never warmed by this runner")
	maxRequests := f.Int64("max-requests", 10000, "total request budget, including sequential observations")
	maxDuration := f.Duration("max-duration", 5*time.Minute, "whole-run time budget, including queue drain")
	if err := f.Parse(args); err != nil {
		return errors.New("invalid benchmark flags")
	}
	levels, err := parseConcurrency(*concurrency)
	if err != nil {
		return err
	}
	u, err := url.Parse(*host)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("base URL must use HTTP(S) without credentials, path, query or fragment")
	}
	if *output == "" || *duration <= 0 || *duration > 5*time.Minute || *rate < 0 || *rate > 1000 || *arrivals <= 0 || *arrivals > 5*time.Minute || *timeout <= 0 || *timeout > time.Minute || *maxDuration <= 0 || *maxDuration > 30*time.Minute || *maxRequests < 1 || *maxRequests > 100000 || *rounds < 1 || *rounds > 10 {
		return errors.New("invalid benchmark budgets")
	}
	raw, err := os.ReadFile(*casesPath)
	if err != nil {
		return errors.New("cannot read private workload")
	}
	var cases []queryCase
	if json.Unmarshal(raw, &cases) != nil || len(cases) == 0 || len(cases) > 10000 || int64(len(cases)**rounds) > *maxRequests {
		return errors.New("invalid workload or insufficient request budget for observations")
	}
	unverified := 0
	classes := map[string]int{}
	for i := range cases {
		c := &cases[i]
		if c.Path == "" {
			c.Path = "/api/search"
		}
		switch c.Path {
		case "/api/search", "/api/export", "/api/group", "/api/address", "/api/record":
		default:
			return errors.New("unsupported workload endpoint")
		}
		if _, err := url.ParseQuery(c.Query); err != nil || strings.Contains(c.Query, "#") {
			return errors.New("invalid private query encoding")
		}
		if c.DigestMode != "" && c.DigestMode != "body" && c.DigestMode != "cursor-neutral-v1" {
			return errors.New("unsupported response digest mode")
		}
		if c.SHA256 == "" && *probe {
			unverified++
		} else if decoded, e := hex.DecodeString(c.SHA256); e != nil || len(decoded) != 32 {
			return errors.New("load cases require a validated SHA-256; use probe-only for exploration")
		}
		family, _ := classify(*c)
		classes[family]++
	}
	transport := &http.Transport{MaxIdleConns: 64, MaxIdleConnsPerHost: 64, MaxConnsPerHost: 64, DisableCompression: true}
	defer transport.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), *maxDuration)
	defer cancel()
	e := &engine{ctx: ctx, client: &http.Client{Transport: transport, Timeout: *timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, base: strings.TrimRight(*host, "/"), cases: cases, max: *maxRequests}
	result := map[string]any{"schema_version": 2, "cases": len(cases), "case_classes": classes, "unverified_cases": unverified, "request_timeout_seconds": timeout.Seconds(), "max_requests": *maxRequests, "max_duration_seconds": maxDuration.Seconds(), "closed_loop": map[string]stageReport{}, "notes": []string{"Initial observations precede any warm-up by this runner. Cache state is otherwise unchanged and is not necessarily cold.", "HTTP transfer, JSON validation and hashing are timed; failed attempts are included in percentiles.", "Only reference-hashed cases may enter load stages. Probe-only unverified successes do not establish result correctness.", "Reports exclude URLs, query values, response records, hashes and raw errors.", "cursor-neutral-v1 compares full response content while replacing opaque next-page tokens with page availability; pagination traversal requires separate verification.", "Document corpus, build, hardware, quotas, import activity and storage/cache conditions alongside this report."}}
	initial := &recorder{}
	start := time.Now()
	for round := 1; round <= *rounds; round++ {
		for i := range cases {
			if o, ok := e.request(i, round, time.Now()); ok {
				initial.add(o)
			}
		}
	}
	result["initial_observations"] = initial.rows
	initialStats := initial.report(time.Since(start))
	result["initial_summary"] = initialStats
	failures := initialStats.Errors
	var sequence atomic.Uint64
	if !*probe {
		for _, clients := range levels {
			r := &recorder{}
			start := time.Now()
			deadline := start.Add(*duration)
			var wg sync.WaitGroup
			for worker := 0; worker < clients; worker++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for time.Now().Before(deadline) && ctx.Err() == nil {
						i := int(sequence.Add(1)-1) % len(cases)
						o, ok := e.request(i, 0, time.Now())
						if !ok {
							return
						}
						r.add(o)
					}
				}()
			}
			wg.Wait()
			s := r.report(time.Since(start))
			result["closed_loop"].(map[string]stageReport)[strconv.Itoa(clients)] = s
			failures += s.Errors
			fmt.Fprintf(stdout, "clients=%d requests=%d p95=%.3fms errors=%d\n", clients, s.Requests, s.P95, s.Errors)
		}
		if *rate > 0 {
			type job struct {
				index int
				at    time.Time
			}
			jobs := make(chan job, 32)
			r := &recorder{}
			var wg sync.WaitGroup
			start := time.Now()
			scheduled := int(arrivals.Seconds() * float64(*rate))
			dropped := 0
			var skipped atomic.Int64
			for worker := 0; worker < 16; worker++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for j := range jobs {
						if o, ok := e.request(j.index, 0, j.at); ok {
							r.add(o)
						} else {
							skipped.Add(1)
						}
					}
				}()
			}
		scheduling:
			for i := 0; i < scheduled; i++ {
				at := start.Add(time.Duration(float64(i) * float64(time.Second) / float64(*rate)))
				if delay := time.Until(at); delay > 0 {
					timer := time.NewTimer(delay)
					select {
					case <-timer.C:
					case <-ctx.Done():
						timer.Stop()
						skipped.Add(int64(scheduled - i))
						break scheduling
					}
				}
				select {
				case jobs <- job{i % len(cases), at}:
				default:
					dropped++
				}
			}
			close(jobs)
			wg.Wait()
			s := r.report(time.Since(start))
			result["scheduled_arrivals"] = map[string]any{"rate_per_second": *rate, "scheduled_requests": scheduled, "load_generator_drops": dropped, "budget_skips": skipped.Load(), "stats": s}
			failures += s.Errors + dropped + int(skipped.Load())
		}
	}
	exhausted := ctx.Err() != nil || e.used.Load() >= *maxRequests
	// Reaching the exact sequential budget is complete in probe-only mode.
	if *probe && len(initial.rows) == len(cases)**rounds && ctx.Err() == nil {
		exhausted = false
	}
	result["requests_attempted"] = e.used.Load()
	result["budget_exhausted"] = exhausted
	result["passed"] = failures == 0 && !exhausted
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return errors.New("cannot encode aggregate report")
	}
	if err = os.WriteFile(*output, append(data, '\n'), 0600); err != nil {
		return errors.New("cannot write aggregate report")
	}
	fmt.Fprintf(stdout, "cases=%d initial_errors=%d requests=%d budget_exhausted=%t\n", len(cases), initialStats.Errors, e.used.Load(), exhausted)
	if failures > 0 || exhausted {
		return errors.New("workload failed; inspect aggregate report")
	}
	return nil
}
