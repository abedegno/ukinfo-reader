package catalog

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestEditionMetricsWithoutRecordingTrace(t *testing.T) {
	config, _ := pgxpool.ParseConfig("postgres://invented@127.0.0.1:1/invented?sslmode=disable")
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	a := NewAPI(pool)
	_, end := a.paradeEditionQuery(context.Background(), "PRIVATE_EDITION", "PRIVATE_STAGE")
	end(context.DeadlineExceeded)
	end(errors.New("PRIVATE_ERROR"))
	_, endOther := a.paradeEditionQuery(context.Background(), "PRIVATE_SECOND_EDITION", "PRIVATE_STAGE")
	endOther(nil)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, end := a.paradeEditionQuery(context.Background(), "se2003-dpcs0104", "positions")
			end(nil)
		}()
	}
	wg.Wait()
	w := httptest.NewRecorder()
	a.Metrics().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	body := w.Body.String()
	for _, want := range []string{`ukinfo_parade_edition_queries_total{edition="other",stage="other",outcome="deadline"} 1`, `ukinfo_parade_edition_queries_total{edition="se2003-dpcs0104",stage="positions",outcome="ok"} 40`, `ukinfo_parade_edition_duration_seconds_bucket{edition="other",stage="other",le="+Inf"} 2`, `ukinfo_parade_edition_duration_seconds_count{edition="other",stage="other"} 2`} {
		if !strings.Contains(body, want) {
			t.Fatal("missing bounded always-on metric", want, body)
		}
	}
	if strings.Contains(body, "PRIVATE") {
		t.Fatal("private metric label")
	}
}
func TestEditionTimerErrorPaths(t *testing.T) {
	a := NewAPI(nil)
	for _, stage := range []string{"positions", "pair_drain"} {
		_, end := a.paradeEditionQuery(context.Background(), "se2004-pcp218-2004-07", stage)
		end(fmt.Errorf("PRIVATE: %w", context.Canceled))
	}
	for _, stage := range []string{"positions", "pair_drain"} {
		if a.metrics.editions[editionMetricKey{"se2004-pcp218-2004-07", stage, "canceled"}].Count != 1 {
			t.Fatal("error timer not counted")
		}
	}
}

// Called by the existing disposable database integration's dense-pair fixture.
func testEditionQueryPaths(t *testing.T, a *API, q query, editions []paradeEdition) {
	t.Helper()
	for _, stage := range []string{"positions", "pair_drain"} {
		if a.metrics.editions[editionMetricKey{"other", stage, "ok"}].Count == 0 {
			t.Fatal("real query path did not emit edition metrics", stage)
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.paradePositions(canceled, q, editions[:1], paradePosition{}); err == nil {
		t.Fatal("canceled positions query succeeded")
	}
	err := a.drainParadePair(canceled, q, editions[:1], paradePosition{Dataset: editions[0].ID, Name: "invented", First: "invented"}, func([]paradePosition) error { return nil })
	if err == nil {
		t.Fatal("canceled pair query succeeded")
	}
	for _, stage := range []string{"positions", "pair_drain"} {
		if a.metrics.editions[editionMetricKey{"other", stage, "canceled"}].Count != 1 {
			t.Fatal("query cancellation missing or double-counted", stage)
		}
	}
}
