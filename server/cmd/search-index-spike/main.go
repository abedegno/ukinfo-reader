// Command search-index-spike compares disposable synthetic search representations.
// It is an operator experiment, not a production migration or capacity test.
package main

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Listing struct {
	ID                                                  int
	Surname, Given, Town, County, Postcode, Street, SIC string
	Kind, Edition                                       int
}
type Case struct {
	Name   string            `json:"name"`
	Fields map[string]string `json:"fields"`
	Prefix map[string]string `json:"prefix,omitempty"`
	Street string            `json:"street,omitempty"`
}
type Sample struct {
	Engine, Case  string
	Clients, Page int
	MS            float64
	Error         string `json:",omitempty"`
}
type Build struct {
	Engine                 string
	Seconds                float64
	TableBytes, IndexBytes int64
}
type Report struct {
	Rows          int
	Version       string
	Extensions    map[string]string
	Settings      map[string]string
	Builds        []Build
	Cases         []Case
	Samples       []Sample
	Plans         map[string]json.RawMessage
	VerifiedPages int
	CompleteWalks int
	Errors        int
	Summaries     []Summary
}
type Summary struct {
	Engine                    string
	Clients, Requests, Errors int
	P50MS, P95MS, P99MS       float64
}

func row(i int) Listing {
	h := md5.Sum([]byte(fmt.Sprint(i)))
	a := int(binary.BigEndian.Uint32(h[0:4]))
	b := int(binary.BigEndian.Uint32(h[4:8]))
	c := int(binary.BigEndian.Uint32(h[8:12]))
	d := int(binary.BigEndian.Uint32(h[12:16]))
	r := Listing{ID: i, Surname: fmt.Sprintf("s%03d", a%997), Given: fmt.Sprintf("g%03d", b%127), Town: fmt.Sprintf("town%03d", c%997), County: fmt.Sprintf("county%02d", d%23), Postcode: fmt.Sprintf("zz%03d %04d", c%1000, d%10000), Street: "willow road", SIC: fmt.Sprintf("%04d", a%200), Kind: d % 8 / 7, Edition: b % 5}
	if a%10 < 3 {
		r.Surname = "williams"
	}
	if b%10 < 2 {
		r.Given = "jonathan"
	}
	if c%100 < 20 {
		r.Town = "metropolis"
	}
	if d%5 == 0 {
		r.Street = "oak lane"
	}
	return r
}
func value(r Listing, k string) string {
	switch k {
	case "surname":
		return r.Surname
	case "given":
		return r.Given
	case "town":
		return r.Town
	case "county":
		return r.County
	case "postcode":
		return r.Postcode
	case "sic":
		return r.SIC
	case "kind":
		return fmt.Sprint(r.Kind)
	case "edition":
		return fmt.Sprint(r.Edition)
	}
	panic(k)
}

var tags = map[string]string{"surname": "sn", "given": "gn", "town": "tw", "county": "co", "postcode": "pc", "sic": "sc", "kind": "ki", "edition": "ed"}

