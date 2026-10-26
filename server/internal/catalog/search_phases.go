package catalog

import (
	"context"
	"sync"
	"time"

	"github.com/abedegno/ukinfo-reader/server/internal/telemetry"
	"github.com/jackc/pgx/v5"
)

type phaseKey struct{ Name, Outcome string }

func searchPhaseName(name string) string {
	switch name {
	case "parade_inventory", "parade_candidates", "parade_pair", "vocabulary", "source_probe", "address_probe", "address_blocks", "address_dictionary", "candidates", "ordered", "group_members", "records":
		return name
	default:
		return "other"
	}
}
func (a *API) searchPhase(ctx context.Context, name string) (context.Context, func(error)) {
	name = searchPhaseName(name)
	ctx, span := telemetry.Operation(ctx, "search."+name)
	started := time.Now()
	return ctx, func(err error) {
		telemetry.End(span, err)
		a.metrics.observePhase(name, time.Since(started).Seconds(), err != nil)
	}
}
func (m *apiMetrics) observePhase(name string, elapsed float64, failed bool) {
	name = searchPhaseName(name)
	outcome := "ok"
	if failed {
		outcome = "error"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := phaseKey{name, outcome}
	item := m.phases[key]
	item.Count++
	item.Seconds += elapsed
	for i, bound := range latencyBounds {
		if elapsed <= bound {
			item.Buckets[i]++
		}
	}
	m.phases[key] = item
}

// phaseRows ends the operation after rows are consumed or closed, rather than
// when Query returns its first packet. Closing also releases any private TX.
// Next closes on exhaustion so pool resources and metrics cannot be left pending.
type phaseRows struct {
	pgx.Rows
	once   sync.Once
	finish func()
	end    func(error)
}

func (r *phaseRows) Close() { r.once.Do(func() { r.Rows.Close(); r.finish(); r.end(r.Rows.Err()) }) }
func (r *phaseRows) Next() bool {
	if r.Rows.Next() {
		return true
	}
	r.Close()
	return false
}
func (a *API) searchRows(ctx context.Context, phase, sql string, args []any, addressBudget bool) (pgx.Rows, error) {
	ctx, end := a.searchPhase(ctx, phase)
	finish := func() {}
	var rows pgx.Rows
	var err error
	if phase == "ordered" {
		rows, finish, err = a.orderedRows(ctx, sql, args)
	} else if addressBudget {
		rows, finish, err = a.addressRows(ctx, sql, args)
	} else {
		rows, err = a.DB.Query(ctx, sql, args...)
	}
	if err != nil {
		finish()
		end(err)
		return nil, err
	}
	return &phaseRows{Rows: rows, finish: finish, end: end}, nil
}
