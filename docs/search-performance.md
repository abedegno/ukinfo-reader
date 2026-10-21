# Search performance checks

## Required CI gates

The required `search` job runs complementary matrices and retains the
`search-performance` JSON artifact for 30 days:

| Layer | Fixture and coverage | What fails CI |
| --- | --- | --- |
| PostgreSQL candidate plans | 50,000 invented entries in two editions; 70% dominant surname, rare names, 1,970 first-name terms, 727 postcode terms, 5% businesses; full postcode, sector/district, given-name variants, all editions and page-two cursors | Results/order differ from a sequential reference; any candidate heap fetch or non-index-only scan; 1,500 or more shared buffer accesses per plan |
| Business classification probes | 50,000 invented business projections; a broad industry prefix and 50 selected postcode records; actual API predicates under forced custom/generic plans | Wrong IDs/order; classification range used as the per-record index condition; non-index-only research access or heap fetches; 800 or more shared buffers |
| Search API | 22,546 invented entries in three editions; selective/broad/scattered names, given-name/wildcard/middle initial, street/town/county/house, full/partial postcode, nearby district centre, businesses, grouped/ordered results and complete pagination | Wrong counts, duplicate pages, wrong address/strategy, HTTP errors, truncated or endless pagination |

The SQL matrix exercises forced custom and generic prepared plans and validates
concurrent index creation, safe reapplication and incompatible-index rejection.
It VACUUMs the disposable fixture before checking heap-free scans. These hosted
covering indexes are optional in desktop imports; API tests also exercise the
compact default indexes. The generic plan check deliberately leaves PostgreSQL's
scan choices enabled; it does not force the desired index.

The JSON records SQL execution time, buffer reads/accesses and heap fetches, plus
API first-page and slowest subsequent-page times. API measurements include the
handler, record decoding and JSON serialization, exclude HTTP transport, and may
include Go race instrumentation. CI latency is trend evidence, not a fixed
millisecond gate on shared runners. Work budgets and correctness are hard gates.
The API artifact contains only explicitly invented fixture queries, never live
searches. Case numbers follow the table in `selective_address_test.go`.

Run locally with Docker and a disposable PostgreSQL database:

```sh
UKINFO_PLAN_REPORT=/tmp/ukinfo-performance/plans.json \
  python3 tools/check_covering_search_indexes.py
cd server
# Set UKINFO_TEST_DATABASE_URL to a disposable PostgreSQL 18 database.
UKINFO_PERFORMANCE_REPORT_DIR=/tmp/ukinfo-performance \
  go test -race -count=1 ./internal/catalog
```

## HDD and cache behaviour

Small fixtures fit in memory. Their results do not establish whole-catalogue
latency, Internet concurrency or cold HDD performance. For the name/postcode fix,
production validation included four partition index-only scans with zero heap
fetches, and an observation with 422 shared reads completed in about 1.13 seconds.
Repeated API checks were about 0.1 seconds. Those are observations of one reported
search, not capacity guarantees or a controlled fully cold run.

Never clear production PostgreSQL/OS caches or restart the database to manufacture
a cold run. On an isolated restored copy, record first observations separately
from repeats and use a workload larger than the combined cache. Record hardware,
corpus size, indexes, PostgreSQL settings, vacuum/visibility coverage, quotas,
concurrency, versions and failures alongside latency. PostgreSQL shared reads may
still be OS-cache hits; do not label them physical HDD reads.

`tools/benchmark_plans.py` can collect bounded plan evidence using `psql` and a
trusted operator-owned JSON array of SELECT strings under `private/`. Supply
`DATABASE_URL` in the environment, preferably using the serving read-only role:

```sh
python3 tools/benchmark_plans.py --cases private/plan-cases.json \
  --output private/plan-observations.json --repeats 3 --timeout-ms 2000
```

The tool uses read-only transactions and a 100 ms lock timeout. It runs one query
at a time, exits unsuccessfully if any observation fails, never changes cache state,
and emits only case numbers, timing and aggregate work. PostgreSQL error details
and SQL stay out of console output. The cases are trusted SQL, not an upload format
or public endpoint. Keep query files and full plans private.

Inspect shared buffer reads, heap fetches, rows removed by filters, spills and
sequential scans before adding indexes. A sequential scan can be appropriate for
broad searches; do not globally disable it. Check visibility-map coverage and
VACUUM after logical restores before concluding a covering index has regressed.

