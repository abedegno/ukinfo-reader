// Private NAS trial preparation and aggregate-only checks. No public URL replay.
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
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/abedegno/ukinfo-reader/server/internal/catalog"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	side := flag.String("side", "", "private snapshot backend: native or candidate")
	reverse := flag.Bool("reverse", false, "reverse the fixed snapshot case order")
	ordinal := flag.Int("case-ordinal", -1, "one snapshot case ordinal; -1 selects the full cohort")
	aggregateReport := flag.String("report", "", "aggregate snapshot report without private digests")
	beforeSnapshot := flag.String("before-snapshot", "", "private native snapshot")
	afterSnapshot := flag.String("after-snapshot", "", "private candidate snapshot")
	planCases := flag.String("plan-cases", "0,1,5,8,16,40,80,120,170,220,280", "up to 12 private case ordinals for diagnostics")
	seeds := flag.String("seed-cases", "", "existing private regression workload for merge")
	dataset := flag.String("dataset", "all", "visible edition scope for a pilot verification or subset")
	stage := flag.String("stage", "verify", "cases, merge, subset, metadata, verify, plans, calibrate or cursors")
	output := flag.String("output", "", "new private output file")
	cases := flag.String("cases", "", "private workload for calibration")
	load := flag.String("load-cases", "", "new private matched-reference workload")
	samples := flag.Int("samples", 256, "spread verification samples per edition, 2..1024")
	flag.Parse()
	if *output == "" {
		return errors.New("output is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if *stage == "hosted-check" {
		return checkHosted(ctx, *cases, *output)
	}
	if *stage == "snapshot" {
		return captureSnapshot(ctx, *cases, *output, *aggregateReport, *side, *reverse, *ordinal)
	}
	if *stage == "compare-snapshots" {
		return compareSnapshots(*beforeSnapshot, *afterSnapshot, *output)
	}
	if *stage == "merge" {
		return merge(*cases, *seeds, *output)
	}
	if *stage == "subset" {
		return subset(*cases, *dataset, *output)
	}
	if *stage == "metadata" {
		return metadata(*cases, *output)
	}
	if *stage == "cursors" {
		return checkCursors(ctx, *cases, *output)
	}
	if *stage == "calibrate" {
		return calibrate(ctx, *cases, *load, *output)
	}
	open := func(name string) (*pgxpool.Pool, error) {
		if os.Getenv(name) == "" {
			return nil, errors.New("explicit private database configuration is required")
		}
		cfg, e := pgxpool.ParseConfig(os.Getenv(name))
		if e != nil {
			return nil, errors.New("invalid database configuration")
		}
		cfg.MaxConns = 2
		cfg.ConnConfig.ConnectTimeout = 5 * time.Second
		cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
		cfg.ConnConfig.RuntimeParams["statement_timeout"] = "120000"
		cfg.ConnConfig.RuntimeParams["application_name"] = "ukinfo-parade-trial"
		return pgxpool.NewWithConfig(ctx, cfg)
	}
	source, err := open("SOURCE_DATABASE_URL")
	if err != nil {
		return err
	}
	defer source.Close()
	if *stage == "cases" {
		values, e := catalog.CreateParadeTrialCases(ctx, source)
		if e != nil {
			return e
		}
		if e = save(*output, values); e != nil {
			return e
		}
		fmt.Printf("private_cases=%d\n", len(values))
		return nil
	}
	if *stage != "verify" && *stage != "plans" {
		return errors.New("unsupported trial stage")
	}
	target, err := open("TARGET_DATABASE_URL")
	if err != nil {
		return err
	}
	defer target.Close()
	if *stage == "plans" {
		raw, e := os.ReadFile(*cases)
		if e != nil {
			return errors.New("private cases unavailable")
		}
		var cohort []catalog.ParadeTrialCase
		if json.Unmarshal(raw, &cohort) != nil {
			return errors.New("invalid private cases")
		}
		indices := []int{}
		for _, part := range strings.Split(*planCases, ",") {
			n, e := strconv.Atoi(part)
			if e != nil {
				return errors.New("invalid diagnostic ordinal")
			}
			indices = append(indices, n)
		}
		report, e := catalog.CaptureParadeTrialPlans(ctx, source, target, cohort, indices)
		if e != nil {
			return e
		}
		return save(*output, report)
	}
	var report catalog.ParadeTrialVerification
	if *dataset == "all" {
		report, err = catalog.VerifyParadeTrial(ctx, source, target, *samples)
	} else {
		report, err = catalog.VerifyParadeEditionTrial(ctx, source, target, *dataset, *samples)
	}
	if e := save(*output, report); e != nil {
		return e
	}
	if err != nil {
		return err
	}
	fmt.Printf("editions=%d passed=%t\n", len(report.Editions), report.Passed)
	return nil
}
func save(path string, value any) error {
	b, e := json.MarshalIndent(value, "", "  ")
	if e != nil {
		return errors.New("cannot encode private output")
	}
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return errors.New("choose a new writable output path")
	}
	_, e = f.Write(append(b, '\n'))
	closeErr := f.Close()
	if e != nil || closeErr != nil {
		return errors.New("cannot save output")
	}
	return nil
}

