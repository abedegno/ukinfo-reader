package catalog

// Operator-only experiment; no HTTP route or serving-format switch is installed.
import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ProjectionCase struct {
	Query string `json:"query"`
}
type ProjectionObservation struct {
	Case                 int     `json:"case"`
	Family               string  `json:"family"`
	Round                int     `json:"round"`
	ProjectionFirst      bool    `json:"projection_first"`
	Page                 int     `json:"page"`
	BaselineMS           float64 `json:"baseline_ms"`
	ProjectionMS         float64 `json:"projection_ms"`
	BaselineRecordsMS    float64 `json:"baseline_records_ms"`
	ProjectionRecordsMS  float64 `json:"projection_records_ms"`
	BaselineIdentityMS   float64 `json:"baseline_identity_ms"`
	ProjectionIdentityMS float64 `json:"projection_identity_ms"`
	BaselineOutcome      string  `json:"baseline_outcome"`
	ProjectionOutcome    string  `json:"projection_outcome"`
	Equal                bool    `json:"equal"`
	Rows                 int     `json:"rows"`
}
type ProjectionReport struct {
	Observations []ProjectionObservation `json:"observations"`
	Complete     bool                    `json:"complete"`
	Failures     int                     `json:"failures"`
	Mismatches   int                     `json:"mismatches"`
}
type projectionRow struct {
	ID    int32
	Group string
}
type projectionRead struct {
	rows                           []projectionRow
	digest                         [32]byte
	queryMS, recordsMS, identityMS float64
	outcome                        string
}

// BenchmarkProjection compares decomposed SQL with a full-edition joined search
// projection, then uses the same production block decoder for both. It is not an
// HTTP capacity test. Raw query values, records, IDs and digests stay in memory.
func BenchmarkProjection(ctx context.Context, db *pgxpool.Pool, cases []ProjectionCase, dataset string, rounds, pages int) (ProjectionReport, error) {
	return benchmarkProjection(ctx, db, cases, dataset, rounds, pages, false)
}

// BenchmarkBusinessProjection is restricted to classified business searches. Its
// smaller private table deliberately omits people and general name indexes.
func BenchmarkBusinessProjection(ctx context.Context, db *pgxpool.Pool, cases []ProjectionCase, dataset string, rounds, pages int) (ProjectionReport, error) {
	return benchmarkProjection(ctx, db, cases, dataset, rounds, pages, true)
}

