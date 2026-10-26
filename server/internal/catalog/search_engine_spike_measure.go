package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SearchEngineSpikeObservation struct {
	ErrorCode string  `json:"error_code,omitempty"`
	Case      int     `json:"case"`
	Family    string  `json:"family"`
	Engine    string  `json:"engine"`
	PlanMode  string  `json:"plan_mode"`
	Clients   int     `json:"clients"`
	Round     int     `json:"round"`
	Page      int     `json:"page"`
	Outcome   string  `json:"outcome"`
	Rows      int     `json:"rows"`
	HasNext   bool    `json:"has_next"`
	QueryMS   float64 `json:"query_ms"`
	RecordsMS float64 `json:"records_ms"`
	TotalMS   float64 `json:"total_ms"`
	Reference string  `json:"reference"`
}
type SearchEngineSpikeReference struct {
	Case    int     `json:"case"`
	Page    int     `json:"page"`
	Outcome string  `json:"outcome"`
	Rows    int     `json:"rows"`
	HasNext bool    `json:"has_next"`
	MS      float64 `json:"ms"`
}
type SearchEngineSpikePlan struct {
	Case        int      `json:"case"`
	Engine      string   `json:"engine"`
	Mode        string   `json:"mode"`
	Outcome     string   `json:"outcome"`
	MS          float64  `json:"ms"`
	Buffers     float64  `json:"buffers"`
	TempBlocks  float64  `json:"temp_blocks"`
	HeapFetches float64  `json:"heap_fetches"`
	Nodes       []string `json:"nodes"`
}
type SearchEngineSpikeReport struct {
	ReferenceCached bool                           `json:"reference_cached"`
	Complete        bool                           `json:"complete"`
	Failures        int                            `json:"failures"`
	Mismatches      int                            `json:"mismatches"`
	Observations    []SearchEngineSpikeObservation `json:"observations"`
	References      []SearchEngineSpikeReference   `json:"references"`
	Plans           []SearchEngineSpikePlan        `json:"plans"`
	WallSeconds     float64                        `json:"wall_seconds"`
}
type spikePosition struct {
	id          int32
	name, first string
}
type spikePage struct {
	positions   []spikePosition
	digest      [32]byte
	observation SearchEngineSpikeObservation
}
type spikeGolden struct {
	ids     []int32
	digest  [32]byte
	hasNext bool
}

