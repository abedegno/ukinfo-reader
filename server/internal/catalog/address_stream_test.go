package catalog

import (
	"bytes"
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNativeTownStreamIntegration(t *testing.T) {
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
	schema := fmt.Sprintf("ukinfo_town_stream_%d", os.Getpid())
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
	testNativeTownStream(t, db)
}

type cancelAddressBlockTrace struct {
	cancel context.CancelFunc
	fired  bool
}

func (tr *cancelAddressBlockTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, "FROM address_record_blocks") {
		tr.fired = true
		tr.cancel()
	}
	return ctx
}
func (*cancelAddressBlockTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func testNativeTownCancellation(t *testing.T, db *pgxpool.Pool, query string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tracer := &cancelAddressBlockTrace{cancel: cancel}
	cfg := db.Config()
	cfg.MaxConns = 2
	cfg.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	api := NewAPI(pool)
	defer api.decoder.Close()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/search?"+query, nil).WithContext(ctx)
	api.Handler().ServeHTTP(w, r)
	if !tracer.fired || w.Code != 503 {
		t.Fatal("cancellation did not interrupt reverse lookup", tracer.fired, w.Code)
	}
	if pool.Stat().AcquiredConns() != 0 {
		t.Fatal("canceled reverse lookup leaked its source cursor")
	}
}

func testNativeTownSmallPools(t *testing.T, db *pgxpool.Pool, query string, expected []byte) {
	t.Helper()
	cfg := db.Config()
	cfg.MaxConns = 1
	one, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer one.Close()
	api := NewAPI(one)
	defer api.decoder.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	w := httptest.NewRecorder()
	api.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?"+query, nil).WithContext(ctx))
	cancel()
	if w.Code != 200 || !bytes.Equal(expected, w.Body.Bytes()) {
		t.Fatal("single-connection address search cannot progress or changed the page", w.Code)
	}
	if one.Stat().AcquiredConns() != 0 {
		t.Fatal("single-connection search leaked its cursor")
	}
	cfg = db.Config()
	cfg.MaxConns = 2
	shared, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer shared.Close()
	apis := []*API{NewAPI(shared), NewAPI(shared)}
	defer apis[0].decoder.Close()
	defer apis[1].decoder.Close()
	var group sync.WaitGroup
	failures := make(chan int, 4)
	for i := 0; i < 4; i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			w := httptest.NewRecorder()
			apis[i%2].Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?"+query, nil).WithContext(ctx))
			if w.Code != 200 || !bytes.Equal(expected, w.Body.Bytes()) {
				failures <- w.Code
			}
		}(i)
	}
	group.Wait()
	close(failures)
	for code := range failures {
		t.Fatal("shared small pool stalled or changed a page", code)
	}
	if shared.Stat().AcquiredConns() != 0 {
		t.Fatal("concurrent searches leaked source cursors")
	}
}
