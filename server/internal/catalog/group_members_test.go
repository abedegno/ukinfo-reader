package catalog

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testGroupRepresentativePlans(t *testing.T, db *pgxpool.Pool, dataset int16) {
	t.Helper()
	ctx := context.Background()
	rows, err := db.Query(ctx, "SELECT id,group_key FROM research_keys WHERE dataset_id=$1 AND id=ANY(ARRAY[1,25000,50000]) ORDER BY id", dataset)
	if err != nil {
		t.Fatal(err)
	}
	groups := [][]byte{}
	expected := map[string]int32{}
	for rows.Next() {
		var id int32
		var g []byte
		if err = rows.Scan(&id, &g); err != nil {
			t.Fatal(err)
		}
		groups = append(groups, g)
		expected[hex.EncodeToString(g)] = id
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(groups) != 3 {
		t.Fatal("representative fixture", err)
	}
	var later int16
	if err = db.QueryRow(ctx, `INSERT INTO datasets(key,manifest,entries) VALUES('later-group-work','{}',3) RETURNING id`).Scan(&later); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, fmt.Sprintf("CREATE TABLE research_d%d PARTITION OF research_keys FOR VALUES IN (%d)", later, later)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO research_keys SELECT $1,row_number() OVER(ORDER BY id),source_hash,group_key,sic,industry FROM research_keys WHERE dataset_id=$2 AND id=ANY(ARRAY[1,25000,50000])`, later, dataset); err != nil {
		t.Fatal(err)
	}
	observations := []map[string]any{}
	for _, mode := range []string{"force_custom_plan", "force_generic_plan"} {
		cfg := db.Config().Copy()
		cfg.MaxConns = 1
		cfg.MinConns = 0
		cfg.ConnConfig.RuntimeParams["plan_cache_mode"] = mode
		probe, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			t.Fatal(err)
		}
		func() {
			defer probe.Close()
			a := NewAPI(probe)
			// Reverse edition input and lower ordinals in the later edition must not
			// change the original min(dataset_id,id) representative contract.
			members, err := a.groupRepresentatives(ctx, []int16{later, dataset}, groups)
			if err != nil || len(members) != 3 {
				t.Fatal("group representatives", err)
			}
			for key, id := range expected {
				if m := members[key]; m.dataset != dataset || m.id != id {
					t.Fatal("wrong representative")
				}
			}
			if a.metrics.phases[phaseKey{"group_members", "ok"}].Count != 1 {
				t.Fatal("unnecessary later-edition probes")
			}
			if _, err = a.groupRepresentatives(ctx, []int16{later, dataset}, [][]byte{make([]byte, 32)}); !errors.Is(err, errRecordIntegrity) {
				t.Fatal("missing group accepted", err)
			}
			if _, err = probe.Exec(ctx, "PREPARE representative_work(smallint,bytea[]) AS "+groupRepresentativeSQL); err != nil {
				t.Fatal(err)
			}
			literals := []string{}
			for _, g := range groups {
				literals = append(literals, "decode('"+hex.EncodeToString(g)+"','hex')")
			}
			execute := fmt.Sprintf("EXECUTE representative_work(%d,ARRAY[%s])", dataset, strings.Join(literals, ","))
			var raw []byte
			if err = probe.QueryRow(ctx, "EXPLAIN(ANALYZE,BUFFERS,FORMAT JSON) "+execute).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var plan []map[string]any
			if err = json.Unmarshal(raw, &plan); err != nil {
				t.Fatal(err)
			}
			root := plan[0]["Plan"].(map[string]any)
			blocks := root["Shared Hit Blocks"].(float64) + root["Shared Read Blocks"].(float64)
			if blocks >= 100 {
				t.Fatalf("representative buffer budget: %g", blocks)
			}
			scans := 0
			var walk func(map[string]any)
			walk = func(n map[string]any) {
				if relation, ok := n["Relation Name"].(string); ok && strings.HasPrefix(relation, "research_d") && n["Actual Loops"] != float64(0) {
					scans++
					if relation != fmt.Sprintf("research_d%d", dataset) || n["Node Type"] != "Index Only Scan" || n["Heap Fetches"] != float64(0) || n["Actual Rows"].(float64) > 1 {
						t.Fatal("representative must prune editions and stop after one covered row per group", mode)
					}
				}
				if children, ok := n["Plans"].([]any); ok {
					for _, child := range children {
						walk(child.(map[string]any))
					}
				}
			}
			walk(root)
			if scans != 1 {
				t.Fatal("expected one representative partition", scans)
			}
			observations = append(observations, map[string]any{"mode": mode, "execution_ms": plan[0]["Execution Time"], "shared_blocks": blocks, "groups": 3, "heap_fetches": 0})
		}()
	}
	// The first edition can miss a group that exists in both later probes.
	// A lower ordinal in a later edition must not beat the earlier edition,
	// regardless of which concurrent probe finishes first.
	var third int16
	if err = db.QueryRow(ctx, `INSERT INTO datasets(key,manifest,entries) VALUES('parallel-later-group-work','{}',1) RETURNING id`).Scan(&third); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, fmt.Sprintf("CREATE TABLE research_d%d PARTITION OF research_keys FOR VALUES IN (%d)", third, third)); err != nil {
		t.Fatal(err)
	}
	unique, _ := hex.DecodeString(strings.Repeat("ef", 32))
	if _, err = db.Exec(ctx, `INSERT INTO research_keys SELECT $1,4,source_hash,$3,'','' FROM research_keys WHERE dataset_id=$2 AND id=1`, later, dataset, unique); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO research_keys SELECT $1,1,source_hash,group_key,'','' FROM research_keys WHERE dataset_id=$2 AND id=4`, third, later); err != nil {
		t.Fatal(err)
	}
	probe := NewAPI(db)
	members, err := probe.groupRepresentatives(ctx, []int16{third, later, dataset}, [][]byte{unique})
	if err != nil || len(members) != 1 || members[hex.EncodeToString(unique)].dataset != later || members[hex.EncodeToString(unique)].id != 4 {
		t.Fatal("parallel representative changed canonical source order", err)
	}
	countGroups := append(slices.Clone(groups), unique)
	countDatasets := []int16{third, later, dataset}
	countRows, err := db.Query(ctx, `SELECT k.group_key,count(*),array_agg(DISTINCT d.key ORDER BY d.key) FROM research_keys k JOIN datasets d ON d.id=k.dataset_id WHERE k.group_key=ANY($1::bytea[]) AND k.dataset_id=ANY($2::smallint[]) GROUP BY k.group_key`, countGroups, countDatasets)
	if err != nil {
		t.Fatal(err)
	}
	wantCounts := map[string]listingMember{}
	for countRows.Next() {
		var key []byte
		var m listingMember
		if err = countRows.Scan(&key, &m.count, &m.editions); err != nil {
			t.Fatal(err)
		}
		wantCounts[hex.EncodeToString(key)] = m
	}
	err = countRows.Err()
	countRows.Close()
	if err != nil {
		t.Fatal(err)
	}
	gotCounts, err := probe.groupOccurrences(ctx, countDatasets, countGroups)
	if err != nil || !reflect.DeepEqual(gotCounts, wantCounts) {
		t.Fatal("parallel group counts or database edition order changed", err)
	}
	if _, err = probe.groupOccurrences(ctx, countDatasets, [][]byte{make([]byte, 32)}); !errors.Is(err, errRecordIntegrity) {
		t.Fatal("missing group count accepted", err)
	}
	if dir := os.Getenv("UKINFO_PERFORMANCE_REPORT_DIR"); dir != "" {
		if err = os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		raw, err := json.MarshalIndent(map[string]any{"fixture_entries": 50000, "plans": observations}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "group-member-plans.json"), append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