func spikeQuery(engine string, q query, after spikePosition) (string, []any) {
	args := []any{after.id, int32(q.Limit + 1)}
	conditions := []string{"id>$1"}
	if q.Sort == "name" && after.id > 0 {
		args = append(args, after.name, after.first)
		// PostgreSQL's row comparison is efficient for B-trees but opaque to the
		// search extension. Equivalent scalar clauses preserve its Top-K pushdown.
		if engine == "parade" {
			conditions = []string{"(name>$3 OR (name=$3 AND first_full>$4) OR (name=$3 AND first_full=$4 AND id>$1))"}
		} else {
			conditions = []string{"(name,first_full,id)>($3,$4,$1)"}
		}
	}
	conditions = append(conditions, searchFieldConditions(q, &args)...)
	if engine == "parade" {
		conditions = append(conditions, "id @@@ pdb.all()")
	}
	order := "id"
	if q.Sort == "name" {
		order = "name,first_full,id"
	}
	return "SELECT id,name,first_full FROM " + engine + " WHERE " + strings.Join(conditions, " AND ") + " ORDER BY " + order + " LIMIT $2", args
}
func spikeRecordDigest(records []result) [32]byte {
	h := sha256.New()
	for _, r := range records {
		var b [8]byte
		binary.LittleEndian.PutUint64(b[:], uint64(r.ID))
		h.Write(b[:])
		h.Write([]byte(r.SourceID))
		h.Write([]byte{0})
		h.Write(r.Record)
		h.Write([]byte{0})
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}
func spikeErrorCode(err error) string {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.Code
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "deadline"
	}
	if errors.Is(err, errRecordIntegrity) {
		return "record_integrity"
	}
	return "client_error"
}
func spikeRead(ctx context.Context, dst *pgxpool.Pool, a *API, edition int16, engine string, q query, after spikePosition) spikePage {
	p := spikePage{observation: SearchEngineSpikeObservation{Engine: engine, Outcome: "query_error", Reference: "unavailable"}}
	start := time.Now()
	// Total search + original-record retrieval retains the five-second request budget.
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	sql, args := spikeQuery(engine, q, after)
	rows, err := dst.Query(c, sql, args...)
	if err == nil {
		for rows.Next() {
			var pos spikePosition
			if err = rows.Scan(&pos.id, &pos.name, &pos.first); err != nil {
				break
			}
			p.positions = append(p.positions, pos)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
	}
	p.observation.QueryMS = time.Since(start).Seconds() * 1000
	if err != nil {
		p.observation.ErrorCode = spikeErrorCode(err)
		p.observation.TotalMS = time.Since(start).Seconds() * 1000
		return p
	}
	p.observation.HasNext = len(p.positions) > q.Limit
	ids := make([]int32, min(q.Limit, len(p.positions)))
	for i := range ids {
		ids[i] = p.positions[i].id
	}
	loadStart := time.Now()
	records, err := a.loadRecords(c, edition, ids)
	p.observation.RecordsMS = time.Since(loadStart).Seconds() * 1000
	p.observation.TotalMS = time.Since(start).Seconds() * 1000
	if err != nil {
		p.observation.Outcome = "record_error"
		p.observation.ErrorCode = spikeErrorCode(err)
		return p
	}
	p.digest = spikeRecordDigest(records)
	p.observation.Rows = len(records)
	p.observation.Outcome = "ok"
	return p
}

// BenchmarkSearchEngines measures two complete private copies. Raw values,
// record identities, digests and cursors never enter its returned report.
func BenchmarkSearchEngines(ctx context.Context, src, dst *pgxpool.Pool, cases []ProjectionCase, rounds, pages, clients int, mode string, reference bool) (SearchEngineSpikeReport, error) {
	r := SearchEngineSpikeReport{Complete: true}
	started := time.Now()
	if len(cases) < 1 || len(cases) > 100 || rounds < 1 || rounds > 4 || pages < 1 || pages > 5 || clients < 1 || clients > 8 {
		return r, errors.New("invalid bounded workload")
	}
	var ready bool
	var expected int64
	if err := dst.QueryRow(ctx, "SELECT finished_at IS NOT NULL AND last_id=expected,expected FROM spike_build WHERE dataset_key='se2003-dpcs0104'").Scan(&ready, &expected); err != nil || !ready || expected != 59886841 {
		return r, errors.New("complete 2003 candidate unavailable")
	}
	var edition int16
	if err := src.QueryRow(ctx, "SELECT id FROM datasets WHERE key='se2003-dpcs0104'").Scan(&edition); err != nil {
		return r, err
	}
	var currentManifest []byte
	var expectedGeneration string
	if err := src.QueryRow(ctx, "SELECT manifest FROM datasets WHERE key='se2003-dpcs0104'").Scan(&currentManifest); err != nil {
		return r, err
	}
	if err := dst.QueryRow(ctx, "SELECT source_fingerprint FROM spike_build WHERE dataset_key='se2003-dpcs0104'").Scan(&expectedGeneration); err != nil {
		return r, err
	}
	manifestHash := sha256.Sum256(currentManifest)
	if hex.EncodeToString(manifestHash[:]) != expectedGeneration {
		return r, errors.New("source changed since projection build")
	}
	queries := make([]query, len(cases))
	for i, c := range cases {
		v, err := url.ParseQuery(c.Query)
		if err != nil {
			return r, errors.New("invalid private case")
		}
		v.Set("dataset", "se2003-dpcs0104")
		v.Set("limit", "50")
		v.Del("cursor")
		q, err := parse(v)
		if err != nil || q.Group != "" || q.Near != "" || q.House != "" || q.Building != "" || q.Flat != "" || q.Middle != "" || (q.Sort != "" && q.Sort != "name") {
			return r, fmt.Errorf("unsupported private case %d", i)
		}
		queries[i] = q
	}
	a := NewAPI(src)
	goldens := map[[2]int]spikeGolden{}
	if reference {
		for i, q := range queries {
			// Unsorted source searches without addresses order by dictionary IDs. Their
			// independent comparison therefore requires an explicit lexical sort.
			if !q.hasAddress() && q.Sort != "name" {
				continue
			}
			for page := 1; page <= pages; page++ {
				c, cancel := context.WithTimeout(ctx, 5*time.Second)
				t := time.Now()
				out, status, err := a.searchResults(c, q)
				cancel()
				o := SearchEngineSpikeReference{Case: i, Page: page, Outcome: "source_error", MS: time.Since(t).Seconds() * 1000}
				if err == nil && status == 200 {
					o.Outcome = "ok"
					o.Rows = len(out.Items)
					o.HasNext = out.Next != ""
					ids := make([]int32, len(out.Items))
					for j, item := range out.Items {
						ids[j] = int32(item.ID)
					}
					goldens[[2]int{i, page}] = spikeGolden{ids, spikeRecordDigest(out.Items), o.HasNext}
				}
				r.References = append(r.References, o)
				if o.Outcome != "ok" || !o.HasNext {
					break
				}
				q.Cursor = out.Next
			}
		}
	}
	// Private reference identities are bound to the exact workload and source
	// generation. They never enter the aggregate report or leave the NAS.
	cachePath := os.Getenv("UKINFO_SPIKE_REFERENCE_FILE")
	if cachePath != "" {
		type savedGolden struct {
			Case, Page int
			IDs        []int32
			Digest     string
			HasNext    bool
		}
		type savedCache struct {
			Workload, Source string
			Values           []savedGolden
			References       []SearchEngineSpikeReference
		}
		rawCases, _ := json.Marshal(cases)
		caseHash := sha256.Sum256(rawCases)
		var sourceHash string
		if err := dst.QueryRow(ctx, "SELECT source_fingerprint FROM spike_build WHERE dataset_key='se2003-dpcs0104'").Scan(&sourceHash); err != nil {
			return r, err
		}
		hash := hex.EncodeToString(caseHash[:])
		if reference {
			saved := savedCache{Workload: hash, Source: sourceHash, References: r.References}
			for k, g := range goldens {
				saved.Values = append(saved.Values, savedGolden{k[0], k[1], g.ids, hex.EncodeToString(g.digest[:]), g.hasNext})
			}
			raw, err := json.Marshal(saved)
			if err != nil {
				return r, err
			}
			if err = os.WriteFile(cachePath+".tmp", raw, 0600); err != nil {
				return r, errors.New("cannot save private references")
			}
			if err = os.Rename(cachePath+".tmp", cachePath); err != nil {
				return r, err
			}
		} else if raw, err := os.ReadFile(cachePath); err == nil {
			var saved savedCache
			if json.Unmarshal(raw, &saved) != nil || saved.Workload != hash || saved.Source != sourceHash {
				return r, errors.New("private reference generation mismatch")
			}
			for _, v := range saved.Values {
				b, e := hex.DecodeString(v.Digest)
				if e != nil || len(b) != 32 {
					return r, errors.New("invalid private reference")
				}
				var d [32]byte
				copy(d[:], b)
				goldens[[2]int{v.Case, v.Page}] = spikeGolden{v.IDs, d, v.HasNext}
			}
			r.References = saved.References
			r.ReferenceCached = true
		} else {
			return r, errors.New("private reference cache unavailable")
		}
	}
	var mu sync.Mutex
	for round := 0; round < rounds; round++ {
		jobs := make(chan int, len(queries))
		for i := range queries {
			jobs <- i
		}
		close(jobs)
		var wg sync.WaitGroup
		for worker := 0; worker < clients; worker++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for ci := range jobs {
					q := queries[ci]
					positions := map[string]spikePosition{}
					done := map[string]bool{}
					for page := 1; page <= pages; page++ {
						reads := map[string]spikePage{}
						for k := 0; k < 2; k++ {
							engine := []string{"native", "parade"}[(round+ci+page+k)%2]
							if done[engine] {
								continue
							}
							p := spikeRead(ctx, dst, a, edition, engine, q, positions[engine])
							o := p.observation
							o.Case = ci
							o.Round = round
							o.Page = page
							o.Clients = clients
							o.PlanMode = mode
							o.Family = "name"
							if q.Postcode != "" {
								o.Family = "postcode"
							}
							if q.hasAddress() {
								o.Family = "address"
							}
							if q.hasBusinessFilters() {
								o.Family = "classification"
							}
							if q.Sort == "name" {
								o.Family += "_lexical"
							}
							if o.Outcome == "ok" {
								if g, ok := goldens[[2]int{ci, page}]; ok {
									ids := make([]int32, min(q.Limit, len(p.positions)))
									for j := range ids {
										ids[j] = p.positions[j].id
									}
									o.Reference = "match"
									if !slices.Equal(ids, g.ids) || p.digest != g.digest || o.HasNext != g.hasNext {
										o.Reference = "mismatch"
									}
								}
								if !o.HasNext {
									done[engine] = true
								} else {
									positions[engine] = p.positions[q.Limit-1]
								}
							} else {
								done[engine] = true
							}
							p.observation = o
							reads[engine] = p
						}
						n, nok := reads["native"]
						p, pok := reads["parade"]
						same := true
						if nok && pok && n.observation.Outcome == "ok" && p.observation.Outcome == "ok" {
							same = len(n.positions) == len(p.positions) && n.digest == p.digest
							for j := 0; same && j < len(n.positions); j++ {
								same = n.positions[j] == p.positions[j]
							}
						}
						mu.Lock()
						if !same {
							r.Mismatches++
						}
						for _, p := range reads {
							r.Observations = append(r.Observations, p.observation)
							if p.observation.Outcome != "ok" {
								r.Failures++
							}
							if p.observation.Reference == "mismatch" {
								r.Mismatches++
							}
						}
						mu.Unlock()
						if !same {
							break
						}
						if ctx.Err() != nil {
							mu.Lock()
							r.Complete = false
							mu.Unlock()
							return
						}
					}
				}
			}()
		}
		wg.Wait()
	}
	r.WallSeconds = time.Since(started).Seconds()
	if ctx.Err() != nil {
		r.Complete = false
	}
	return r, nil
}

