package catalog

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Statistics deliberately exports only sizes, counts, manifests and timings.
// It never includes dictionary terms, source records or search parameters.
func Statistics(ctx context.Context, db *pgxpool.Pool) (json.RawMessage, error) {
	var raw json.RawMessage
	e := db.QueryRow(ctx, `SELECT json_build_object(
 'schema',2,'postgres',version(),'database_bytes',pg_database_size(current_database()),
 'terms',(SELECT count(*) FROM terms),'term_bytes',pg_total_relation_size('terms'),
 'datasets',COALESCE((SELECT json_agg(json_build_object(
 'key',d.key,'manifest',d.manifest,'entries',d.entries,'stats',d.stats,
 'projection_heap_bytes',pg_table_size(format('entries_d%s',d.id)::regclass),
 'projection_index_bytes',pg_indexes_size(format('entries_d%s',d.id)::regclass),
 'record_block_bytes',pg_total_relation_size(format('blocks_d%s',d.id)::regclass))) FROM datasets d),'[]'::json))`).Scan(&raw)
	if e != nil {
		return nil, fmt.Errorf("catalogue statistics: %w", e)
	}
	return raw, nil
}
