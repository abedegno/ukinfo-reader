// Operator-only full-edition search experiment. No public routes are added.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/abedegno/ukinfo-reader/server/internal/catalog"
	"github.com/abedegno/ukinfo-reader/server/internal/desktop"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	stage := flag.String("stage", "prepare", "prepare, cases, measure, plans or explain")
	input := flag.String("cases", "", "private workload JSON")
	output := flag.String("output", "", "aggregate report path")
	rounds := flag.Int("rounds", 2, "rounds per case (1..4)")
	pages := flag.Int("pages", 3, "pages (1..5)")
	clients := flag.Int("clients", 1, "workers (1..8)")
	mode := flag.String("plan-mode", "auto", "auto or force_custom_plan")
	reference := flag.Bool("reference", false, "capture private independent source references")
	flag.Parse()
	if *mode != "auto" && *mode != "force_custom_plan" {
		return fmt.Errorf("invalid plan mode")
	}
	budget := 4 * time.Hour
	if *stage != "prepare" {
		budget = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	if os.Getenv("SOURCE_DATABASE_URL") == "" || os.Getenv("TARGET_DATABASE_URL") == "" {
		return fmt.Errorf("explicit source and target database configurations are required")
	}
	sc, err := pgxpool.ParseConfig(os.Getenv("SOURCE_DATABASE_URL"))
	if err != nil {
		return fmt.Errorf("invalid source configuration")
	}
	sc.MaxConns = 8
	sc.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	sc.ConnConfig.RuntimeParams["statement_timeout"] = "120000"
	sc.ConnConfig.RuntimeParams["application_name"] = "ukinfo-real-search-spike"
	dc, err := pgxpool.ParseConfig(os.Getenv("TARGET_DATABASE_URL"))
	if err != nil {
		return fmt.Errorf("invalid target configuration")
	}
	dc.MaxConns = 8
	dc.ConnConfig.RuntimeParams["application_name"] = "ukinfo-real-search-spike"
	dc.ConnConfig.RuntimeParams["statement_timeout"] = "2400000"
	dc.ConnConfig.RuntimeParams["lock_timeout"] = "5000"
	dc.ConnConfig.RuntimeParams["work_mem"] = "32MB"
	dc.ConnConfig.RuntimeParams["maintenance_work_mem"] = "256MB"
	dc.ConnConfig.RuntimeParams["max_parallel_maintenance_workers"] = "0"
	if *stage == "prepare" {
		dc.ConnConfig.RuntimeParams["temp_file_limit"] = "16GB"
	}

	if *stage != "prepare" {
		sc.ConnConfig.RuntimeParams["statement_timeout"] = "4000"
		dc.ConnConfig.RuntimeParams["statement_timeout"] = "4000"
		dc.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
		dc.ConnConfig.RuntimeParams["plan_cache_mode"] = *mode
	}
	src, err := pgxpool.NewWithConfig(ctx, sc)
	if err != nil {
		return fmt.Errorf("source pool unavailable")
	}
	defer src.Close()
	dst, err := pgxpool.NewWithConfig(ctx, dc)
	if err != nil {
		return fmt.Errorf("target pool unavailable")
	}
	defer dst.Close()
	if *stage == "prepare" {
		last := time.Time{}
		return catalog.BuildSearchEngineSpike(ctx, src, dst, "se2003-dpcs0104", func() error {
			b, e := desktop.AvailableBytes("/work")
			if e != nil {
				return e
			}
			if b < 50<<30 {
				return catalog.ErrDiskHeadroom
			}
			return nil
		}, func(phase string, done, total int64) {
			if done == 0 || time.Since(last) > 15*time.Second || done == total {
				raw, _ := json.Marshal(map[string]any{"phase": phase, "done": done, "total": total})
				fmt.Println(string(raw))
				last = time.Now()
			}
		})
	}
	if *input == "" || *output == "" {
		return fmt.Errorf("private cases and aggregate output are required")
	}
	raw, err := os.ReadFile(*input)
	if err != nil {
		return fmt.Errorf("private workload unavailable")
	}
	var cases []catalog.ProjectionCase
	if json.Unmarshal(raw, &cases) != nil {
		return fmt.Errorf("invalid private workload")
	}
	save := func(v any) error {
		raw, e := json.MarshalIndent(v, "", "  ")
		if e != nil {
			return e
		}
		return os.WriteFile(*output, append(raw, '\n'), 0600)
	}
	if *stage == "cases" {
		out, e := catalog.CreateSearchEngineCases(ctx, dst, cases)
		if e != nil {
			return e
		}
		if e = save(out); e != nil {
			return e
		}
		fmt.Printf("private_cases=%d\n", len(out))
		return nil
	}
	if *stage == "plans" || *stage == "explain" {
		plans, e := catalog.CaptureSearchEnginePlans(ctx, dst, cases, *stage == "plans")
		if e != nil {
			return e
		}
		return save(plans)
	}
	if *stage != "measure" {
		return fmt.Errorf("unsupported experiment stage")
	}
	report, err := catalog.BenchmarkSearchEngines(ctx, src, dst, cases, *rounds, *pages, *clients, *mode, *reference)
	if err != nil {
		return err
	}
	if err = save(report); err != nil {
		return err
	}
	fmt.Printf("observations=%d failures=%d mismatches=%d complete=%t\n", len(report.Observations), report.Failures, report.Mismatches, report.Complete)
	if !report.Complete || report.Failures > 0 || report.Mismatches > 0 {
		return fmt.Errorf("benchmark recorded failures; aggregate report retained")
	}
	return nil
}