// CaptureSearchEnginePlans strips all query text and predicate values before
// returning only allowlisted work counters and node/index names.
func CaptureSearchEnginePlans(ctx context.Context, dst *pgxpool.Pool, cases []ProjectionCase, analyze bool) ([]SearchEngineSpikePlan, error) {
	out := []SearchEngineSpikePlan{}
	for _, mode := range []string{"force_custom_plan", "force_generic_plan"} {
		for _, engine := range []string{"native", "parade"} {
			for _, ci := range []int{0, 1, 4, 8, 16, 24, 36, 48, 64, 80} {
				if ci >= len(cases) {
					continue
				}
				v, e := url.ParseQuery(cases[ci].Query)
				if e != nil {
					return nil, e
				}
				v.Set("dataset", "se2003-dpcs0104")
				v.Set("limit", "50")
				q, e := parse(v)
				if e != nil {
					return nil, e
				}
				sql, args := spikeQuery(engine, q, spikePosition{})
				conn, e := dst.Acquire(ctx)
				if e != nil {
					return nil, e
				}
				tx, e := conn.Begin(ctx)
				if e != nil {
					conn.Release()
					return nil, e
				}
				_, e = tx.Exec(ctx, "SET LOCAL plan_cache_mode="+mode)
				if e == nil {
					_, e = tx.Exec(ctx, "PREPARE real_spike_plan AS "+sql)
				}
				obs := SearchEngineSpikePlan{Case: ci, Engine: engine, Mode: mode, Outcome: "plan_error"}
				if e == nil {
					lit := []string{}
					for _, arg := range args {
						switch x := arg.(type) {
						case string:
							lit = append(lit, "'"+strings.ReplaceAll(x, "'", "''")+"'")
						case int:
							lit = append(lit, fmt.Sprint(x))
						case int16:
							lit = append(lit, fmt.Sprint(x))
						case int32:
							lit = append(lit, fmt.Sprint(x))
						default:
							e = errors.New("unsupported private argument")
						}
					}
					var raw []byte
					t := time.Now()
					if e == nil {
						explain := "EXPLAIN(FORMAT JSON)"
						if analyze {
							explain = "EXPLAIN(ANALYZE,BUFFERS,FORMAT JSON)"
						}
						e = tx.QueryRow(ctx, explain+" EXECUTE real_spike_plan("+strings.Join(lit, ",")+")").Scan(&raw)
					}
					obs.MS = time.Since(t).Seconds() * 1000
					if e == nil {
						var plan []map[string]any
						if json.Unmarshal(raw, &plan) != nil {
							e = errors.New("invalid plan JSON")
						} else {
							obs.Outcome = "ok"
							root, _ := plan[0]["Plan"].(map[string]any)
							num := func(m map[string]any, k string) float64 { x, _ := m[k].(float64); return x }
							obs.Buffers = num(root, "Shared Hit Blocks") + num(root, "Shared Read Blocks")
							obs.TempBlocks = num(root, "Temp Read Blocks") + num(root, "Temp Written Blocks")
							var walk func(map[string]any)
							walk = func(n map[string]any) {
								kind, _ := n["Node Type"].(string)
								idx, _ := n["Index Name"].(string)
								exec, _ := n["Exec Method"].(string)
								if _, ok := n["Filter"]; ok {
									kind += " [residual filter]"
								}
								obs.Nodes = append(obs.Nodes, kind+":"+idx+exec)
								obs.HeapFetches += num(n, "Heap Fetches")
								children, _ := n["Plans"].([]any)
								for _, child := range children {
									m, _ := child.(map[string]any)
									walk(m)
								}
							}
							walk(root)
						}
					}
				}
				tx.Rollback(context.Background())
				conn.Exec(context.Background(), "DEALLOCATE real_spike_plan")
				conn.Release()
				out = append(out, obs)
			}
		}
	}
	return out, nil
}

