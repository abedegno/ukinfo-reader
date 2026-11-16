package catalog

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ConfigureSourcePreparation installs metadata-only preparation on the serving
// pool. Call the returned activation function after migration/schema validation,
// then Ping before accepting traffic. An independent health config must be
// copied before calling this function. No search values or rows are loaded.
// Explicit modes other than cached statements retain their existing behavior.
func ConfigureSourcePreparation(config *pgxpool.Config) func() {
	// Respect explicit protocol choices, including transaction-pooler modes.
	if config.ConnConfig.DefaultQueryExecMode != pgx.QueryExecModeCacheStatement {
		return func() {}
	}
	var ready atomic.Bool
	previous := config.PrepareConn
	if previous == nil && config.BeforeAcquire != nil {
		legacy := config.BeforeAcquire
		previous = func(ctx context.Context, c *pgx.Conn) (bool, error) { return legacy(ctx, c), nil }
	}
	where, _ := catalogueSelection("all")
	statements := []string{
		"SELECT id,key FROM datasets d WHERE " + where + " ORDER BY id",
		"SELECT id,key FROM datasets d WHERE key=$1 ORDER BY id",
		"SELECT id,key FROM terms WHERE key=$1",
		"SELECT id,key FROM terms WHERE key >= $1 AND key < $2 ORDER BY key LIMIT 8193",
		"SELECT id,key FROM terms WHERE key >= $1 AND key < $2 AND (length(key)=$3 OR key=$1) ORDER BY key LIMIT 8193",
		"SELECT id,key FROM (SELECT id,key FROM terms WHERE key=$1 UNION ALL SELECT id,key FROM terms WHERE key >= $2 AND key < $3) given_terms ORDER BY key LIMIT 8193",
	}
	config.PrepareConn = func(ctx context.Context, c *pgx.Conn) (bool, error) {
		if previous != nil {
			ok, err := previous(ctx, c)
			if !ok || err != nil {
				return ok, err
			}
		}
		if !ready.Load() {
			return true, nil
		}
		bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		for _, sql := range statements {
			// The SQL-as-name binding lets Query(full SQL) reuse this description;
			// pgx hashes the actual server name. Replacement connections prepare
			// independently under their actual role.
			if _, err := c.Prepare(bounded, sql, sql); err != nil {
				return false, fmt.Errorf("source metadata preparation: %w", err)
			}
		}
		return true, nil
	}
	return func() { ready.Store(true) }
}
