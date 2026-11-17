package catalog

import (
	"context"
	"time"
)

// SourceReadTimeout gives cold HDD reads three seconds while retaining separate
// health, target and refinement budgets. Pool sizes and concurrency are unchanged.
const SourceReadTimeout = 3 * time.Second

func searchRequestContext(parent context.Context, q query) (context.Context, context.CancelFunc) {
	timeout := SourceReadTimeout
	if q.Near != "" || q.hasAddress() || q.hasRefinements() || q.needsResearch() || q.Sort != "" {
		timeout = 5 * time.Second
	}
	return context.WithTimeout(parent, timeout)
}
