package main

import (
	"context"
	"github.com/jackc/pgx/v5"
	"strings"
	"testing"
	"time"
)

func TestReaderPoolsKeepServingBoundsAndRejectOwnerOrForeignRouting(t *testing.T) {
	source, err := poolConfig("postgres://ukinfo_v14_verify:PRIVATE@ukinfo-db:5432/ukinfo?sslmode=disable", false)
	if err != nil || source.MaxConns != 8 || source.MinConns != 1 || source.ConnConfig.RuntimeParams["default_transaction_read_only"] != "on" || source.ConnConfig.RuntimeParams["statement_timeout"] != "3000" || source.AfterConnect == nil {
		t.Fatal("source settings changed")
	}
	target, err := poolConfig("postgres://ukinfo_parade_search:PRIVATE@ukinfo-parade-preview-db:5432/ukinfo_search?sslmode=disable", true)
	if err != nil || target.MaxConns != 4 || target.MinConns != 0 || target.ConnConfig.RuntimeParams["plan_cache_mode"] != "force_generic_plan" || target.ConnConfig.RuntimeParams["statement_timeout"] != "4000" || target.AfterConnect == nil {
		t.Fatal("target settings changed")
	}
	for _, uri := range []string{"", "postgres://postgres:PRIVATE@ukinfo-parade-preview-db/ukinfo_search", "postgres://ukinfo_parade_search:PRIVATE@outside.invalid/ukinfo_search", "postgres://ukinfo_parade_search@ukinfo-parade-preview-db/ukinfo_search"} {
		if _, err := poolConfig(uri, true); err == nil {
			t.Fatal("unexpected reader accepted")
		}
	}
}

func TestMalformedPrivateURLCannotPanicOrEchoValues(t *testing.T) {
	_, err := privateRequest(context.Background(), workloadCase{Path: "/api/search", Query: "dataset=invented&postcode=SECRET\nVALUE&sort=name"})
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatal("private URL error not sanitized")
	}
}

func TestPreparedModeAndStatementCacheCannotBeDisabledByDSN(t *testing.T) {
	cfg, err := poolConfig("postgres://ukinfo_parade_search:PRIVATE@ukinfo-parade-preview-db/ukinfo_search?sslmode=disable&default_query_exec_mode=simple_protocol&statement_cache_capacity=0", true)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.DefaultQueryExecMode != pgx.QueryExecModeCacheStatement || cfg.ConnConfig.StatementCacheCapacity < 1 {
		t.Fatal("receipt could falsely claim prepared generic execution")
	}
}
func TestEveryFallbackDestinationMustMatchApprovedIdentity(t *testing.T) {
	for _, dsn := range []string{
		"host=ukinfo-parade-preview-db,outside.invalid port=5432,5432 user=ukinfo_parade_search password=PRIVATE dbname=ukinfo_search sslmode=disable",
		"host=ukinfo-parade-preview-db,ukinfo-parade-preview-db port=5432,6432 user=ukinfo_parade_search password=PRIVATE dbname=ukinfo_search sslmode=disable",
	} {
		if _, err := poolConfig(dsn, true); err == nil {
			t.Fatal("foreign fallback accepted")
		}
	}
	if _, err := poolConfig("postgres://ukinfo_parade_search:PRIVATE@ukinfo-parade-preview-db/ukinfo_search?sslmode=prefer", true); err != nil {
		t.Fatal("same destination TLS fallback rejected")
	}
}

func TestConnectionSetupCannotExceedItsBoundFromDSN(t *testing.T) {
	cfg, err := poolConfig("postgres://ukinfo_parade_search:PRIVATE@ukinfo-parade-preview-db/ukinfo_search?sslmode=disable&connect_timeout=120", true)
	if err != nil || cfg.ConnConfig.ConnectTimeout != 5*time.Second {
		t.Fatal("unbounded setup accepted")
	}
}
