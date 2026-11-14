package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pageBlockTracer struct {
	enabled           atomic.Bool
	blocks            atomic.Int32
	active            atomic.Int32
	maximum           atomic.Int32
	largest           atomic.Int32
	cancel            context.CancelFunc
	cancelAfter       int32
	successful        atomic.Int32
	completedAtCancel atomic.Int32
}

func (p *pageBlockTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if p.enabled.Load() && strings.Contains(d.SQL, "FROM record_blocks") {
		p.blocks.Add(1)
		n := p.active.Add(1)
		for m := p.maximum.Load(); n > m; m = p.maximum.Load() {
			if p.maximum.CompareAndSwap(m, n) {
				break
			}
		}
		branches := int32(strings.Count(d.SQL, "FROM record_blocks"))
		for m := p.largest.Load(); branches > m; m = p.largest.Load() {
			if p.largest.CompareAndSwap(m, branches) {
				break
			}
		}
		ctx = context.WithValue(ctx, pageBlockTraceKey{}, true)
		if p.cancel != nil && (p.cancelAfter == 0 || p.blocks.Load() >= p.cancelAfter) {
			p.completedAtCancel.Store(p.successful.Load())
			p.cancel()
		}
	}
	return ctx
}

type pageBlockTraceKey struct{}

func (p *pageBlockTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryEndData) {
	if ctx.Value(pageBlockTraceKey{}) == true {
		p.active.Add(-1)
		if d.Err == nil {
			p.successful.Add(1)
		}
	}
}

