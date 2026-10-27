package catalog

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/errgroup"
)

func parallelNamePrefix(q query) bool {
	return q.Kind == "person" && q.Sort == "" && q.Group == "" && q.First != "" && q.Surname != "" &&
		(q.Match == "prefix" || strings.HasSuffix(q.Surname, "*")) &&
		q.Postcode == "" && q.Near == "" && !q.hasAddress() && !q.hasBusinessFilters()
}

func scanCandidatePositions(rows pgx.Rows) ([]cursor, error) {
	defer rows.Close()
	out := []cursor{}
	for rows.Next() {
		var c cursor
		if err := rows.Scan(&c.Dataset, &c.ID, &c.Name, &c.First, &c.Postcode); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (a *API) sourceCandidatePositions(ctx context.Context, sql string, args []any, addressBudget bool) ([]cursor, error) {
	rows, err := a.searchRows(ctx, "candidates", sql, args, addressBudget)
	if err != nil {
		return nil, err
	}
	return scanCandidatePositions(rows)
}

// Two edition probes overlap cold index I/O in the same eight-connection pool.
// Every edition contributes a full page/lookahead; their exact numeric merge is
// equivalent to the existing UNION ALL. Any failed probe cancels its sibling
// and discards every partial candidate, under unchanged SQL/HTTP deadlines.
func (a *API) sourceNamePrefixPage(ctx context.Context, args []any, conditions []string, datasets []int16, limit int) (out []cursor, err error) {
	ctx, end := a.searchPhase(ctx, "candidates")
	defer func() { end(err) }()
	group, ctx := errgroup.WithContext(ctx)
	group.SetLimit(2)
	var mu sync.Mutex
	for _, dataset := range datasets {
		group.Go(func() error {
			queryArgs := append(slices.Clone(args), limit, dataset)
			sql := fmt.Sprintf("SELECT dataset_id,id,name_id,first_id,postcode_id FROM entries WHERE %s AND dataset_id=$%d ORDER BY name_id,first_id,id,dataset_id LIMIT $%d", strings.Join(conditions, " AND "), len(queryArgs), len(queryArgs)-1)
			rows, err := a.DB.Query(ctx, sql, queryArgs...)
			if err != nil {
				return err
			}
			batch, err := scanCandidatePositions(rows)
			if err != nil {
				return err
			}
			mu.Lock()
			out = append(out, batch...)
			mu.Unlock()
			return nil
		})
	}
	if err = group.Wait(); err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b cursor) int {
		for _, pair := range [][2]int32{{a.Name, b.Name}, {a.First, b.First}, {a.ID, b.ID}, {int32(a.Dataset), int32(b.Dataset)}} {
			if c := cmp.Compare(pair[0], pair[1]); c != 0 {
				return c
			}
		}
		return 0
	})
	return out[:min(limit, len(out))], nil
}
