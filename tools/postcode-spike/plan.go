package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type planSummary struct {
	GenericPlanConfirmed bool    `json:"generic_plan_confirmed"`
	ExpectedIndexUsed    bool    `json:"expected_index_used"`
	Nodes                int64   `json:"nodes"`
	IndexOnlyScans       int64   `json:"index_only_scans"`
	OtherIndexScans      int64   `json:"other_index_scans"`
	SequentialScans      int64   `json:"sequential_scans"`
	ScanRows             int64   `json:"scan_rows"`
	HeapFetchesAvailable bool    `json:"heap_fetches_available"`
	HeapFetches          int64   `json:"heap_fetches"`
	RootRows             int64   `json:"root_rows"`
	RootLoops            int64   `json:"root_loops"`
	SharedHitBlocks      int64   `json:"shared_hit_blocks"`
	SharedReadBlocks     int64   `json:"shared_read_blocks"`
	TempReadBlocks       int64   `json:"temp_read_blocks"`
	TempWrittenBlocks    int64   `json:"temp_written_blocks"`
	Sorts                int64   `json:"sorts"`
	SortWorkers          int64   `json:"sort_workers"`
	ExternalSorts        int64   `json:"external_sorts"`
	MaxSortKB            int64   `json:"max_sort_kb"`
	PlanningMS           float64 `json:"planning_ms"`
	ExecutionMS          float64 `json:"execution_ms"`
}

const maxPlanCounter int64 = 1_000_000_000_000

