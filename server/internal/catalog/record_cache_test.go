package catalog

import (
	"bytes"
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRecordCacheBudgetAndIsolation(t *testing.T) {
	ctx := withRecordCache(context.Background())
	c := recordCache(ctx)
	for i := int32(0); i < 200; i++ {
		c.put(recordBlockKey{1, i}, bytes.Repeat([]byte{1}, 65536))
	}
	if c.bytes > requestBlockCacheBytes || len(c.blocks) > 128 || len(c.queue) > 128 {
		t.Fatal("cache unbounded")
	}
	if c.get(recordBlockKey{1, 0}) != nil || c.get(recordBlockKey{1, 199}) == nil {
		t.Fatal("wrong eviction")
	}
	if c.get(recordBlockKey{2, 199}) != nil {
		t.Fatal("cross-edition block collision")
	}
	if recordCache(withRecordCache(context.Background())).get(recordBlockKey{1, 199}) != nil {
		t.Fatal("shared visitor cache")
	}
	c.put(recordBlockKey{1, 201}, make([]byte, requestBlockCacheBytes+1))
	if c.get(recordBlockKey{1, 201}) != nil {
		t.Fatal("oversized block cached")
	}
	small := recordCache(withRecordCache(context.Background()))
	for i := int32(0); i < 200; i++ {
		small.put(recordBlockKey{1, i}, []byte{1})
	}
	if len(small.blocks) != 128 || small.get(recordBlockKey{1, 0}) != nil {
		t.Fatal("block-count bound failed")
	}
	c.putPlan("one", candidateFilters{args: []any{1}, conditions: []string{"original"}, termCTEs: []string{"original"}})
	f, ok := c.getPlan("one")
	if !ok {
		t.Fatal("plan missing")
	}
	f.args[0] = 2
	f.conditions[0] = "changed"
	f.termCTEs[0] = "changed"
	copy, _ := c.getPlan("one")
	if copy.args[0] != 1 || copy.conditions[0] != "original" || copy.termCTEs[0] != "original" {
		t.Fatal("cached plan mutated")
	}
	if _, ok := c.getPlan("two"); ok {
		t.Fatal("different filters reused a plan")
	}
}

type blockReadTracer struct {
	reads   atomic.Int32
	queries atomic.Int32
}

func (t *blockReadTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	t.queries.Add(1)
	if strings.Contains(d.SQL, "SELECT id,payload FROM record_blocks") {
		t.reads.Add(1)
	}
	return ctx
}
func (t *blockReadTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func testRequestBlockReuse(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	trace := &blockReadTracer{}
	cfg := db.Config()
	cfg.ConnConfig.Tracer = trace
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	a := NewAPI(pool)
	var dataset int16
	if err = pool.QueryRow(ctx, "SELECT id FROM datasets WHERE key='selective-one'").Scan(&dataset); err != nil {
		t.Fatal(err)
	}
	cached := withRecordCache(ctx)
	for _, id := range []int32{1, 2, 1} {
		if _, err = a.loadRecords(cached, dataset, []int32{id}); err != nil {
			t.Fatal(err)
		}
	}
	if trace.reads.Load() != 1 {
		t.Fatal("reread shared source block", trace.reads.Load())
	}
	if _, err = a.loadRecords(withRecordCache(ctx), dataset, []int32{1}); err != nil {
		t.Fatal(err)
	}
	if trace.reads.Load() != 2 {
		t.Fatal("request cache leaked between requests")
	}
	q, err := parse(mustValues("dataset=selective-one&surname=Smallmatch&town=Invented+Town"))
	if err != nil {
		t.Fatal(err)
	}
	planned := withRecordCache(ctx)
	if _, _, err = a.candidateFilters(planned, q); err != nil {
		t.Fatal(err)
	}
	before := trace.queries.Load()
	q.Cursor = "next-page"
	q.Limit = 1
	f, _, err := a.candidateFilters(planned, q)
	if err != nil || f.q.Cursor != "next-page" || f.q.Limit != 1 || trace.queries.Load() != before {
		t.Fatal("refinement batch repeated planning or lost cursor", err)
	}
}
