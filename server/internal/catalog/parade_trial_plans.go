package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ParadeTrialPlan struct {
	Strategy    string   `json:"strategy"`
	Case        int      `json:"case"`
	Dataset     string   `json:"dataset"`
	Mode        string   `json:"mode"`
	Outcome     string   `json:"outcome"`
	ErrorCode   string   `json:"error_code,omitempty"`
	MS          float64  `json:"ms"`
	Buffers     float64  `json:"buffers"`
	TempBlocks  float64  `json:"temp_blocks"`
	HeapFetches float64  `json:"heap_fetches"`
	Nodes       []string `json:"nodes"`
}

// CaptureParadeTrialPlans keeps prepared execution diagnostics inside the private
// target and returns only allowlisted structural/work fields, never predicates.
func CaptureParadeTrialPlans(ctx context.Context, src, dst *pgxpool.Pool, cases []ParadeTrialCase, indices []int) ([]ParadeTrialPlan, error) {
	if len(indices) < 1 || len(indices) > 12 {
		return nil, errors.New("choose 1..12 diagnostic case ordinals")
	}
	out := []ParadeTrialPlan{}
	for _, ci := range indices {
		if ci < 0 || ci >= len(cases) {
			continue
		}
		c := cases[ci]
		if c.Path != "/api/search" {
			continue
		}
		v, e := url.ParseQuery(c.Query)
		if e != nil {
			return out, errors.New("invalid private case")
		}
		q, e := parse(v)
		if e != nil {
			return out, errors.New("invalid private case")
		}
		if q.Group != "" || q.Near != "" || (q.Sort != "" && q.Sort != "name") {
			continue
		}
		q.Limit++
		if q.House != "" || q.Building != "" || q.Flat != "" || q.Middle != "" {
			q.Limit = 100
		}
		editions, e := searchEditions(ctx, src, q.Dataset)
		if e != nil {
			return out, errors.New("source inventory unavailable")
		}
		for _, d := range editions {
			strategies := []bool{false}
			if q.Postcode != "" && q.PostcodeLength > 0 {
				strategies = append(strategies, true)
			}
			for _, prefilter := range strategies {
				strategy := "scalar"
				if prefilter {
					strategy = "postcode_prefilter"
				}
				sql, args := paradeSQLPrefix(q, d, paradePosition{}, prefilter)

				for _, mode := range []string{"force_custom_plan", "force_generic_plan"} {
					obs := ParadeTrialPlan{Strategy: strategy, Case: ci, Dataset: d.Key, Mode: mode, Outcome: "plan_error", Nodes: []string{}}
					conn, e := dst.Acquire(ctx)
					if e != nil {
						return out, errors.New("target connection unavailable")
					}
					tx, e := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
					if e != nil {
						conn.Release()
						return out, errors.New("target transaction unavailable")
					}
					_, e = tx.Exec(ctx, "SET LOCAL plan_cache_mode="+mode+"; SET LOCAL statement_timeout='4000'; SET LOCAL standard_conforming_strings=on")
					if e == nil {
						_, e = tx.Exec(ctx, "PREPARE private_parade_plan AS "+sql)
					}
					if e == nil {
						literals := []string{}
						for _, arg := range args {
							switch value := arg.(type) {
							case string:
								literals = append(literals, "'"+strings.ReplaceAll(value, "'", "''")+"'")
							case int, int16, int32:
								literals = append(literals, fmt.Sprint(value))
							default:
								e = errors.New("unsupported private argument")
							}
						}
						var raw []byte
						started := time.Now()
						if e == nil {
							e = tx.QueryRow(ctx, "EXPLAIN(ANALYZE,BUFFERS,FORMAT JSON) EXECUTE private_parade_plan("+strings.Join(literals, ",")+")").Scan(&raw)
						}
						obs.MS = time.Since(started).Seconds() * 1000
						if e == nil {
							var plans []map[string]any
							if json.Unmarshal(raw, &plans) != nil || len(plans) != 1 {
								e = errors.New("invalid private plan")
							} else {
								root, _ := plans[0]["Plan"].(map[string]any)
								num := func(m map[string]any, k string) float64 { n, _ := m[k].(float64); return n }
								obs.Buffers = num(root, "Shared Hit Blocks") + num(root, "Shared Read Blocks")
								obs.TempBlocks = num(root, "Temp Read Blocks") + num(root, "Temp Written Blocks")
								var walk func(map[string]any)
								walk = func(n map[string]any) {
									kind, _ := n["Node Type"].(string)
									method, _ := n["Exec Method"].(string)
									if _, exists := n["Filter"]; exists {
										kind += " [residual filter]"
									}
									obs.Nodes = append(obs.Nodes, kind+":"+method)
									obs.HeapFetches += num(n, "Heap Fetches")
									children, _ := n["Plans"].([]any)
									for _, child := range children {
										m, _ := child.(map[string]any)
										walk(m)
									}
								}
								walk(root)
								obs.Outcome = "ok"
							}
						}
					}
					if e != nil {
						var pg *pgconn.PgError
						obs.ErrorCode = "client_error"
						if errors.As(e, &pg) {
							obs.ErrorCode = pg.Code
						}
					}
					tx.Rollback(context.Background())
					conn.Exec(context.Background(), "DEALLOCATE private_parade_plan")
					conn.Release()
					out = append(out, obs)
				}
			}
		}
	}
	return out, nil
}
