package catalog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type sourcePrepareTraceKey struct{}
type sourceConsumerPrepareKey struct{}
type sourcePrepareTrace struct {
	mu               sync.Mutex
	executions       map[string]bool
	prepared         atomic.Int32
	consumerPrepares atomic.Int32
	cancel           context.CancelFunc
}

func (tr *sourcePrepareTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if strings.Contains(d.SQL, "FROM datasets") || strings.Contains(d.SQL, "FROM terms") || strings.Contains(d.SQL, "FROM record_blocks") {
		tr.mu.Lock()
		tr.executions[d.SQL] = true
		tr.mu.Unlock()
	}
	return ctx
}
func (*sourcePrepareTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
func (tr *sourcePrepareTrace) TracePrepareStart(ctx context.Context, _ *pgx.Conn, d pgx.TracePrepareStartData) context.Context {
	if strings.Contains(d.SQL, "FROM datasets") || strings.Contains(d.SQL, "FROM terms") {
		ctx = context.WithValue(ctx, sourceConsumerPrepareKey{}, true)
	}
	if strings.HasPrefix(d.Name, "ukinfo_source_metadata_") || d.Name == d.SQL {
		if tr.cancel != nil {
			tr.cancel()
		}
		return context.WithValue(ctx, sourcePrepareTraceKey{}, true)
	}
	return ctx
}
func (tr *sourcePrepareTrace) TracePrepareEnd(ctx context.Context, _ *pgx.Conn, d pgx.TracePrepareEndData) {
	if ctx.Value(sourceConsumerPrepareKey{}) != nil && d.Err == nil && !d.AlreadyPrepared {
		tr.consumerPrepares.Add(1)
	}
	if ctx.Value(sourcePrepareTraceKey{}) != nil && d.Err == nil && !d.AlreadyPrepared {
		tr.prepared.Add(1)
	}
}

var sourcePrepareFixtureSerial atomic.Int32

func sourcePrepareFixture(t *testing.T) (*pgxpool.Config, *sourcePrepareTrace) {
	t.Helper()
	dsn := os.Getenv("UKINFO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("ukinfo_source_prep_%d_%d", os.Getpid(), sourcePrepareFixtureSerial.Add(1))
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); admin.Close(ctx) })
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1
	cfg.MinConns = 0
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	trace := &sourcePrepareTrace{executions: map[string]bool{}}
	cfg.ConnConfig.Tracer = trace
	return cfg, trace
}

