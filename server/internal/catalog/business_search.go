package catalog

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed business_search.sql
var businessSearchSchema string

// IndexBusinessSearch derives a complete edition from existing verified indexes.
// SQL publication is atomic; ordinary VACUUM follows commit for index-only reads.
func IndexBusinessSearch(ctx context.Context, db *pgxpool.Pool, key string, check func() error) (n int64, err error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	if check != nil {
		if err = check(); err != nil {
			return 0, err
		}
		var cancelCause context.CancelCauseFunc
		ctx, cancelCause = context.WithCancelCause(ctx)
		done := make(chan struct{})
		go func() {
			defer close(done)
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if e := check(); e != nil {
						cancelCause(e)
						return
					}
				}
			}
		}()
		defer func() {
			cancelCause(nil)
			<-done
			if err != nil && context.Cause(ctx) != context.Canceled {
				err = context.Cause(ctx)
			}
		}()
	}
	tx, e := db.Begin(ctx)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback(context.Background())
	if _, e = tx.Exec(ctx, `SET LOCAL lock_timeout='5s'; SET LOCAL statement_timeout='20min'; SET LOCAL work_mem='64MB'; SET LOCAL maintenance_work_mem='256MB'; SET LOCAL max_parallel_workers_per_gather=0; SET LOCAL max_parallel_maintenance_workers=0; SET LOCAL temp_file_limit='4GB'; SET LOCAL jit=off`); e != nil {
		return 0, e
	}
	if e = tx.QueryRow(ctx, "SELECT build_business_search($1)", key).Scan(&n); e != nil {
		return 0, e
	}
	if e = tx.Commit(ctx); e != nil {
		return 0, e
	}
	var dataset int16
	if err := db.QueryRow(ctx, "SELECT id FROM datasets WHERE key=$1", key).Scan(&dataset); err != nil {
		return n, err
	}
	_, err = db.Exec(ctx, "VACUUM(ANALYZE) "+pgx.Identifier{fmt.Sprintf("business_d%d", dataset)}.Sanitize())
	return n, err
}

func (a *API) businessSearchReady(ctx context.Context, q query, datasets []int16) (bool, error) {
	if a.DisableBusinessSearch || q.Kind != "business" || !q.hasBusinessFilters() || q.Group != "" || q.Sort != "" || q.Near != "" || len(datasets) == 0 {
		return false, nil
	}
	var available bool
	if err := a.DB.QueryRow(ctx, `SELECT COALESCE(has_table_privilege(current_user,to_regclass('business_search'),'SELECT') AND has_table_privilege(current_user,to_regclass('business_search_indexes'),'SELECT'),false)`).Scan(&available); err != nil || !available {
		return false, err
	}
	var ready int
	err := a.DB.QueryRow(ctx, `SELECT count(*) FROM business_search_indexes i JOIN datasets d ON d.id=i.dataset_id AND d.entries=i.entries JOIN pg_inherits p ON p.inhparent='business_search'::regclass AND p.inhrelid=to_regclass(format('%I.%I',current_schema(),'business_d'||i.dataset_id)) WHERE i.dataset_id=ANY($1::smallint[]) AND i.version=1`, datasets).Scan(&ready)
	return ready == len(datasets), err
}

func businessClassificationPredicate(q query, args *[]any, alias string) string {
	parts := []string{}
	for _, f := range []struct{ column, value string }{{"sic", q.SIC}, {"industry", q.Industry}} {
		if f.value == "" {
			continue
		}
		*args = append(*args, strings.TrimSuffix(f.value, "*"))
		predicate := fmt.Sprintf("%s.%s=$%d", alias, f.column, len(*args))
		if strings.HasSuffix(f.value, "*") {
			*args = append(*args, strings.TrimSuffix(f.value, "*")+"\U0010ffff")
			predicate = fmt.Sprintf("%s.%s >= $%d AND %s.%s < $%d", alias, f.column, len(*args)-1, alias, f.column, len(*args))
		}
		parts = append(parts, alias+"."+f.column+"<>''", predicate)
	}
	return strings.Join(parts, " AND ")
}

func businessSearchBranch(f candidateFilters, dataset int16, datasetArg, limitArg int, conditions []string, args *[]any, ctes *[]string, order string) string {
	name := fmt.Sprintf("business_eligible_%d", dataset)
	predicate := strings.Join(conditions, " AND ") + fmt.Sprintf(" AND dataset_id=$%d", datasetArg)
	*ctes = append(*ctes, fmt.Sprintf("%s AS MATERIALIZED (SELECT dataset_id,id,name_id,first_id,postcode_id,location_id FROM business_search entries WHERE %s)", name, predicate))
	source := name
	if f.q.hasAddress() {
		locations := fmt.Sprintf("business_locations_%d", dataset)
		*ctes = append(*ctes, locations+" AS MATERIALIZED (SELECT l.id FROM address_locations l WHERE "+addressPredicate(f.q, args)+")")
		source = fmt.Sprintf("(SELECT e.* FROM %s e JOIN %s l ON l.id=e.location_id) eligible", name, locations)
	}
	return fmt.Sprintf("(SELECT dataset_id,id,name_id,first_id,postcode_id FROM %s ORDER BY %s LIMIT $%d)", source, order, limitArg)
}
