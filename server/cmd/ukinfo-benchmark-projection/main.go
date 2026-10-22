// Private, operator-run benchmark; never runs from the public HTTP service.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/abedegno/ukinfo-reader/server/internal/catalog"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"sync"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	input := flag.String("cases", "", "private cases JSON")
	output := flag.String("output", "", "aggregate report")
	dataset := flag.String("dataset", "", "full edition key")
	rounds := flag.Int("rounds", 2, "rounds (1-4)")
	pages := flag.Int("pages", 3, "pages (1-10)")
	businessOnly := flag.Bool("business-only", false, "compare the private classification-only business projection")
	workers := flag.Int("concurrency", 1, "bounded concurrent workers (1-8)")
	duration := flag.Duration("max-duration", 10*time.Minute, "whole-run budget (max 30m)")
	flag.Parse()
	if *input == "" || *output == "" || *dataset == "" || *rounds < 1 || *rounds > 4 || *pages < 1 || *pages > 10 || *workers < 1 || *workers > 8 || *duration <= 0 || *duration > 30*time.Minute {
		return fmt.Errorf("invalid benchmark configuration")
	}
	data, err := os.ReadFile(*input)
	if err != nil {
		return fmt.Errorf("cannot read private workload")
	}
	var cases []catalog.ProjectionCase
	if json.Unmarshal(data, &cases) != nil || len(cases) < 1 || len(cases) > 100 {
		return fmt.Errorf("invalid private workload")
	}
	if *workers > len(cases) {
		return fmt.Errorf("concurrency exceeds private case count")
	}
	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	if err != nil {
		return fmt.Errorf("invalid database configuration")
	}
	cfg.MaxConns = 8
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "4000"
	cfg.ConnConfig.RuntimeParams["work_mem"] = "32MB"
	cfg.ConnConfig.RuntimeParams["search_path"] = "public"
	cfg.ConnConfig.RuntimeParams["application_name"] = "ukinfo-projection-benchmark"
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("cannot initialize benchmark pool")
	}
	defer db.Close()
	started := time.Now()
	results := make([]catalog.ProjectionReport, *workers)
	errors := make([]error, *workers)
	var wg sync.WaitGroup
	for worker := 0; worker < *workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			subset := []catalog.ProjectionCase{}
			for i := worker; i < len(cases); i += *workers {
				subset = append(subset, cases[i])
			}
			benchmark := catalog.BenchmarkProjection
			if *businessOnly {
				benchmark = catalog.BenchmarkBusinessProjection
			}
			results[worker], errors[worker] = benchmark(ctx, db, subset, *dataset, *rounds, *pages)
			for i := range results[worker].Observations {
				results[worker].Observations[i].Case = results[worker].Observations[i].Case**workers + worker
			}
		}(worker)
	}
	wg.Wait()
	report := catalog.ProjectionReport{Complete: true, Observations: []catalog.ProjectionObservation{}}
	setupFailures := 0
	for i, r := range results {
		if errors[i] != nil {
			setupFailures++
			report.Complete = false
			report.Failures++
		}
		report.Complete = report.Complete && r.Complete
		report.Failures += r.Failures
		report.Mismatches += r.Mismatches
		report.Observations = append(report.Observations, r.Observations...)
	}
	envelope := struct {
		SchemaVersion int     `json:"schema_version"`
		BusinessOnly  bool    `json:"business_only"`
		Concurrency   int     `json:"concurrency"`
		WallSeconds   float64 `json:"wall_seconds"`
		SetupFailures int     `json:"setup_failures"`
		catalog.ProjectionReport
	}{1, *businessOnly, *workers, time.Since(started).Seconds(), setupFailures, report}
	raw, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(*output, append(raw, '\n'), 0600); err != nil {
		return fmt.Errorf("cannot save private report")
	}
	fmt.Printf("observations=%d failures=%d mismatches=%d complete=%t\n", len(report.Observations), report.Failures, report.Mismatches, report.Complete)
	if !report.Complete || report.Failures > 0 || report.Mismatches > 0 {
		return fmt.Errorf("projection benchmark failed; inspect aggregate report")
	}
	return nil
}
