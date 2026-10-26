// Explicit hosted preview: build derived indexes in a separate owner database.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
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
	dataset := flag.String("dataset", "", "one source edition key or all visible editions")
	reader := flag.String("reader-role", "", "existing SELECT-only derived-database role to grant")
	work := flag.String("work-dir", "", "directory on the target database filesystem for free-space checks")
	floor := flag.Uint64("minimum-free-gib", 50, "minimum free space before each batch/build phase (at least 8 GiB)")
	flag.Parse()
	if *dataset == "" || *work == "" || *reader == "" || *floor < 8 || *floor > 1024 {
		return errors.New("require dataset, reader-role, work-dir and minimum-free-gib in 8..1024")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 24*time.Hour)
	defer cancel()
	open := func(name string, source bool) (*pgxpool.Pool, error) {
		value := os.Getenv(name)
		if value == "" {
			return nil, errors.New("explicit SOURCE_DATABASE_URL and TARGET_DATABASE_URL are required")
		}
		c, e := pgxpool.ParseConfig(value)
		if e != nil {
			return nil, errors.New("invalid database configuration")
		}
		c.MaxConns = 2
		c.ConnConfig.ConnectTimeout = 5 * time.Second
		c.ConnConfig.RuntimeParams["application_name"] = "ukinfo-parade-index"
		c.ConnConfig.RuntimeParams["statement_timeout"] = "3600000"
		c.ConnConfig.RuntimeParams["lock_timeout"] = "5000"
		if source {
			c.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
		} else {
			c.ConnConfig.RuntimeParams["maintenance_work_mem"] = "256MB"
			c.ConnConfig.RuntimeParams["max_parallel_maintenance_workers"] = "0"
			c.ConnConfig.RuntimeParams["temp_file_limit"] = "16GB"
		}
		return pgxpool.NewWithConfig(ctx, c)
	}
	source, err := open("SOURCE_DATABASE_URL", true)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := open("TARGET_DATABASE_URL", false)
	if err != nil {
		return err
	}
	defer target.Close()
	last := time.Time{}
	n, err := catalog.BuildParadeIndex(ctx, source, target, *dataset, catalog.ParadeBuildOptions{ReaderRole: *reader, CheckResources: func() error {
		free, e := desktop.AvailableBytes(*work)
		if e != nil {
			return e
		}
		if free < *floor<<30 {
			return catalog.ErrDiskHeadroom
		}
		return nil
	}, Progress: func(key string, done, total int64) {
		if done == total || time.Since(last) > 15*time.Second {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"dataset": key, "done": done, "total": total})
			last = time.Now()
		}
	}})
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"entries": n, "ready": true})
}