func TestCrossEditionRecordBatchIntegration(t *testing.T) {
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
	schema := fmt.Sprintf("ukinfo_record_batch_%d", os.Getpid())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	trace := &pageBlockTracer{}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.ConnConfig.Tracer = trace
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err = Import(ctx, db, strings.NewReader(strings.ReplaceAll(strings.ReplaceAll(fixture(fmt.Sprintf("batch-%d", i), true), "not-a-real-source", strings.Repeat(fmt.Sprint(i+1), 64)), `"entry_id":`, fmt.Sprintf(`"edition_marker":%d,"entry_id":`, i)))); err != nil {
			t.Fatal(err)
		}
	}
	api := NewAPI(db)
	defer api.decoder.Close()
	// Each edition has IDs 1,2,5 in block zero. Equal IDs across editions must
	// never overwrite each other; the numeric candidate merge interleaves them.
	base := "/api/search?dataset=all&kind=person&postcode=AA1&limit=10"
	// Independent retained single-edition reader supplies exact original bytes
	// and source/archive identities, without using the new batch loader.
	expected := map[string]map[int64]result{}
	requested := map[int16][]int32{}
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("batch-%d", i)
		var dataset int16
		if err = db.QueryRow(ctx, "SELECT id FROM datasets WHERE key=$1", key).Scan(&dataset); err != nil {
			t.Fatal(err)
		}
		requested[dataset] = []int32{1, 2, 5, 1}
		records, e := api.loadRecords(ctx, dataset, []int32{1, 2, 5})
		if e != nil {
			t.Fatal(e)
		}
		expected[key] = map[int64]result{}
		for _, r := range records {
			expected[key][r.ID] = r
		}
	}
	trace.enabled.Store(true)
	w := httptest.NewRecorder()
	api.Handler().ServeHTTP(w, httptest.NewRequest("GET", base, nil))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var page searchResult
	if err = json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 10 || page.Next == "" {
		t.Fatal("missing first page/lookahead")
	}
	for i, r := range page.Items {
		wantID := int64(1 + i/5)
		wantDataset := fmt.Sprintf("batch-%d", i%5)
		var raw map[string]any
		if err = json.Unmarshal(r.Record, &raw); err != nil {
			t.Fatal(err)
		}
		if r.ID != wantID || r.Dataset != wantDataset || r.SourceID != []string{"a", "b"}[i/5] || raw["surname"] != []string{"Li", "LI"}[i/5] || r.Archive == "" {
			t.Fatalf("wrong record at %d: %+v", i, r)
		}
	}
	for _, r := range page.Items {
		want := expected[r.Dataset][r.ID]
		if r.Archive != want.Archive || r.SourceID != want.SourceID || !bytes.Equal(r.Record, want.Record) {
			t.Fatal("batch changed original record or archive identity")
		}
	}
	if n := trace.blocks.Load(); n != 2 {
		t.Fatalf("serial cross-edition block retrieval: got %d statements, want 2", n)
	}
	trace.blocks.Store(0)
	w = httptest.NewRecorder()
	api.Handler().ServeHTTP(w, httptest.NewRequest("GET", base+"&cursor="+url.QueryEscape(page.Next), nil))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if err = json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 5 || page.Next != "" {
		t.Fatal("wrong continuation")
	}
	for i, r := range page.Items {
		if r.ID != 5 || r.Dataset != fmt.Sprintf("batch-%d", i) || r.SourceID != "e" {
			t.Fatal("wrong continuation identity")
		}
	}
	cached := withRecordCache(ctx)
	trace.blocks.Store(0)
	first, e := api.loadPageRecords(cached, requested)
	if e != nil || len(first) != 15 {
		t.Fatal("duplicate ID handling", len(first), e)
	}
	count := trace.blocks.Load()
	second, e := api.loadPageRecords(cached, requested)
	if e != nil || len(second) != 15 || trace.blocks.Load() != count {
		t.Fatal("request cache reread blocks", e)
	}
	if _, e = api.loadPageRecords(ctx, map[int16][]int32{-1: {1}}); e == nil {
		t.Fatal("missing manifest accepted")
	}
	for dataset := range requested {
		if _, e = api.loadPageRecords(ctx, map[int16][]int32{dataset: {0}}); e == nil {
			t.Fatal("invalid ID accepted")
		}
		break
	}
	if trace.largest.Load() > 4 || trace.maximum.Load() > 2 || trace.active.Load() != 0 {
		t.Fatal("read bounds exceeded", trace.largest.Load(), trace.maximum.Load(), trace.active.Load())
	}
	// More than two batches exercises the global scheduler, with records
	// spanning ten distinct blocks in one edition plus five other editions.
	var input bytes.Buffer
	enc := json.NewEncoder(&input)
	enc.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: "batch-many", Edition: "invented-many", Expected: 1280, SHA256: map[string]string{"invented": strings.Repeat("a", 64)}})
	for i := 0; i < 1280; i++ {
		enc.Encode(map[string]any{"type": "record", "source_id": fmt.Sprint(i), "record": map[string]any{"surname": "Manyblocks", "firstName": "June", "postcode": "ZZ1 1ZZ", "marker": i}})
	}
	enc.Encode(map[string]any{"type": "end", "count": 1280})
	trace.enabled.Store(false)
	if _, err = Import(ctx, db, &input); err != nil {
		t.Fatal(err)
	}
	var many int16
	if err = db.QueryRow(ctx, "SELECT id FROM datasets WHERE key='batch-many'").Scan(&many); err != nil {
		t.Fatal(err)
	}
	wide := map[int16][]int32{}
	for d := range requested {
		wide[d] = []int32{1}
	}
	for i := int32(0); i < 10; i++ {
		wide[many] = append(wide[many], i*128+1)
	}
	trace.enabled.Store(true)
	trace.blocks.Store(0)
	trace.successful.Store(0)
	all, e := api.loadPageRecords(ctx, wide)
	if e != nil || len(all) != 15 || trace.blocks.Load() != 4 {
		t.Fatal("wide page batch", len(all), trace.blocks.Load(), e)
	}
	for i := int32(0); i < 10; i++ {
		var raw map[string]any
		json.Unmarshal(all[sourceRecordKey{many, i*128 + 1}].Record, &raw)
		if raw["marker"] != float64(i*128) {
			t.Fatal("wrong wide block")
		}
	}
	if trace.largest.Load() != 4 || trace.maximum.Load() > 2 || trace.active.Load() != 0 {
		t.Fatal("wide read bounds exceeded", trace.largest.Load(), trace.maximum.Load(), trace.active.Load())
	}
	late, lateCancel := context.WithCancel(ctx)
	trace.cancel = lateCancel
	trace.cancelAfter = 3
	trace.blocks.Store(0)
	trace.successful.Store(0)
	got, e := api.loadPageRecords(late, wide)
	lateCancel()
	trace.cancel = nil
	trace.cancelAfter = 0
	if e == nil || got != nil || trace.completedAtCancel.Load() < 1 {
		t.Fatal("late cancellation exposed prefix or did not exercise completed batch", e)
	}
	// Cancellation after dispatch must never expose an otherwise valid prefix.
	canceled, cancel := context.WithCancel(ctx)
	defer cancel()
	trace.cancel = cancel
	w = httptest.NewRecorder()
	api.Handler().ServeHTTP(w, httptest.NewRequest("GET", base, nil).WithContext(canceled))
	trace.cancel = nil
	if w.Code != 503 || strings.Contains(w.Body.String(), `"items"`) {
		t.Fatal("cancellation exposed partial page", w.Code, w.Body.String())
	}
	trace.enabled.Store(false)
	var original []byte
	var broken int16
	if err = db.QueryRow(ctx, "SELECT dataset_id,payload FROM record_blocks WHERE dataset_id=(SELECT id FROM datasets WHERE key='batch-4') AND id=0").Scan(&broken, &original); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "UPDATE record_blocks SET payload='bad'::bytea WHERE dataset_id=$1 AND id=0", broken); err != nil {
		t.Fatal(err)
	}
	if got, e := api.loadPageRecords(ctx, requested); !errors.Is(e, errRecordIntegrity) || got != nil {
		t.Fatal("corrupt block exposed partial data", e)
	}
	if _, err = db.Exec(ctx, "UPDATE record_blocks SET payload=$1 WHERE dataset_id=$2 AND id=0", original, broken); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "DELETE FROM record_blocks WHERE dataset_id=(SELECT id FROM datasets WHERE key='batch-4')"); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	api.Handler().ServeHTTP(w, httptest.NewRequest("GET", base, nil))
	if w.Code != 503 || strings.Contains(w.Body.String(), `"items"`) {
		t.Fatal("missing block exposed partial page", w.Code, w.Body.String())
	}
}