type loadCase struct {
	catalog.ParadeTrialCase
	SHA256 string `json:"sha256"`
}
type calibrationObservation struct {
	Case            int    `json:"case"`
	NativeStatus    int    `json:"native_status"`
	CandidateStatus int    `json:"candidate_status"`
	Outcome         string `json:"outcome"`
}
type calibrationReport struct {
	Complete        bool                     `json:"complete"`
	Cases           int                      `json:"cases"`
	Matching        int                      `json:"matching_cases"`
	Mismatches      int                      `json:"mismatches"`
	NativeErrors    int                      `json:"native_errors"`
	CandidateErrors int                      `json:"candidate_errors"`
	Observations    []calibrationObservation `json:"observations"`
}

func digest(body []byte, mode string) (string, error) {
	if mode == "cursor-neutral-v1" {
		var fields map[string]json.RawMessage
		if json.Unmarshal(body, &fields) != nil || fields == nil {
			return "", errors.New("invalid response")
		}
		var next string
		if value, ok := fields["next_cursor"]; ok {
			if json.Unmarshal(value, &next) != nil {
				return "", errors.New("invalid response cursor")
			}
		}
		fields["next_cursor"] = json.RawMessage("false")
		if next != "" {
			fields["next_cursor"] = json.RawMessage("true")
		}
		var err error
		body, err = json.Marshal(fields)
		if err != nil {
			return "", errors.New("invalid response")
		}
	}
	if mode == "" {
		for _, line := range bytes.Split(body, []byte{'\n'}) {
			line = bytes.TrimSpace(line)
			if len(line) > 0 && !json.Valid(line) {
				return "", errors.New("invalid export response")
			}
		}
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}
func calibrate(ctx context.Context, path, load, output string) error {
	if path == "" || load == "" {
		return errors.New("calibration requires cases and load-cases")
	}
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
	// These unexposed Docker names are intentional. Do not accept Internet URLs,
	// proxies or redirects for saved real-data queries.
	bases := privateBases
	fetch := func(base string, c catalog.ParadeTrialCase) (int, string) {
		req, e := http.NewRequestWithContext(ctx, http.MethodGet, base+c.Path+"?"+c.Query, nil)
		if e != nil {
			return 0, ""
		}
		r, e := client.Do(req)
		if e != nil {
			return 0, ""
		}
		defer r.Body.Close()
		body, e := io.ReadAll(io.LimitReader(r.Body, (8<<20)+1))
		if e != nil || len(body) > 8<<20 || r.StatusCode != 200 {
			return r.StatusCode, ""
		}
		hash, e := digest(body, c.DigestMode)
		if e != nil {
			return r.StatusCode, ""
		}
		return r.StatusCode, hash
	}
	report := calibrationReport{Cases: len(cases), Observations: []calibrationObservation{}}
	selected := []loadCase{}
	for i, c := range cases {
		switch c.Path {
		case "/api/search", "/api/record", "/api/address", "/api/group", "/api/export":
		default:
			return errors.New("unsupported private case path")
		}
		var status [2]int
		var hashes [2]string
		for k := 0; k < 2; k++ {
			side := (i + k) % 2
			status[side], hashes[side] = fetch(bases[side], c)
		}
		obs := calibrationObservation{Case: i, NativeStatus: status[0], CandidateStatus: status[1], Outcome: "excluded_error"}
		if hashes[0] == "" {
			report.NativeErrors++
		}
		if hashes[1] == "" {
			report.CandidateErrors++
		}
		if hashes[0] != "" && hashes[1] != "" {
			if hashes[0] == hashes[1] {
				report.Matching++
				obs.Outcome = "match"
				selected = append(selected, loadCase{c, hashes[0]})
			} else {
				report.Mismatches++
				obs.Outcome = "mismatch"
			}
		}
		report.Observations = append(report.Observations, obs)
		if ctx.Err() != nil {
			break
		}
	}
	report.Complete = len(report.Observations) == len(cases) && ctx.Err() == nil
	if err = save(output, report); err != nil {
		return err
	}
	if !report.Complete || report.Mismatches > 0 {
		return errors.New("calibration failed; aggregate evidence retained")
	}
	if len(selected) == 0 {
		return errors.New("no independently matching load cases")
	}
	if err = save(load, selected); err != nil {
		return err
	}
	fmt.Printf("cases=%d matched=%d native_errors=%d candidate_errors=%d mismatches=%d\n", report.Cases, report.Matching, report.NativeErrors, report.CandidateErrors, report.Mismatches)
	return nil
}