func planCount(m map[string]any, key string) (int64, error) {
	n, ok := m[key].(json.Number)
	if !ok {
		return 0, errors.New("missing plan counter")
	}
	v, err := n.Int64()
	if err != nil || v < 0 || v > maxPlanCounter {
		return 0, errors.New("invalid plan counter")
	}
	return v, nil
}
func planMS(m map[string]any, key string) (float64, error) {
	n, ok := m[key].(json.Number)
	if !ok {
		return 0, errors.New("missing plan timing")
	}
	v, err := n.Float64()
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 60000 {
		return 0, errors.New("invalid plan timing")
	}
	return v, nil
}
func summarizeExplain(raw []byte, expected string) (planSummary, error) {
	p := planSummary{}
	bad := func() (planSummary, error) { return planSummary{}, errors.New("bounded executed plan required") }
	if len(raw) > 1<<20 || !regexp.MustCompile(`^p8_pc_d[1-9][0-9]{0,4}_[a-f0-9]{24}_cover$`).MatchString(expected) {
		return bad()
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var docs []map[string]any
	if dec.Decode(&docs) != nil || len(docs) != 1 {
		return bad()
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return bad()
	}
	root, ok := docs[0]["Plan"].(map[string]any)
	if !ok {
		return bad()
	}
	var err error
	if p.RootRows, err = planCount(root, "Actual Rows"); err != nil || p.RootRows > 2147483647 {
		return bad()
	}
	if p.RootLoops, err = planCount(root, "Actual Loops"); err != nil || p.RootLoops < 1 {
		return bad()
	}
	counters := []struct {
		key string
		out *int64
	}{{"Shared Hit Blocks", &p.SharedHitBlocks}, {"Shared Read Blocks", &p.SharedReadBlocks}, {"Temp Read Blocks", &p.TempReadBlocks}, {"Temp Written Blocks", &p.TempWrittenBlocks}}
	for _, c := range counters {
		if *c.out, err = planCount(root, c.key); err != nil {
			return bad()
		}
	}
	if p.PlanningMS, err = planMS(docs[0], "Planning Time"); err != nil {
		return bad()
	}
	if p.ExecutionMS, err = planMS(docs[0], "Execution Time"); err != nil {
		return bad()
	}
	var walk func(map[string]any, int) error
	walk = func(n map[string]any, depth int) error {
		p.Nodes++
		if depth > 16 || p.Nodes > 128 {
			return errors.New("plan exceeds bound")
		}
		rows, e := planCount(n, "Actual Rows")
		if e != nil {
			return e
		}
		loops, e := planCount(n, "Actual Loops")
		if e != nil {
			return e
		}
		typ, ok := n["Node Type"].(string)
		if !ok {
			return errors.New("missing node type")
		}
		scan := false
		switch typ {
		case "Index Only Scan":
			if loops > 0 {
				p.IndexOnlyScans++
				p.HeapFetchesAvailable = true
			}
			heap, e := planCount(n, "Heap Fetches")
			if e != nil {
				return e
			}
			if loops > 0 {
				p.HeapFetches += heap
			}
			scan = true
		case "Index Scan", "Bitmap Index Scan":
			if loops > 0 {
				p.OtherIndexScans++
			}
			scan = true
		case "Seq Scan":
			if loops > 0 {
				p.SequentialScans++
			}
			scan = true
		case "Sort":
			if loops > 0 {
				p.Sorts++
				var workers []any
				if raw, exists := n["Workers"]; exists {
					var ok bool
					workers, ok = raw.([]any)
					if !ok || len(workers) > 64 {
						return errors.New("bounded sort workers required")
					}
				}
				addSort := func(info map[string]any) error {
					method, _ := info["Sort Method"].(string)
					space, _ := info["Sort Space Type"].(string)
					switch method {
					case "top-N heapsort", "quicksort":
						if space != "Memory" {
							return errors.New("invalid memory sort storage")
						}
					case "external merge", "external sort":
						if space != "Disk" {
							return errors.New("invalid external sort storage")
						}
						p.ExternalSorts++
					default:
						return errors.New("unknown sort method")
					}
					kb, e := planCount(info, "Sort Space Used")
					if e != nil {
						return e
					}
					if kb > p.MaxSortKB {
						p.MaxSortKB = kb
					}
					return nil
				}
				if _, exists := n["Sort Method"]; exists {
					if e := addSort(n); e != nil {
						return e
					}
				} else if len(workers) == 0 {
					return errors.New("executed sort details unavailable")
				}
				for _, worker := range workers {
					info, ok := worker.(map[string]any)
					if !ok {
						return errors.New("invalid sort worker")
					}
					if e := addSort(info); e != nil {
						return e
					}
					p.SortWorkers++
				}
			}
		case "Limit", "Result", "Bitmap Heap Scan", "Gather", "Gather Merge", "Materialize", "Unique", "Append", "Merge Append", "Subquery Scan":
		default:
			return errors.New("unknown native plan node")
		}
		if scan {
			if loops != 0 && rows > maxPlanCounter/loops {
				return errors.New("scan counter overflow")
			}
			p.ScanRows += rows * loops
			if typ != "Seq Scan" {
				name, ok := n["Index Name"].(string)
				if !ok || len(name) < 1 || len(name) > 63 {
					return errors.New("missing index identity")
				}
				p.ExpectedIndexUsed = p.ExpectedIndexUsed || (loops > 0 && name == expected)
			}
		}
		if p.ScanRows > maxPlanCounter || p.HeapFetches > maxPlanCounter {
			return errors.New("aggregate exceeds bound")
		}
		if children, exists := n["Plans"]; exists {
			list, ok := children.([]any)
			if !ok {
				return errors.New("invalid child plans")
			}
			for _, child := range list {
				m, ok := child.(map[string]any)
				if !ok {
					return errors.New("invalid child node")
				}
				if e := walk(m, depth+1); e != nil {
					return e
				}
			}
		}
		return nil
	}
	if walk(root, 0) != nil {
		return bad()
	}
	return p, nil
}

type preparedPlanConnection interface {
	Prepare(context.Context, string, string) (*pgconn.StatementDescription, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Deallocate(context.Context, string) error
}

func probePreparedPlan(ctx context.Context, conn preparedPlanConnection, s statement, expected string) (summary planSummary, failure error) {
	native, err := coveringVariant(s.SQL, s.Args)
	if err != nil {
		return summary, errors.New("exact district query required")
	}
	const name = "ukinfo_postcode_cover_probe"
	if _, err = conn.Prepare(ctx, name, native); err != nil {
		return summary, errors.New("native prepare failed")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if conn.Deallocate(cleanup, name) != nil && failure == nil {
			failure = errors.New("native deallocation failed")
		}
	}()
	// pgx simple protocol safely encodes EXECUTE's argument expressions. The
	// already prepared SELECT, under force_generic_plan, owns the actual plan.
	args := append([]any{pgx.QueryExecModeSimpleProtocol}, s.Args...)
	var raw []byte
	err = conn.QueryRow(ctx, "EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON,TIMING OFF,SUMMARY ON) EXECUTE "+name+"($1,$2,$3,$4,$5,$6,$7)", args...).Scan(&raw)
	if err != nil {
		return summary, err
	}
	summary, err = summarizeExplain(raw, expected)
	if err != nil {
		return planSummary{}, err
	}
	var generic, custom int64
	if conn.QueryRow(ctx, "SELECT generic_plans,custom_plans FROM pg_prepared_statements WHERE name='"+name+"'").Scan(&generic, &custom) != nil || generic != 1 || custom != 0 {
		return planSummary{}, errors.New("actual generic plan not confirmed")
	}
	summary.GenericPlanConfirmed = true
	return summary, nil
}
