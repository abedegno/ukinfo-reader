package catalog

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The matching name sorts beyond the bounded dictionary probe. Broad prefixes
// must stay complete without making a selective source probe scan huge arrays.
func TestBroadTermSourceProbeIntegration(t *testing.T) {
	dsn := os.Getenv("UKINFO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	schema := fmt.Sprintf("ukinfo_broad_terms_%d", os.Getpid())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO datasets(key,manifest,entries) VALUES('broad-terms','{}',10000);
 CREATE TABLE entries_broad PARTITION OF entries FOR VALUES IN (1);
 INSERT INTO terms(key) VALUES('commonname'),('june a'),('amy');
 INSERT INTO terms(key) SELECT 'jaaa'||lpad(i::text,6,'0') FROM generate_series(1,200000) i;
 INSERT INTO entries(id,dataset_id,kind,name_id,first_id,postcode_id)
 SELECT i,1,0,(SELECT id FROM terms WHERE key='commonname'),
 (SELECT id FROM terms WHERE key=CASE WHEN i%2=1 THEN 'june a' ELSE 'amy' END),0
 FROM generate_series(1,10000) i; ANALYZE;`)
	if err != nil {
		t.Fatal(err)
	}
	a := NewAPI(db)
	defer a.decoder.Close()
	q, err := parse(mustValues("dataset=broad-terms&kind=person&match=exact&surname=Commonname&first_name=J*"))
	if err != nil {
		t.Fatal(err)
	}
	f, status, err := a.buildCandidateFiltersWith(ctx, q, false)
	if err != nil || status != 200 {
		t.Fatalf("filters: status=%d err=%v", status, err)
	}
	sql := "WITH " + strings.Join(f.termCTEs, ",") + " SELECT dataset_id,id FROM entries WHERE " + strings.Join(f.conditions, " AND ") + " LIMIT 4097"
	for _, mode := range []string{"auto", "force_custom_plan", "force_generic_plan"} {
		t.Run(mode, func(t *testing.T) {
			probeConfig := cfg.Copy()
			probeConfig.MaxConns = 1
			probeConfig.ConnConfig.RuntimeParams["plan_cache_mode"] = mode
			probeDB, err := pgxpool.NewWithConfig(ctx, probeConfig)
			if err != nil {
				t.Fatal(err)
			}
			defer probeDB.Close()
			probe := NewAPI(probeDB)
			defer probe.decoder.Close()
			bounded, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
			defer cancel()
			started := time.Now()
			candidates, count, err := probe.addressPlanCandidates(bounded, "source_probe", sql, f.args)
			if err != nil {
				t.Fatalf("selective broad-term probe exceeded its budget or failed: %v", err)
			}
			t.Logf("bounded source probe: %s", time.Since(started))
			if count != 4097 {
				t.Fatalf("prefix was truncated or source set changed: got %d", count)
			}
			for _, ids := range candidates {
				for _, id := range ids {
					if id%2 != 1 {
						t.Fatal("nonmatching first name admitted")
					}
				}
			}
		})
	}
}
