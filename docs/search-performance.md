# Search performance checks

## Required CI gates

The required `search` job runs two complementary matrices and retains the
`search-performance` JSON artifact for 30 days:

| Layer | Fixture and coverage | What fails CI |
| --- | --- | --- |
| PostgreSQL candidate plans | 50,000 invented entries in two editions; 70% dominant surname, rare names, 1,970 first-name terms, 727 postcode terms, 5% businesses; full postcode, sector/district, given-name variants, all editions and page-two cursors | Results/order differ from a sequential reference; any candidate heap fetch or non-index-only scan; 1,500 or more shared buffer accesses per plan |
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
