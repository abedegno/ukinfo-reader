// Private bounded experiment; never a serving backend or publication gate.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "postcode comparison incomplete; inspect private aggregate")
		os.Exit(1)
	}
}

type execution struct {
	capture func(context.Context, []workloadCase) ([]statement, error)
	measure func(context.Context, []statement) (aggregate, error)
	cover   func(context.Context, []statement) (aggregate, error)
}

func run(parent context.Context, args []string) error {
	return runWith(parent, args, execution{capture: captureStatements, measure: measure, cover: measureCovering})
}
func runWith(parent context.Context, args []string, exec execution) (failure error) {
	f := flag.NewFlagSet("postcode-spike", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	mode := f.String("mode", "prefilter", "prefilter or covering diagnostic")
	input := f.String("cases", "", "private two-case subset")
	output := f.String("output", "", "new private aggregate")
	if f.Parse(args) != nil || (*mode != "prefilter" && *mode != "covering") || f.NArg() != 0 || !filepath.IsAbs(*input) || !filepath.IsAbs(*output) || filepath.Clean(*input) == filepath.Clean(*output) {
		return errors.New("explicit private paths required")
	}
	cases, err := readCases(*input)
	if err != nil {
		return err
	}
	receipt, err := createReport(*output)
	if err != nil {
		return err
	}
	previousLog := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(previousLog)
	report := newAggregate()
	if *mode == "covering" {
		report.Mode = *mode
	}
	report.Stage = "capture"
	defer func() {
		if err := finishReport(receipt, report); err != nil {
			failure = err
		}
		if failure == nil {
			fmt.Printf("diagnostic_requests=%d errors=%d complete=%t sql_rows_equal=%t\n", report.Requests, report.Errors, report.Complete, report.SQLRowsEqual)
		}
	}()
	ctx, cancel := context.WithTimeout(parent, 60*time.Second)
	defer cancel()
	statements, err := exec.capture(ctx, cases)
	report.Captures = len(statements)
	if err != nil {
		var boundary *captureFailure
		if errors.As(err, &boundary) {
			report.CaptureFailure = boundary
		}
		return errors.New("exact candidate capture incomplete")
	}
	measureFn := exec.measure
	if *mode == "covering" {
		measureFn = exec.cover
	}
	if measureFn == nil {
		return errors.New("measurement mode unavailable")
	}
	measured, comparisonErr := measureFn(ctx, statements)
	report = measured
	if *mode == "covering" {
		report.Mode = *mode
	}
	report.Stage = "compare"
	report.Captures = len(statements)
	if report.Complete {
		report.Stage = "complete"
	}
	if comparisonErr != nil {
		return errors.New("bounded comparison incomplete")
	}
	return nil
}
