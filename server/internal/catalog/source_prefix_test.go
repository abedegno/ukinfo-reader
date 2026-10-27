package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestParallelNamePrefixSelection(t *testing.T) {
	for _, input := range []string{"surname=Test*&first_name=June", "surname=Testing&match=prefix&first_name=June&first_match=given"} {
		q, err := parse(mustValues("dataset=all&" + input))
		if err != nil || !parallelNamePrefix(q) {
			t.Fatal("eligible prefix conjunction", err)
		}
	}
	for _, input := range []string{"surname=Testing&first_name=June", "surname=Test*", "surname=Test*&first_name=June&sort=name", "surname=Test*&first_name=June&group=listing", "surname=Test*&first_name=June&postcode=AA1", "surname=Test*&first_name=June&town=Invented", "surname=Test*&first_name=June&near=AA11AA", "kind=business&organisation=Test*&first_name=June"} {
		q, err := parse(mustValues("dataset=all&" + input))
		if err == nil && parallelNamePrefix(q) {
			t.Fatal("overbroad prefix selection")
		}
	}
}

type prefixCancelTracer struct {
	once   sync.Once
	cancel context.CancelFunc
}

func (p *prefixCancelTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if strings.Contains(d.SQL, "FROM entries WHERE") {
		p.once.Do(p.cancel)
	}
	return ctx
}
func (*prefixCancelTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func testParallelNamePrefix(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	for edition := 0; edition < 3; edition++ {
		var b bytes.Buffer
		enc := json.NewEncoder(&b)
		enc.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: fmt.Sprintf("parallel-prefix-%d", edition), Edition: fmt.Sprint(edition), Title: "Invented parallel name conjunction", SHA256: map[string]string{"fixture": "invented"}, Expected: 15})
		for i := 0; i < 15; i++ {
			name := []string{"Parallelprefixone", "Parallelprefixtwo", "Other"}[i%3]
			first := []string{"June A", "June", "Junette", "Zora"}[i%4]
			enc.Encode(map[string]any{"type": "record", "source_id": fmt.Sprint(i), "record": map[string]any{"surname": name, "firstName": first, "postcode": "AA1 1AA"}})
		}
		enc.Encode(map[string]any{"type": "end", "count": 15})
		if _, err := Import(ctx, db, &b); err != nil {
			t.Fatal(err)
		}
	}
	// Independent full SQL population/order, without API paging or per-edition
	// limits. June must include its middle-initial form and exclude Junette.
	rows, err := db.Query(ctx, `SELECT d.key,e.id FROM entries e JOIN datasets d ON d.id=e.dataset_id JOIN terms n ON n.id=e.name_id JOIN terms f ON f.id=e.first_id WHERE n.key IN ('parallelprefixone','parallelprefixtwo') AND f.key IN ('june','june a') ORDER BY e.name_id,e.first_id,e.id,e.dataset_id`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{}
	for rows.Next() {
		var key string
		var id int32
		if err = rows.Scan(&key, &id); err != nil {
			t.Fatal(err)
		}
		want = append(want, fmt.Sprintf("%s:%d", key, id))
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(want) != 18 {
		t.Fatal("independent invented population", len(want), err)
	}
	api := NewAPI(db)
	input := "dataset=all&surname=Parallelprefix*&first_name=June&first_match=given&limit=2"
	got := []string{}
	for _, r := range paradeWalk(t, api, api, input) {
		got = append(got, fmt.Sprintf("%s:%d", r.Dataset, r.ID))
		if r.SourceID != fmt.Sprint(r.ID-1) {
			t.Fatal("source identity changed")
		}
	}
	if !reflect.DeepEqual(got, want) || api.metrics.plans["source_prefix_parallel"] == 0 {
		t.Fatal("parallel prefix lost order or population")
	}
	canceled, cancel := context.WithCancel(ctx)
	defer cancel()
	cfg := db.Config()
	cfg.ConnConfig.Tracer = &prefixCancelTracer{cancel: cancel}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	failed := NewAPI(pool)
	w := httptest.NewRecorder()
	failed.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?"+input, nil).WithContext(canceled))
	if w.Code != 503 || strings.Contains(w.Body.String(), `"items"`) {
		t.Fatal("failed source branch returned partial results", w.Code)
	}
	waitForPoolIdle(t, pool)
}
