package catalog

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSourceCandidateCustomPlanIsTransactionLocal(t *testing.T) {
	dsn := os.Getenv("UKINFO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 1
	config.ConnConfig.RuntimeParams["plan_cache_mode"] = "force_generic_plan"
	config.ConnConfig.RuntimeParams["statement_timeout"] = "3000"
	ctx := context.Background()
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	api := NewAPI(db)
	defer api.decoder.Close()

	const statement = `SELECT 1::smallint,2::integer,3::integer,4::integer,5::integer
		WHERE current_setting('plan_cache_mode')='force_custom_plan'
		AND current_setting('transaction_read_only')='on'
		AND current_setting('statement_timeout')='3s'`
	checkSession := func() {
		t.Helper()
		var mode string
		if err := db.QueryRow(ctx, "SHOW plan_cache_mode").Scan(&mode); err != nil || mode != "force_generic_plan" {
			t.Fatalf("pooled plan mode: %q, %v", mode, err)
		}
	}
	custom, err := api.sourceCandidatePositions(ctx, statement, nil, false, true)
	if err != nil || len(custom) != 1 || custom[0].ID != 2 {
		t.Fatalf("custom plan did not run under the original statement budget and read-only transaction: %v, %v", custom, err)
	}
	checkSession()
	ordinary, err := api.sourceCandidatePositions(ctx, statement, nil, false, false)
	if err != nil || len(ordinary) != 0 {
		t.Fatalf("ordinary statement used the custom plan mode: %v, %v", ordinary, err)
	}
	if _, err := api.sourceCandidatePositions(ctx, "SELECT 1/0", nil, false, true); err == nil {
		t.Fatal("expected statement error")
	}
	checkSession()
}