For full HTTP throughput, use [the load generator](../tools/search-load/README.md)
with independently verified private response hashes. Include errors, drops and
scheduled-arrival latency; do not run an unbounded load test against the public NAS.
The [observability dashboard](observability.md) complements these tests with
production latency, failures and pool waits without recording search values.

## Full-catalogue profiling

The operator-only profiler runs a private workload through the actual API handler
using a read-only pool and the usual request/statement deadlines:

```sh
cd server
# DATABASE_URL must be supplied privately, preferably using the serving role.
go run ./cmd/ukinfo-profile-search --cases ../private/profile-cases.json \
  --output ../private/profile-report.json --max-cases 20 --max-duration 5m
```

Cases use the HTTP runner's `path` and `query` fields. The profiler exports case
numbers, HTTP status, end-to-end and per-query timings, fixed stages, and aggregate
API metrics. Slow/failed SELECTs receive **EXPLAIN without ANALYZE**, so profiling
does not execute them twice. Plans retain node/index names, estimates and work
counters only; predicates, output expressions, SQL, parameters and response records
are omitted. Estimated plans are not evidence of actual rows, disk reads or spills.
The default slow threshold is 100 ms. Limit workloads to 100 cases and 15 minutes.
The profiler retains failed requests and continues; inspect `failed_requests`.
Use the HTTP runner when failures must also fail an availability gate.

The API exposes `ukinfo_search_phase_duration_seconds{phase,outcome}` for vocabulary,
source/address probes, address blocks/dictionary, candidates, ordered candidates,
group members and record loading. Labels come from fixed allowlists. A phase may
occur more than once per request or edition; phase histograms are not request
latency distributions. Query phases include row consumption and transaction/pool
cleanup; record phases also include decoding and manifest reads. OTel spans use
the same fixed stage names and never include search values.

Hosted dictionary covering indexes support both prefix-to-ID expansion and
ID-to-label resolution during sorting/grouping. The concurrent build script checks
both index shapes before accepting an existing managed name. CI verifies both
lookup directions under custom and generic plans, zero fixture heap fetches,
result ordering and bounded shared-buffer work.

### Concurrent index recovery

The build script uses a five-second lock timeout and refuses an existing invalid
index. A concurrent build can finish scanning yet time out while waiting for old
snapshots; long import or verification transactions can cause that wait. Inspect
`pg_stat_progress_create_index`, snapshot ages and `pg_index.indisvalid/indisready`
before retrying. An invalid index is not a successfully deployed optimization.

For the specifically verified, correctly shaped invalid dictionary index, an
operator can use `REINDEX INDEX CONCURRENTLY public.nas_terms_id_key_cover` in a
maintenance session with a suitably bounded statement timeout. Its lock-wait
budget must allow existing snapshots to finish. Keep serving-role and request
budgets unchanged, and monitor the operation. Do not drop source constraints,
terminate imports or silently accept an invalid managed name to bypass the guard.
See PostgreSQL's [REINDEX documentation](https://www.postgresql.org/docs/18/sql-reindex.html)
and [progress phases](https://www.postgresql.org/docs/18/progress-reporting.html#CREATE-INDEX-PROGRESS-REPORTING).

Name-ordered pages enumerate eligible name/first-name pairs, then expand only the
pairs needed for a complete page plus lookahead. This applies to source-name and
postcode anchors. The API suite traverses 360 invented pairs, inserts unused
interleaved dictionary terms, and verifies both grouped and ordinary pages. A
one-item page must expand only its two needed pairs, rather than all 128 candidates.
Anchored business filters seek one record's classification before applying a
national industry/SIC prefix; classification-only searches keep range access.

### Prepare an imported hosted edition

After import and full verification transactions release their snapshots, run the
owner-only maintenance script before publishing the edition:

```sh
psql "$DATABASE_URL" -v dataset='exact-complete-edition-key' \
  -f deploy/production/prepare-search-catalogue.sql
```

The script checks one complete edition's source/research partitions, then runs
ordinary VACUUM/ANALYZE on those partitions and shared dictionary/address lookups.
It uses bounded maintenance memory, lock waits, statement time and vacuum cost.
It requires the complete hosted research/address projections; it is not a desktop
import prerequisite. Inspect visibility coverage after completion. A valid
covering index alone does not guarantee zero heap reads while snapshots prevent
pages becoming all-visible. Schedule backups and index builds separately from
representative interactive latency measurements.