// Removing initialization leaves real metadata statements unprepared. Executing
// a dummy search instead fails the zero-source-read gate. Preparing too early
// prevents migration of an empty desktop archive.
func TestSourcePreparationMigrationAndCoverageIntegration(t *testing.T) {
	cfg, tr := sourcePrepareFixture(t)
	ctx := context.Background()
	var prior, legacy atomic.Int32
	cfg.PrepareConn = func(ctx context.Context, c *pgx.Conn) (bool, error) {
		prior.Add(1)
		_, err := c.Exec(ctx, "SET application_name='source-preparation-test'")
		return err == nil, err
	}
	cfg.BeforeAcquire = func(context.Context, *pgx.Conn) bool { legacy.Add(1); return true }
	healthCfg := cfg.Copy()
	activate := ConfigureSourcePreparation(cfg)
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = Migrate(ctx, db); err != nil {
		t.Fatal("first-launch migration failed", err)
	}
	if _, err = Import(ctx, db, strings.NewReader(fixture("source-preparation", true))); err != nil {
		t.Fatal(err)
	}
	if tr.prepared.Load() != 0 {
		t.Fatal("metadata preparation ran before schema readiness")
	}
	var version int
	if err = db.QueryRow(ctx, "SELECT max(version) FROM catalog_schema").Scan(&version); err != nil || version != SchemaVersion {
		t.Fatal("source schema", err)
	}
	tr.mu.Lock()
	tr.executions = map[string]bool{}
	tr.mu.Unlock()
	activate()
	activate()
	conn, err := db.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := conn.Query(ctx, "SELECT statement FROM pg_prepared_statements WHERE name NOT LIKE 'stmtcache_%'")
	if err != nil {
		t.Fatal(err)
	}
	prepared := map[string]bool{}
	for rows.Next() {
		var sql string
		if err = rows.Scan(&sql); err != nil {
			t.Fatal(err)
		}
		prepared[sql] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	var app string
	if err = conn.QueryRow(ctx, "SHOW application_name").Scan(&app); err != nil || app != "source-preparation-test" {
		t.Fatal("previous connection hook lost", err)
	}
	conn.Release()
	tr.mu.Lock()
	reads := len(tr.executions)
	tr.mu.Unlock()
	if reads != 0 {
		t.Fatal("initialization executed source searches", reads)
	}
	if len(prepared) < 6 || tr.prepared.Load() != 6 {
		t.Fatal("standard metadata was not prepared", len(prepared), tr.prepared.Load())
	}
	count := tr.prepared.Load()
	conn, err = db.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	conn.Release()
	if tr.prepared.Load() != count {
		t.Fatal("reused connection reparsed initialized statements")
	}
	a := NewAPI(db)
	defer a.decoder.Close()
	beforeConsumers := tr.consumerPrepares.Load()
	for _, input := range []string{
		"dataset=all&kind=person&surname=Invented&match=exact",
		"dataset=source-preparation&kind=person&surname=Inv*&match=prefix",
		"dataset=source-preparation&kind=person&postcode=AA1",
		"dataset=source-preparation&kind=person&surname=Invented&first_name=J&first_match=given",
	} {
		q, err := parse(mustValues(input))
		if err != nil {
			t.Fatal(err)
		}
		if _, status, err := a.buildCandidateFiltersWith(ctx, q, false); err != nil || status != 200 {
			t.Fatal("metadata consumer", status, err)
		}
	}
	if tr.consumerPrepares.Load() != beforeConsumers {
		t.Error("actual consumers reparsed initialized catalogue/vocabulary statements", tr.consumerPrepares.Load()-beforeConsumers)
	}
	tr.mu.Lock()
	executed := make([]string, 0, len(tr.executions))
	for sql := range tr.executions {
		executed = append(executed, sql)
	}
	tr.mu.Unlock()
	if len(executed) < 6 {
		t.Fatal("fixture did not cover all metadata query shapes", len(executed))
	}
	for _, sql := range executed {
		if !prepared[sql] {
			t.Error("actual catalogue/vocabulary consumer was not initialized")
		}
	}
	// New connections must initialize too, without requiring an application restart.
	conn, err = db.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pid := conn.Conn().PgConn().PID()
	conn.Conn().Close(ctx)
	conn.Release()
	conn, err = db.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if conn.Conn().PgConn().PID() == pid {
		t.Fatal("replacement connection not created")
	}
	conn.Release()
	if tr.prepared.Load() != count+6 {
		t.Fatal("replacement source connection was not initialized")
	}
	// Health configuration is captured before installing the source-only hook.
	health, err := pgxpool.NewWithConfig(ctx, healthCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer health.Close()
	before := tr.prepared.Load()
	if err = health.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if tr.prepared.Load() != before || prior.Load() == 0 || legacy.Load() != 0 {
		t.Fatal("health isolation or hook precedence changed")
	}
}

func TestSourcePreparationFailsClosedIntegration(t *testing.T) {
	cfg, tr := sourcePrepareFixture(t)
	ctx := context.Background()
	activate := ConfigureSourcePreparation(cfg)
	activate()
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	conn, err := db.Acquire(ctx)
	if conn != nil {
		conn.Release()
		t.Fatal("uninitialized source schema accepted")
	}
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "42P01" {
		t.Fatal("source schema preparation failure lost", err)
	}
	waitForPoolIdle(t, db)
	if tr.prepared.Load() != 0 {
		t.Fatal("unready schema initialized")
	}
}
func TestSourcePreparationPreservesHookFailureIntegration(t *testing.T) {
	cfg, tr := sourcePrepareFixture(t)
	ctx := context.Background()
	sentinel := errors.New("previous hook rejected connection")
	cfg.PrepareConn = func(context.Context, *pgx.Conn) (bool, error) { return false, sentinel }
	activate := ConfigureSourcePreparation(cfg)
	activate()
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	conn, err := db.Acquire(ctx)
	if conn != nil {
		conn.Release()
	}
	if !errors.Is(err, sentinel) || tr.prepared.Load() != 0 {
		t.Fatal("previous hook failure bypassed", err)
	}
}
func TestSourcePreparationLegacyAndCancellationIntegration(t *testing.T) {
	cfg, tr := sourcePrepareFixture(t)
	ctx := context.Background()
	var legacy atomic.Int32
	cfg.BeforeAcquire = func(context.Context, *pgx.Conn) bool { legacy.Add(1); return true }
	activate := ConfigureSourcePreparation(cfg)
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	defer cancel()
	tr.cancel = cancel
	activate()
	conn, err := db.Acquire(canceled)
	if conn != nil {
		conn.Release()
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatal("preparation ignored request cancellation", err)
	}
	waitForPoolIdle(t, db)
	tr.cancel = nil
	conn, err = db.Acquire(ctx)
	if err != nil {
		t.Fatal("pool did not recover", err)
	}
	conn.Release()
	if legacy.Load() == 0 || tr.prepared.Load() != 6 {
		t.Fatal("legacy hook or replacement initialization lost")
	}
}

// Explicit no-named-statement modes can be used with transaction poolers. Do
// not replace the caller's protocol choice with persistent prepared statements.
func TestSourcePreparationRespectsQueryModeIntegration(t *testing.T) {
	for _, mode := range []pgx.QueryExecMode{pgx.QueryExecModeCacheDescribe, pgx.QueryExecModeDescribeExec, pgx.QueryExecModeExec, pgx.QueryExecModeSimpleProtocol} {
		t.Run(mode.String(), func(t *testing.T) {
			cfg, tr := sourcePrepareFixture(t)
			cfg.ConnConfig.DefaultQueryExecMode = mode
			var calls atomic.Int32
			cfg.PrepareConn = func(context.Context, *pgx.Conn) (bool, error) { calls.Add(1); return true, nil }
			activate := ConfigureSourcePreparation(cfg)
			activate()
			db, err := pgxpool.NewWithConfig(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			conn, err := db.Acquire(context.Background())
			if err != nil {
				t.Fatal("explicit query mode attempted metadata preparation", err)
			}
			conn.Release()
			if tr.prepared.Load() != 0 || calls.Load() == 0 {
				t.Fatal("query mode or previous hook was replaced")
			}
		})
	}
}
