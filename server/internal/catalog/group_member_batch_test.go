package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type memberReadKey struct{}
type memberReadTracer struct {
	calls, largest, active, peak atomic.Int32
	cancel                       context.CancelFunc
	cancelAfter                  int32
}

func atomicMax(v *atomic.Int32, n int32) {
	for old := v.Load(); n > old; old = v.Load() {
		if v.CompareAndSwap(old, n) {
			return
		}
	}
}
func (tr *memberReadTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if d.SQL != groupMemberSQL {
		return ctx
	}
	n := tr.calls.Add(1)
	atomicMax(&tr.largest, int32(len(d.Args[1].([][]byte))))
	atomicMax(&tr.peak, tr.active.Add(1))
	if tr.cancel != nil && n == tr.cancelAfter {
		tr.cancel()
	}
	return context.WithValue(ctx, memberReadKey{}, true)
}
func (tr *memberReadTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if ctx.Value(memberReadKey{}) != nil {
		tr.active.Add(-1)
	}
}

// A full result page must not turn into one large HDD read per edition.
// Reintroducing unbounded reads fails the work gate; losing/duplicating a batch
// fails independently derived counts, edition labels and representatives.
func TestGroupMemberBatchIntegration(t *testing.T) {
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
	schema := fmt.Sprintf("ukinfo_member_batch_%d", os.Getpid())
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
	_, err = db.Exec(ctx, `INSERT INTO datasets(key,manifest,entries) VALUES('zeta','{}',130),('alpha','{}',65),('middle','{}',1);
 CREATE TABLE research_d1 PARTITION OF research_keys FOR VALUES IN(1);
 CREATE TABLE research_d2 PARTITION OF research_keys FOR VALUES IN(2);
 CREATE TABLE research_d3 PARTITION OF research_keys FOR VALUES IN(3);
 INSERT INTO research_keys SELECT 1,i,decode(repeat('00',32),'hex'),sha256(convert_to('invented batch '||((i+1)/2),'UTF8')),'','' FROM generate_series(1,130)i;
 INSERT INTO research_keys SELECT 2,1000+i,decode(repeat('00',32),'hex'),sha256(convert_to('invented batch '||i,'UTF8')),'','' FROM generate_series(1,65)i;
 INSERT INTO research_keys SELECT 3,1,decode(repeat('00',32),'hex'),sha256(convert_to('invented batch 65','UTF8')),'','';`)
	if err != nil {
		t.Fatal(err)
	}
	keys := [][]byte{}
	want := map[string]listingMember{}
	for i := 1; i <= 65; i++ {
		digest := sha256.Sum256([]byte(fmt.Sprintf("invented batch %d", i)))
		key := append([]byte(nil), digest[:]...)
		keys = append(keys, key)
		m := listingMember{dataset: 1, id: int32(2*i - 1), count: 3, editions: []string{"alpha", "zeta"}}
		if i == 65 {
			m.count = 4
			m.editions = []string{"alpha", "middle", "zeta"}
		}
		want[hex.EncodeToString(key)] = m
	}
	// Repeats span multiple future batches; SQL ANY originally had set semantics.
	keys = append(keys, keys[:40]...)
	for _, mode := range []string{"force_custom_plan", "force_generic_plan"} {
		t.Run(mode, func(t *testing.T) {
			tr := &memberReadTracer{}
			c := cfg.Copy()
			c.MaxConns = 8
			c.ConnConfig.Tracer = tr
			c.ConnConfig.RuntimeParams["plan_cache_mode"] = mode
			pool, err := pgxpool.NewWithConfig(ctx, c)
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			a := NewAPI(pool)
			defer a.decoder.Close()
			got, err := a.groupMembers(ctx, []int16{3, 2, 1}, keys)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatal("incomplete or duplicate member counts/editions/representatives", err)
			}
			if tr.largest.Load() > 16 || tr.peak.Load() > 2 {
				t.Errorf("unbounded source work: largest=%d active=%d; want <=16 groups/read, <=2 reads", tr.largest.Load(), tr.peak.Load())
			}
			empty, err := a.groupMembers(ctx, []int16{3, 2, 1}, nil)
			if err != nil || len(empty) != 0 {
				t.Fatal("empty group result", err)
			}
			if missing, err := a.groupMembers(ctx, []int16{3, 2, 1}, [][]byte{make([]byte, 32)}); err == nil || missing != nil {
				t.Fatal("missing group returned a partial result")
			}
			canceled, cancel := context.WithCancel(ctx)
			defer cancel()
			tr.cancel = cancel
			tr.cancelAfter = tr.calls.Load() + 2
			partial, err := a.groupMembers(canceled, []int16{3, 2, 1}, keys)
			if err == nil || partial != nil {
				t.Fatal("cancelled member lookup returned partial counts")
			}
			waitForPoolIdle(t, pool)
			if tr.active.Load() != 0 {
				t.Fatal("member reads still active")
			}
		})
	}
}