func benchmarkProjection(ctx context.Context, db *pgxpool.Pool, cases []ProjectionCase, dataset string, rounds, pages int, businessOnly bool) (ProjectionReport, error) {
	schema := "ukinfo_projection_spike"
	if businessOnly {
		schema = "ukinfo_business_spike"
	}
	report := ProjectionReport{Observations: []ProjectionObservation{}, Complete: true}
	if len(cases) < 1 || len(cases) > 100 || rounds < 1 || rounds > 4 || pages < 1 || pages > 10 {
		return report, fmt.Errorf("invalid projection workload budget")
	}
	a := NewAPI(db)
	var edition int16
	var complete bool
	if err := db.QueryRow(ctx, `SELECT d.id,b.finished_at IS NOT NULL AND b.expected=d.entries
 FROM datasets d JOIN `+schema+`.build b ON b.dataset_id=d.id WHERE d.key=$1`, dataset).Scan(&edition, &complete); err != nil || !complete {
		return report, fmt.Errorf("complete projection unavailable")
	}
	for round := 0; round < rounds; round++ {
		for index, c := range cases {
			values, err := url.ParseQuery(c.Query)
			if err != nil {
				return report, fmt.Errorf("invalid private case %d", index)
			}
			values.Set("dataset", dataset)
			values.Del("cursor")
			values.Set("limit", "50")
			q, err := parse(values)
			if err != nil {
				return report, fmt.Errorf("unsupported private case %d", index)
			}
			if businessOnly && (q.Kind != "business" || !q.hasBusinessFilters()) {
				return report, fmt.Errorf("business-only projection requires classification filters in case %d", index)
			}
			if q.Near != "" || q.hasRefinements() || q.Group != "" || q.Sort != "" {
				return report, fmt.Errorf("unsupported private case %d", index)
			}
			family := "name"
			if q.Postcode != "" {
				family = "postcode"
			}
			if q.hasAddress() {
				family = "address"
			}
			if q.hasBusinessFilters() {
				family = "classification"
				if q.hasAddress() {
					family = "classification_address"
				}
			}
			plain := q
			plain.Street = ""
			plain.Locality = ""
			plain.Town = ""
			plain.County = ""
			plain.SIC = ""
			plain.Industry = ""
			f, status, err := a.buildCandidateFilters(ctx, plain)
			if err != nil || status != 200 {
				return report, fmt.Errorf("filter setup failed for case %d", index)
			}
			args := append([]any(nil), f.args...)
			conditions := append([]string(nil), f.conditions...)
			columns := regexp.MustCompile(`\b(dataset_id|kind|name_id|first_id|postcode_id)\b`)
			for i := range conditions {
				conditions[i] = columns.ReplaceAllString(conditions[i], "entries.$1")
			}
			for _, filter := range []struct{ column, value string }{{"sic", q.SIC}, {"industry", q.Industry}} {
				if filter.value == "" {
					continue
				}
				base := strings.TrimSuffix(filter.value, "*")
				// Match the API's explicit partial-index predicate. In a generic
				// plan, a bound equality/range alone cannot prove nonempty text.
				conditions = append(conditions, "entries."+filter.column+"<>''")
				args = append(args, base)
				condition := fmt.Sprintf("entries.%s=$%d", filter.column, len(args))
				if strings.HasSuffix(filter.value, "*") {
					args = append(args, base+"\U0010ffff")
					condition = fmt.Sprintf("entries.%s >= $%d AND entries.%s < $%d", filter.column, len(args)-1, filter.column, len(args))
				}
				conditions = append(conditions, condition)
			}
			// Baseline retains independently stored keys, classifications and reverse arrays.
			baselineFrom := `entries`
			if q.hasBusinessFilters() {
				baselineFrom += ` JOIN research_keys r USING(dataset_id,id)`
			}
			projectionFrom := schema + `.entries entries`
			if q.hasAddress() {
				predicate := addressPredicate(q, &args)
				conditions = append(conditions, predicate)
				baselineFrom += ` JOIN address_record_blocks b ON b.dataset_id=entries.dataset_id AND b.block_id=(entries.id-1)/128 JOIN address_locations l ON l.id=b.location_ids[(entries.id-1)%128+1]`
				projectionFrom += ` JOIN address_locations l ON l.id=entries.location_id`
			}
			args = append(args, int32(0))
			afterArg := len(args)
			conditions = append(conditions, fmt.Sprintf("entries.id>$%d", afterArg))
			where := strings.Join(conditions, " AND ")
			baselineWhere := strings.ReplaceAll(strings.ReplaceAll(where, "entries.sic", "r.sic"), "entries.industry", "r.industry")

			// Establish the eligible set before source-ID ordering. Otherwise an
			// order-driven PK walk can scan a national corpus for an absent class.
			querySQL := func(from, where string) string {
				ctes := append([]string(nil), f.termCTEs...)
				ctes = append(ctes, "projection_eligible AS MATERIALIZED (SELECT entries.id FROM "+from+" WHERE "+where+")")
				return "WITH " + strings.Join(ctes, ",") + " SELECT id FROM projection_eligible ORDER BY id LIMIT 51"
			}
			baseline := querySQL(baselineFrom, baselineWhere)
			projection := querySQL(projectionFrom, where)
			for page := 0; page < pages; page++ {
				if ctx.Err() != nil {
					report.Complete = false
					return report, nil
				}
				first := (round+index+page)%2 == 0
				var before, after projectionRead
				read := func(sql string, projected bool) projectionRead {
					readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
					defer cancel()
					r := projectionRead{outcome: "query_error"}
					start := time.Now()
					rows, err := db.Query(readCtx, sql, args...)
					if err == nil {
						for rows.Next() {
							var id int32
							if err = rows.Scan(&id); err != nil {
								break
							}
							r.rows = append(r.rows, projectionRow{ID: id})
						}
						if err == nil {
							err = rows.Err()
						}
						rows.Close()
					}
					r.queryMS = float64(time.Since(start)) / float64(time.Millisecond)
					if err != nil {
						if readCtx.Err() != nil {
							r.outcome = "timeout"
						}
						return r
					}
					ids := make([]int32, min(50, len(r.rows)))
					for i := range ids {
						ids[i] = r.rows[i].ID
					}
					start = time.Now()
					records, err := a.loadRecords(readCtx, edition, ids)
					r.recordsMS = float64(time.Since(start)) / float64(time.Millisecond)
					if err != nil {
						r.outcome = "record_error"
						if readCtx.Err() != nil {
							r.outcome = "timeout"
						}
						return r
					}
					raw, err := json.Marshal(records)
					if err != nil {
						r.outcome = "record_error"
						return r
					}
					r.digest = sha256.Sum256(raw)
					// Identity validation is a bounded, separate read after the timed
					// search/record path. Ordinary search does not request group keys;
					// selecting them before LIMIT would force unnecessary heap reads.
					if len(r.rows) > 0 {
						allIDs := make([]int32, len(r.rows))
						for i := range allIDs {
							allIDs[i] = r.rows[i].ID
						}
						identityCtx, done := context.WithTimeout(ctx, 4*time.Second)
						defer done()
						identitySQL := "SELECT id,group_key FROM research_keys WHERE dataset_id=$1 AND id=ANY($2::integer[])"
						if projected {
							identitySQL = "SELECT id,group_key FROM " + schema + ".entries WHERE dataset_id=$1 AND id=ANY($2::integer[])"
						}
						start = time.Now()
						groups := map[int32]string{}
						groupRows, e := db.Query(identityCtx, identitySQL, edition, allIDs)
						if e == nil {
							for groupRows.Next() {
								var id int32
								var key []byte
								if e = groupRows.Scan(&id, &key); e != nil {
									break
								}
								if len(key) != 32 {
									e = fmt.Errorf("invalid identity")
									break
								}
								groups[id] = string(key)
							}
							if e == nil {
								e = groupRows.Err()
							}
							groupRows.Close()
						}
						r.identityMS = float64(time.Since(start)) / float64(time.Millisecond)
						if e != nil || len(groups) != len(r.rows) {
							r.outcome = "identity_error"
							return r
						}
						for i := range r.rows {
							key, ok := groups[r.rows[i].ID]
							if !ok {
								r.outcome = "identity_error"
								return r
							}
							r.rows[i].Group = key
						}
					}
					r.outcome = "ok"
					return r
				}
				if first {
					after = read(projection, true)
					before = read(baseline, false)
				} else {
					before = read(baseline, false)
					after = read(projection, true)
				}
				equal := before.outcome == "ok" && after.outcome == "ok" && slices.Equal(before.rows, after.rows) && before.digest == after.digest
				observation := ProjectionObservation{Case: index, Family: family, Round: round + 1, ProjectionFirst: first, Page: page + 1,
					BaselineMS: before.queryMS, ProjectionMS: after.queryMS, BaselineRecordsMS: before.recordsMS, ProjectionRecordsMS: after.recordsMS,
					BaselineIdentityMS: before.identityMS, ProjectionIdentityMS: after.identityMS,
					BaselineOutcome: before.outcome, ProjectionOutcome: after.outcome, Equal: equal, Rows: min(50, len(after.rows))}
				report.Observations = append(report.Observations, observation)
				if before.outcome != "ok" || after.outcome != "ok" {
					report.Failures++
					break
				}
				if !equal {
					report.Mismatches++
					break
				}
				if len(after.rows) < 51 {
					break
				}
				args[afterArg-1] = after.rows[49].ID
			}
		}
	}
	return report, nil
}
