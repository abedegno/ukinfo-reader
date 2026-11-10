package catalog

import (
	"context"
	"github.com/abedegno/ukinfo-reader/server/internal/telemetry"
	"sync"
	"time"
)

type editionMetricKey struct{ Edition, Stage, Outcome string }
type editionDurationKey struct{ Edition, Stage string }

// Includes acquisition and row consumption/close; parallel calls overlap.
// Metrics are independent of head sampling, including when OTEL is disabled.
func (a *API) paradeEditionQuery(ctx context.Context, edition, stage string) (context.Context, func(error)) {
	edition, stage = telemetry.EditionKey(edition), telemetry.EditionStage(stage)
	ctx, end := telemetry.ParadeEdition(ctx, edition, stage)
	started := time.Now()
	var once sync.Once
	return ctx, func(err error) {
		once.Do(func() {
			elapsed := time.Since(started).Seconds()
			end(err)
			m := a.metrics
			m.mu.Lock()
			defer m.mu.Unlock()
			key := editionMetricKey{edition, stage, telemetry.QueryOutcome(err)}
			item := m.editions[key]
			item.Count++
			item.Seconds += elapsed
			for i, bound := range latencyBounds {
				if elapsed <= bound {
					item.Buckets[i]++
				}
			}
			m.editions[key] = item
		})
	}
}
