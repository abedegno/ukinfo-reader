# Existing native versus ParadeDB SIC routes — 9 November 2026

**Keep the existing ParadeDB routing.** On the three previously identified slow
SIC shapes, the native business projection was slower and produced nine HTTP 503
failures in 66 searches. ParadeDB matched all 66 frozen responses with no errors.
All nine page pairs matched. This diagnostic does not qualify a release, the new
index layout, all editions or v14 publication. Production configuration is unchanged.

## Fixed protocol and resources

The [earlier SIC investigation](2003-sic-tail-2026-11-09.md) selected one saved
case from each leading slow shape: SIC prefix+town in catalogue order, SIC prefix
in name order, and exact SIC in name order. Selection was not repeated. Queries,
original hashes and cursor-neutral digest mode were unchanged.

Run native → ParadeDB → ParadeDB → native, with a fresh private API per arm.
All use the same street-only b485487 executable in pinned image
`sha256:ecf25b2b925bba6aa580e127e822dc4774dc653d1585f021b05b3beba5b36ed2`;
only `--search-backend` varies. This binary does not contain the rejected SIC
predicate. It is not the deployed executable: its street predicate differs, but
none of the three selected cases contains a street filter.

Each arm makes three initial requests, then five fixed shuffled rounds at C1 and
C4 (15 requests per stage): 33 load requests, 132 total. The first native and
ParadeDB arms then independently walk three pages for every case, up to 18 more
requests. Their own cursors are used; opaque cursor tokens are not compared.
All 150 budgeted requests completed, with failures retained rather than retried.

Read-only preflight verified 59,886,841 source entries, the complete 2003 derived
generation and pg_search 0.26.3. Both existing databases kept their normal 2 CPU /
6 GiB limits; images, start times and limits were verified unchanged before each
arm and after the comparison. API .5 CPU/512 MiB; client .25 CPU/128 MiB. No DB
restart, cache flush, warmup, SQL deadline change, index build or owner role.
The API budget is below the public API's 1 CPU limit.

## Observations

Errors are included in the latency statistics. With only 15 observations per
stage, this runner's nearest-rank p95 equals the maximum; these are small-sample
observations, not a stable tail-latency estimate.

| Arm | Initial errors / 3 | C1 p95 / 15 | C4 p95 / 15 | C4 errors / 15 |
| --- | ---: | ---: | ---: | ---: |
| Native A1 | 2 | 2,978 ms | 4,567 ms | 3 |
| ParadeDB B1 | 0 | 202 ms | 405 ms | 0 |
| ParadeDB B2 | 0 | 220 ms | 405 ms | 0 |
| Native A2 | 0 | 2,811 ms | 4,648 ms | 4 |

Native's initial failures explicitly returned `query_timeout`; concurrent failures
are retained as HTTP 503 in the aggregate load reports. All 57 successful native
responses and all 66 ParadeDB responses matched their frozen hashes. No hash
mismatch, unverified result or exhausted budget. Native C1 had no errors.

Separate load counters verified exactly 33 `business_projection` decisions in
each native arm and exactly 33 `paradedb` decisions in each candidate arm, with
no ParadeDB fallback. Pagination counters independently recorded nine decisions
per route. This compares the existing native projection, not an unindexed scan.

All three cases returned three full 50-row pages per route. Every native/
ParadeDB page pair matched full content, ordering and next-page availability;
each query's 150 rows were distinct. This is nine matching page pairs, 450 rows
per route, with another page available each time. It is a bounded walk, not
exhaustive enumeration or a real-data cross-route cursor exchange test.

The diagnostic completed, but its combined correctness/route flag remains false
because native requests failed. The page flag is true. Do not relabel the whole
comparison as passing merely because ParadeDB succeeded.

## Decision and limits

There is no evidence here for routing these SIC searches back to native. The
native concurrent failures persist in the final arm after both databases have
been exercised. However, shared HDD/cache/host load and different index formats
remain uncontrolled. ParadeDB's initial-request maximum was 1,598 ms; warm figures must not
hide that observation. No causal engine-capacity ratio or cold-storage SLA follows.

This also does not explain the earlier private-layout C4 failure: that experiment
used a different index layout with 1.25 CPU/1,408 MiB for PostgreSQL. This run used
the existing original derived target at 2 CPU/6 GiB. Layout, memory, CPU and cache
effects cannot be separated from these different experiments.

Next, establish the complete frozen 127-case 2003 workload on the existing
derived target at unchanged operating limits. A subsequent layout comparison
must match those budgets before choosing another runtime or index change.
Earlier failed HTTP, baseline-page and v14 gates remain unchanged; PR #48 stays
draft and nothing is deployed.

## Reproduction and cleanup

Homelab contains `docker/ukinfo/native-sic-inventory/` and
`docker/ukinfo/native-sic-compare/`, with frozen protocol, controllers and safe
numeric evidence. Independent review fixed incremental error retention, separate
load/page counters and explicit route checks before launch; no launch blocker
remained. Five Python files parse successfully; no application code changed.

NAS roots under `/home/jonw/ukinfo-backups/`:

- `native-sic-inventory-2026-11-09`: read-only roles/readiness preflight.
- `native-sic-compare-2026-11-09`: sealed protocol, four load reports, incremental
  page reports/raw private responses, route counters, final summary and closeout.

Queries, records, IDs, cursors, data hashes and raw error text stay on NAS. No
local Docker image/database copy or build was made. Fresh checks verified all
ten owned API/client containers stopped, lease released, public health OK and
serving image unchanged. Existing databases were never stopped or updated.