func token(s string) string { return strings.ReplaceAll(s, " ", "") }
func matches(r Listing, c Case) bool {
	for k, v := range c.Fields {
		if value(r, k) != v {
			return false
		}
	}
	for k, v := range c.Prefix {
		if !strings.HasPrefix(value(r, k), v) {
			return false
		}
	}
	return c.Street == "" || r.Street == c.Street
}
func query(engine string, c Case, after, limit int) (string, []any) {
	args := []any{after, limit}
	where := []string{"id>$1"}
	var ts []string
	keys := make([]string, 0, len(c.Fields))
	for k := range c.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := c.Fields[k]
		ts = append(ts, tags[k]+token(v))
		args = append(args, v)
		cast := ""
		if k == "kind" || k == "edition" {
			cast = "::text::integer"
		}
		where = append(where, fmt.Sprintf("%s=$%d%s", k, len(args), cast))
	}
	keys = keys[:0]
	for k := range c.Prefix {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := c.Prefix[k]
		ts = append(ts, tags[k]+token(v)+":*")
		args = append(args, v, v+"~")
		where = append(where, fmt.Sprintf("%s >= $%d AND %s < $%d", k, len(args)-1, k, len(args)))
	}
	if c.Street != "" {
		for _, s := range strings.Fields(c.Street) {
			ts = append(ts, "st"+s)
		}
		args = append(args, c.Street)
		if engine == "parade" {
			where = append(where, fmt.Sprintf("street &&& $%d", len(args)))
		} else {
			where = append(where, fmt.Sprintf("street_vec @@ plainto_tsquery('simple',$%d)", len(args)))
		}
	}
	if engine == "text" {
		args = []any{after, limit, strings.Join(ts, " & ")}
		where = []string{"id>$1", "doc @@ to_tsquery('simple',$3)"}
	}
	if engine == "parade" { // An explicit search condition enables the custom scan even without street text.
		where = append(where, "id @@@ pdb.all()")
	}
	return fmt.Sprintf("SELECT id FROM %s WHERE %s ORDER BY id LIMIT $2", engine, strings.Join(where, " AND ")), args
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
func main() {
	n := flag.Int("rows", 1000000, "synthetic listings (1000..2000000)")
	rounds := flag.Int("rounds", 4, "rounds per concurrency level")
	reuse := flag.String("measure-existing", "", "reuse synthetic fixtures and build metadata from this prior report")
	planMode := flag.String("plan-mode", "auto", "auto or force_custom_plan")
	diagnose := flag.Bool("diagnose", false, "append custom/generic prepared plans to an existing report; no data builds")
	output := flag.String("output", "/tmp/search-index-spike.json", "aggregate report path")
	flag.Parse()
	if *planMode != "auto" && *planMode != "force_custom_plan" {
		panic("unsupported plan mode")
	}
	if *n < 1000 || *n > 2000000 || *rounds < 1 || *rounds > 10 {
		panic("experiment bounds exceeded")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	cfg, err := pgxpool.ParseConfig("postgres://postgres@/spike?host=/var/run/postgresql&sslmode=disable")
	must(err)
	cfg.MaxConns = 4
	cfg.ConnConfig.RuntimeParams = map[string]string{"statement_timeout": "10000", "default_text_search_config": "pg_catalog.simple", "application_name": "synthetic-search-index-spike", "plan_cache_mode": *planMode}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	must(err)
	defer pool.Close()
	if !*diagnose && *reuse == "" {
		_, err = pool.Exec(ctx, "CREATE DATABASE spike_text")
		must(err)
	}
	textcfg := cfg.Copy()
	textcfg.ConnConfig.Database = "spike_text"
	textpool, err := pgxpool.NewWithConfig(ctx, textcfg)
	must(err)
	defer textpool.Close()
	enginePool := func(e string) *pgxpool.Pool {
		if e == "text" {
			return textpool
		}
		return pool
	}
	if *diagnose {
		var r Report
		data, err := os.ReadFile(*output)
		must(err)
		must(json.Unmarshal(data, &r))
		capturePlans(ctx, enginePool, &r)
		data, err = json.MarshalIndent(r, "", "  ")
		must(err)
		must(os.WriteFile(*output, append(data, '\n'), 0644))
		return
	}
	report := Report{Rows: *n, Extensions: map[string]string{}, Settings: map[string]string{}, Plans: map[string]json.RawMessage{}}
	engines := []string{"btree", "gin", "parade", "text"}
	if *reuse != "" {
		data, err := os.ReadFile(*reuse)
		must(err)
		must(json.Unmarshal(data, &report))
		if report.Rows != *n {
			panic("row count differs from reused fixture")
		}
		report.Samples = nil
		report.Summaries = nil
		report.Plans = map[string]json.RawMessage{}
		report.Errors = 0
		report.VerifiedPages = 0
		report.CompleteWalks = 0
	} else {
		must(pool.QueryRow(ctx, "SELECT version()").Scan(&report.Version))
		for _, e := range []string{"vector", "pg_search", "pg_textsearch", "btree_gin"} {
			ep := pool
			if e == "pg_textsearch" {
				ep = textpool
			}
			_, err = ep.Exec(ctx, "CREATE EXTENSION "+e+" CASCADE")
			must(err)
			var v string
			must(ep.QueryRow(ctx, "SELECT extversion FROM pg_extension WHERE extname=$1", e).Scan(&v))
			report.Extensions[e] = v
		}
		for _, s := range []string{"shared_buffers", "work_mem", "max_parallel_workers_per_gather", "jit", "io_method", "effective_io_concurrency"} {
			var v string
			must(pool.QueryRow(ctx, "SHOW "+s).Scan(&v))
			report.Settings[s] = v
		}
		start := time.Now()
		for _, seedpool := range []*pgxpool.Pool{pool, textpool} {
			_, err = seedpool.Exec(ctx, `CREATE TABLE seed(id integer PRIMARY KEY,surname text COLLATE "C",given text COLLATE "C",town text COLLATE "C",county text COLLATE "C",postcode text COLLATE "C",street text COLLATE "C",sic text COLLATE "C",kind integer,edition integer,doc text,street_vec tsvector)`)
			must(err)
			start = time.Now()
			conn, err := seedpool.Acquire(ctx)
			must(err)
			_, err = conn.Exec(ctx, "SET statement_timeout='180s'")
			must(err)
			_, err = conn.Exec(ctx, fmt.Sprintf(`INSERT INTO seed
 WITH h AS MATERIALIZED (SELECT i,md5(i::text) h FROM generate_series(1,%d) i), nums AS(SELECT i,('x'||substr(h,1,8))::bit(32)::bigint a,('x'||substr(h,9,8))::bit(32)::bigint b,('x'||substr(h,17,8))::bit(32)::bigint c,('x'||substr(h,25,8))::bit(32)::bigint d FROM h), fields AS(SELECT i,
 CASE WHEN a%%10<3 THEN 'williams' ELSE 's'||lpad((a%%997)::text,3,'0') END sn,
 CASE WHEN b%%10<2 THEN 'jonathan' ELSE 'g'||lpad((b%%127)::text,3,'0') END gn,
 CASE WHEN c%%100<20 THEN 'metropolis' ELSE 'town'||lpad((c%%997)::text,3,'0') END tw,
 'county'||lpad((d%%23)::text,2,'0') co,'zz'||lpad((c%%1000)::text,3,'0')||' '||lpad((d%%10000)::text,4,'0') pc,
 CASE WHEN d%%5=0 THEN 'oak lane' ELSE 'willow road' END st,lpad((a%%200)::text,4,'0') sc,(d%%8/7)::integer ki,(b%%5)::integer ed FROM nums)
 SELECT i,sn,gn,tw,co,pc,st,sc,ki,ed,
 'sn'||sn||' gn'||gn||' tw'||tw||' co'||co||' pc'||replace(pc,' ','')||' sc'||sc||' ki'||ki||' ed'||ed||' st'||replace(st,' ',' st'),to_tsvector('simple',st) FROM fields`, *n))
			must(err)
			fmt.Fprintf(os.Stderr, "seed: %d rows in %.1fs\n", *n, time.Since(start).Seconds())
			conn.Release()
		}
		indexes := map[string]string{
			"btree":  `CREATE INDEX ON btree(surname,given,town,id);CREATE INDEX ON btree(surname,given,id);CREATE INDEX ON btree(town,id);CREATE INDEX ON btree(county,id);CREATE INDEX ON btree(postcode,id);CREATE INDEX ON btree(kind,sic,id);CREATE INDEX ON btree USING gin(street_vec)`,
			"gin":    `CREATE INDEX ON gin USING gin(surname,given,town,county,postcode,sic,kind,edition,street_vec)`,
			"parade": `CREATE INDEX parade_search ON parade USING paradedb(id,(surname::pdb.literal),(given::pdb.literal),(town::pdb.literal),(county::pdb.literal),(postcode::pdb.literal),(sic::pdb.literal),kind,edition,street)`,
			"text":   `CREATE INDEX text_search ON text USING bm25(doc) WITH(text_config='simple')`}
		for _, e := range engines {
			conn, err := enginePool(e).Acquire(ctx)
			must(err)
			_, err = conn.Exec(ctx, "SET statement_timeout='180s'")
			must(err)
			start = time.Now()
			_, err = conn.Exec(ctx, "CREATE TABLE "+e+" (LIKE seed INCLUDING ALL); INSERT INTO "+e+" SELECT * FROM seed;"+indexes[e])
			must(err)
			_, err = conn.Exec(ctx, "VACUUM ANALYZE "+e)
			must(err)
			var b Build
			b.Engine = e
			b.Seconds = time.Since(start).Seconds()
			must(conn.QueryRow(ctx, "SELECT pg_table_size($1::regclass),pg_indexes_size($1::regclass)", e).Scan(&b.TableBytes, &b.IndexBytes))
			report.Builds = append(report.Builds, b)
			fmt.Fprintf(os.Stderr, "%s: %.1fs, %.1fMiB indexes\n", e, b.Seconds, float64(b.IndexBytes)/(1024*1024))
			_, err = conn.Exec(ctx, "SET statement_timeout='10s'")
			must(err)
			conn.Release()
		}
	}
	report.Settings["plan_cache_mode"] = *planMode
	report.Cases = []Case{
		{Name: "common-name-town", Fields: map[string]string{"surname": "williams", "given": "jonathan", "town": "metropolis"}},
		{Name: "common-name-county", Fields: map[string]string{"surname": "williams", "given": "jonathan", "county": "county07"}},
		{Name: "common-name-postcode-prefix", Fields: map[string]string{"surname": "williams", "given": "jonathan"}, Prefix: map[string]string{"postcode": "zz01"}},
		{Name: "name-town-street", Fields: map[string]string{"surname": "williams", "town": "metropolis"}, Street: "oak lane"},
		{Name: "broad-town", Fields: map[string]string{"town": "metropolis"}},
		{Name: "prefix-name-county", Fields: map[string]string{"county": "county07"}, Prefix: map[string]string{"surname": "s00"}},
		{Name: "postcode-prefix", Prefix: map[string]string{"postcode": "zz01"}},
		{Name: "empty-town", Fields: map[string]string{"surname": "williams", "given": "jonathan", "town": "absent"}},
		{Name: "business-classification-town", Fields: map[string]string{"kind": "1", "sic": "0042", "town": "metropolis"}},
		{Name: "edition-name-town", Fields: map[string]string{"edition": "1", "surname": "williams", "given": "jonathan", "town": "metropolis"}},
		{Name: "street-county", Fields: map[string]string{"county": "county07"}, Street: "oak lane"},
	}
	selected := row(543)
	report.Cases = append(report.Cases, Case{Name: "rare-positive", Fields: map[string]string{"surname": selected.Surname, "given": selected.Given, "postcode": selected.Postcode}})
	oracle := make([][]int, len(report.Cases))
	for i := 1; i <= *n; i++ {
		r := row(i)
		for j, c := range report.Cases {
			if matches(r, c) {
				oracle[j] = append(oracle[j], i)
			}
		}
	}
	// Compare every page against the independent generator before collecting latency.
	// Bounded by rows; this is not limited to an initial candidate universe.
	for ci, c := range report.Cases {
		for _, e := range engines {
			after := 0
			maxPages := 3
			if c.Name == "rare-positive" || c.Name == "empty-town" || c.Name == "prefix-name-county" {
				maxPages = (*n / 50) + 1
			}
			for page := 0; page < maxPages; page++ {
				ids, _, err := execute(ctx, enginePool(e), e, c, after, 51)
				must(err)
				expected := expectedPage(oracle[ci], after, 51)
				if !equal(ids, expected) {
					panic(fmt.Sprintf("oracle mismatch %s %s page%d got%d want%d", e, c.Name, page, len(ids), len(expected)))
				}
				report.VerifiedPages++
				if len(ids) < 51 {
					report.CompleteWalks++
					break
				}
				after = ids[49]
			}
		}
	}
	fmt.Fprintf(os.Stderr, "oracle verified: %d pages, %d complete walks\n", report.VerifiedPages, report.CompleteWalks)
	// Warm each engine equally; alternate execution order by round/case/page.
	for _, e := range engines {
		for _, c := range report.Cases {
			_, _, err = execute(ctx, enginePool(e), e, c, 0, 51)
			must(err)
		}
	}
	var mu sync.Mutex
	for _, clients := range []int{1, 4} {
		for round := 0; round < *rounds; round++ {
			jobs := make(chan int, len(report.Cases))
			for i := range report.Cases {
				jobs <- i
			}
			close(jobs)
			var wg sync.WaitGroup
			for worker := 0; worker < clients; worker++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for ci := range jobs {
						c := report.Cases[ci]
						afters := map[string]int{}
						done := map[string]bool{}
						for page := 1; page <= 3; page++ {
							for k := range engines {
								e := engines[(k+round+ci+page)%len(engines)]
								if done[e] {
									continue
								}
								ids, ms, err := execute(ctx, enginePool(e), e, c, afters[e], 51)
								s := Sample{Engine: e, Case: c.Name, Clients: clients, Page: page, MS: ms}
								if err != nil {
									s.Error = err.Error()
									done[e] = true
								} else if !equal(ids, expectedPage(oracle[ci], afters[e], 51)) {
									s.Error = "oracle mismatch"
									done[e] = true
								} else if len(ids) < 51 {
									done[e] = true
								} else {
									afters[e] = ids[49]
								}
								mu.Lock()
								report.Samples = append(report.Samples, s)
								if s.Error != "" {
									report.Errors++
								}
								mu.Unlock()
							}
						}
					}
				}()
			}
			wg.Wait()
			fmt.Fprintf(os.Stderr, "timing stage clients=%d round=%d complete\n", clients, round+1)
		}
	}
	if *reuse == "" {
		capturePlans(ctx, enginePool, &report)
	}
	for _, clients := range []int{1, 4} {
		for _, e := range engines {
			var ms []float64
			s := Summary{Engine: e, Clients: clients}
			for _, x := range report.Samples {
				if x.Engine == e && x.Clients == clients {
					ms = append(ms, x.MS)
					s.Requests++
					if x.Error != "" {
						s.Errors++
					}
				}
			}
			sort.Float64s(ms)
			s.P50MS = percentile(ms, .50)
			s.P95MS = percentile(ms, .95)
			s.P99MS = percentile(ms, .99)
			report.Summaries = append(report.Summaries, s)
		}
	}
	data, err := json.MarshalIndent(report, "", "  ")
	must(err)
	must(os.WriteFile(*output, append(data, '\n'), 0644))
	for _, s := range report.Summaries {
		fmt.Fprintf(os.Stderr, "%s c%d n%d errors%d p50=%.2f p95=%.2f p99=%.2fms\n", s.Engine, s.Clients, s.Requests, s.Errors, s.P50MS, s.P95MS, s.P99MS)
	}
	if report.Errors > 0 {
		os.Exit(1)
	}
}
func execute(ctx context.Context, p *pgxpool.Pool, e string, c Case, after, limit int) ([]int, float64, error) {
	q, args := query(e, c, after, limit)
	start := time.Now()
	rows, err := p.Query(ctx, q, args...)
	if err != nil {
		return nil, time.Since(start).Seconds() * 1000, err
	}
	defer rows.Close()
	ids := []int{}
	for rows.Next() {
		var id int
		if err = rows.Scan(&id); err != nil {
			return nil, time.Since(start).Seconds() * 1000, err
		}
		ids = append(ids, id)
	}
	return ids, time.Since(start).Seconds() * 1000, rows.Err()
}
func expectedPage(ids []int, after, limit int) []int {
	i := sort.SearchInts(ids, after+1)
	end := i + limit
	if end > len(ids) {
		end = len(ids)
	}
	return ids[i:end]
}
func equal(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func percentile(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	return v[int(math.Ceil(float64(len(v))*p))-1]
}

// EXPLAIN EXECUTE is deliberate: EXPLAIN of a new parameterized statement alone
// would not expose the generic plan used after prepared-statement reuse.
func capturePlans(ctx context.Context, enginePool func(string) *pgxpool.Pool, r *Report) {
	for _, mode := range []string{"force_custom_plan", "force_generic_plan"} {
		for _, e := range []string{"btree", "gin", "parade", "text"} {
			for _, ci := range []int{0, 2, 7, 10} {
				q, args := query(e, r.Cases[ci], 0, 51)
				conn, err := enginePool(e).Acquire(ctx)
				must(err)
				tx, err := conn.Begin(ctx)
				must(err)
				_, err = tx.Exec(ctx, "SET LOCAL plan_cache_mode="+mode)
				must(err)
				_, err = tx.Exec(ctx, "PREPARE spike_plan AS "+q)
				must(err)
				literals := make([]string, len(args))
				for i, a := range args {
					switch v := a.(type) {
					case int:
						literals[i] = fmt.Sprint(v)
					case string:
						literals[i] = "'" + strings.ReplaceAll(v, "'", "''") + "'"
					default:
						panic("unsupported synthetic parameter")
					}
				}
				var plan []byte
				err = tx.QueryRow(ctx, "EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) EXECUTE spike_plan("+strings.Join(literals, ",")+")").Scan(&plan)
				must(err)
				r.Plans[mode+"/"+e+"/"+r.Cases[ci].Name] = json.RawMessage(plan)
				_, err = tx.Exec(ctx, "DEALLOCATE spike_plan")
				must(err)
				must(tx.Rollback(ctx))
				conn.Release()
			}
		}
	}
}