// CreateSearchEngineCases adds private guaranteed-positive, geographically spread
// cases without exporting the sampled records or their field values.
func CreateSearchEngineCases(ctx context.Context, dst *pgxpool.Pool, seeds []ProjectionCase) ([]ProjectionCase, error) {
	out := []ProjectionCase{}
	seen := map[string]bool{}
	limit := 98
	add := func(v url.Values) {
		v.Set("dataset", "se2003-dpcs0104")
		v.Set("limit", "50")
		v.Del("cursor")
		q, e := parse(v)
		if e != nil || q.Group != "" || q.Near != "" || q.House != "" || q.Building != "" || q.Flat != "" || q.Middle != "" || (q.Sort != "" && q.Sort != "name") {
			return
		}
		if !q.hasAddress() && q.Sort == "" {
			v.Set("sort", "name")
		}
		s := v.Encode()
		if !seen[s] && len(out) < limit {
			seen[s] = true
			out = append(out, ProjectionCase{Query: s})
		}
	}
	for _, seed := range seeds {
		v, e := url.ParseQuery(seed.Query)
		if e != nil {
			return nil, e
		}
		add(v)
	}
	ids := []int32{1201, 1000001, 5000001, 10000001, 15000001, 20000001, 25000001, 30000001, 35000001, 40000001, 45000001, 50000001, 55000001, 57500001, 58500001, 59500001}
	rows, e := dst.Query(ctx, "SELECT kind,name,given,postcode,street,town,county,sic FROM native WHERE id=ANY($1::integer[]) ORDER BY id", ids)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	sampleIndex := 0
	for rows.Next() {
		current := sampleIndex
		sampleIndex++
		var kind int16
		var name, first, postcode, street, town, county, sic string
		if e = rows.Scan(&kind, &name, &first, &postcode, &street, &town, &county, &sic); e != nil {
			return nil, e
		}
		base := url.Values{"kind": {"person"}, "surname": {name}, "first_name": {first}, "first_match": {"given"}}
		if kind == 1 {
			base = url.Values{"kind": {"business"}, "organisation": {name}}
		}
		clone := func() url.Values {
			v := url.Values{}
			for k, vs := range base {
				v[k] = slices.Clone(vs)
			}
			return v
		}
		if town != "" {
			v := clone()
			v.Set("town", town)
			add(v)
			v.Set("sort", "name")
			add(v)
		}
		if county != "" && current%2 == 0 {
			v := clone()
			v.Set("county", county)
			add(v)
		}
		if len(postcode) >= 5 && current%3 == 0 {
			v := clone()
			prefix := postcode
			if parsed, e := parse(url.Values{"dataset": {"se2003-dpcs0104"}, "postcode": {postcode}}); e == nil && parsed.PostcodeLength == 0 {
				prefix = postcode[:len(postcode)-3]
			}
			v.Set("postcode", prefix)
			add(v)
		}
		if kind == 1 && sic != "" && town != "" {
			v := url.Values{"kind": {"business"}, "sic": {sic}, "town": {town}}
			add(v)
		}
		if street != "" && town != "" && current%4 == 0 {
			v := url.Values{"kind": {base.Get("kind")}, "street": {street}, "town": {town}}
			add(v)
		}
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	limit = 100
	for _, suffix := range []string{"", "name"} {
		v := url.Values{"kind": {"person"}, "surname": {"williams"}, "first_name": {"jonathan"}, "first_match": {"given"}, "town": {"ukinfo benchmark absent town 20261025"}, "sort": {suffix}}
		add(v)
	}
	return out, nil
}
