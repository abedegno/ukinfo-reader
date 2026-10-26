# All-edition ParadeDB serving trial — 26 October 2026

**The real serving projection passes coverage and compatibility checks, and
ParadeDB performs better in the warmed concurrency runs. It is not ready for a
blanket public switch: the complete paired workload exposes broad-postcode and
simple-name regressions. Keep production native while testing selective routing.**

This tests the actual optional Go API backend over **248,951,107 entries in five
editions**, including businesses. It follows the
[single-edition index spike](real-search-spike-2026-10-26.md), whose simpler
projection, ordering, workload and query budgets differ. These results supersede
that spike as evidence about the serving projection; they do not invalidate its
observations about its own configuration.

- [Aggregate results, retained pilots, plans and resource counters](parade-serving-trial-2026-10-26-results.json)
- [Operator stages and reporting method](../../tools/parade-preview/README.md)
- [Serving configuration, permissions and rollback](../paradedb-preview.md)

No real search inputs, records, record IDs, response hashes or cursor values are
published. Workload and reference files remain private on the NAS. No real queries
were replayed to the public Internet endpoint.

## Coverage and environment

| Edition | Entries | Field samples | Independent-index samples |
|---|---:|---:|---:|
| 2005 SE, PCF175 | 51,350,897 | 256 / 256 | 256 / 256 |
| 2004 SE, PCP218 | 63,373,974 | 256 / 256 | 256 / 256 |
| 1999 SE4, PCF94 | 46,355,850 | 256 / 256 | 256 / 256 |
| 1998 SE, PCF78 | 27,983,545 | 256 / 256 | 256 / 256 |
| 2003 SE, DPCS0104 | 59,886,841 | 256 / 256 | 256 / 256 |

Every edition passed exact count/range, current-generation readiness and index
validity checks. During the copy, every row's name, first name, postcode and kind
were compared with the original record while preserving the source dictionary
IDs. The 1,280 spread samples additionally checked all projected fields against
original compressed records and independent compact/address/research indexes.
Sampling does not independently prove every address field in every copied row.

The private derived database uses PostgreSQL **18.6**, `pg_search` **0.26.3**, Linux
amd64, **2 CPUs / 6 GiB**, 1 GiB shared buffers and 32 MiB work memory. Parallel
query/maintenance workers and JIT are disabled. The checkpointed builder has
1 CPU / 512 MiB. The existing source database retains its 2-CPU / 6-GiB limits.
Each private API has 1 CPU / 512 MiB; all database serving access is SELECT-only.
Neither database nor API trial container publishes a host port.

The candidate adds a second database alongside the source store. This is an
end-to-end architecture comparison, **not an equal-total-resource engine test**.
Timing includes HTTP transfer, JSON validation/hashing and original-record
retrieval/decompression. Cloudflare, Internet latency and browser rendering are
excluded. Existing route deadlines remain: simple searches normally have a
two-second context; address, ordered, nearby and refinement searches can have five
seconds. The source pool defaults to two seconds; address, ordered and nearby
source transactions already override this to four seconds. The optional target
pool has a four-second backstop. Cancellation can
overshoot a deadline; the observed maximum HTTP latency was 5.2 seconds.

All indexing, vacuum and verification finished before final timing. No OS or
source database cache was flushed. Verification and earlier pilots affected
cache state, and unrelated NAS activity was uncontrolled. The first paired run
is an earlier-cache-state observation, **not a controlled cold-cache test**.

## Complete paired workload

The frozen 294-case corpus combines sampled displayed fields with earlier
regressions. It includes 54 business cases, people, partial postcodes, address
fields, ordering, refinements and native fallbacks. Routes comprise 278 searches,
four first-page exports, five record details, three address views and four group
views. This is a targeted regression corpus, not a traffic-weighted user sample.

The two APIs ran sequentially per pair, alternating which went first. Each case
had two rounds, with up to ten pages. This produced **588 fixed first-page
attempts per backend** and **531 later-page attempts per backend**. Later pages
were followed only while both responses matched and both supplied a continuation;
their population therefore excludes searches that failed earlier. All bounds
completed without exhaustion. Errors remain in latency percentiles.

| Population | Native p50 / p95 | ParadeDB p50 / p95 | Native / candidate errors |
|---|---:|---:|---:|
| First pages, 588 each | 44 / 2,949 ms | 53 / 2,001 ms | 25 / 25 |
| Later pages, 531 each | 90 / 1,432 ms | 112 / 515 ms | 1 / 0 |
| All attempts, 1,119 each | 71 / 1,937 ms | 90 / 1,033 ms | 26 / 25 |

All errors were `query_timeout` responses. **There were no content mismatches in
the 1,073 pairs where both requests succeeded.** In 21 pairs only the candidate
succeeded, in 20 only native succeeded, and five failed on both sides. A response
available from only one backend is not a content-equivalence proof.

The failures cluster by query shape:

| Candidate path / query shape | Pairs | Native errors | Candidate errors |
|---|---:|---:|---:|
| Accelerated address-filter searches | 599 | 16 | 0 |
| Accelerated postcode searches without address filters | 306 | 4 | 17 |
| Other accelerated searches | 146 | 0 | 2 |
| Native fallback routes | 68 | 6 | 6 |

The postcode group includes both names with postcodes and broader postcode-only
queries; the latter account for the principal regressions. The two other
accelerated failures were a simple-name case. All **298 business request pairs**
succeeded and matched. Grouping, nearby and edition-order fallbacks still have
native performance limits; the candidate does not accelerate those paths.

## Fixed-cohort concurrency

A fresh calibration matched **287 of 294 first pages**. Seven cases were excluded
because one or both requests failed; calibration recorded three native and six
candidate errors, with no content mismatches. Those exclusions remain represented
in the complete paired workload above. Calibration is not an independent search
oracle: it hashes mutually matching responses for subsequent stability checks.

The four separate runs used the order native → candidate → candidate → native.
Each run attempted every retained case once initially, then exactly twice at one
client and twice at four clients: **1,435 requests per run**. Each concurrency row
below therefore represents the same 574-case-attempt population. A fixed shuffled
order prevents faster runs from covering a different mix of cases. All runs
completed their bounds; successful responses matched their saved references.

| Run | Clients | p50 | p95 | Errors | Observed requests/s |
|---|---:|---:|---:|---:|---:|
| Native first | 1 | 12.1 ms | 210 ms | 0 | 15.8 |
| Native first | 4 | 26.8 ms | 581 ms | 3 | 25.3 |
| Candidate first | 1 | 7.3 ms | 123 ms | 0 | 36.4 |
| Candidate first | 4 | 14.3 ms | 406 ms | 0 | 46.7 |
| Candidate repeat | 1 | 7.4 ms | 105 ms | 0 | 39.6 |
| Candidate repeat | 4 | 14.7 ms | 385 ms | 0 | 48.8 |
| Native repeat | 1 | 12.9 ms | 197 ms | 0 | 18.2 |
| Native repeat | 4 | 68.0 ms | 1,043 ms | 7 | 16.4 |

Including initial passes, native had **11 errors in 2,870 attempts**; candidate had
**none in 2,870**. These are encouraging warmed, closed-loop results on a selected
subset. They neither erase earlier postcode failures nor establish a maximum
public arrival rate, high-concurrency capacity or a latency SLO. No scheduled
arrival-rate test or controlled cold-cache run was performed.

## Build, storage and resource cost

The builder ran from 08:24:23 to 13:41:11 UTC: **5 hours 17 minutes**, exiting
successfully without an OOM event. Earlier pilot work overlapped parts of this
build, so this is observed operational elapsed time, not isolated build throughput.

| Physical database measurement | GiB |
|---|---:|
| Derived tables, including table auxiliary storage | 28.60 |
| Derived indexes, including primary indexes | 19.80 |
| Complete derived database | 48.41 |
| Existing source database, including retained rollback data | 154.12 |

The derived database is additional storage. `pg_database_size` excludes cluster
WAL, other databases, backups and rebuild headroom; 48.41 GiB is not an adequate
whole-deployment disk allocation. The NAS retained about 22 TiB free during the
build, well above the configured 200-GiB stop threshold. No old source or spike
volume was pruned.

Observed CPU-counter deltas across the source database, derived database and both
private APIs were **111.7 / 137.3 CPU-seconds** for the native runs and
**56.0 / 52.7** for the candidate runs. Each ran the same 1,435 requests. These
windows also include idle/background work and controller overhead; they are not
per-query CPU attribution. Source-database CPU fell substantially during candidate
runs, which still load original records there. Resource snapshots include file
cache and do not establish peak memory or a minimum safe RAM allocation.

## Plans and retained failures

Initial 2005 pilots ran during 2004 construction and had errors on both backends.
Their outcome counts and sanitized plans are retained in the aggregate
results. They are diagnostic evidence, not acceptance timings.

A redundant literal-token postcode-prefix prefilter was added while retaining
the original scalar range and district/sector precision predicates. Synthetic
integration checks cover postcode district/sector boundaries and suppressed raw
postcodes. No data generation or index rebuild was needed for this query change.

After the final timing runs, explicit custom/generic prepared plans used Top-K
scans for all captured cases. For one broad 2005 postcode case, the prefilter
reduced logical buffer counters from **22,510 to 10,978** in both plan modes.
Warm plan executions were about 23–24 ms with it and 710–785 ms without it.
Strategies ran in a fixed order and shared caches, so those times are not an
isolated speedup estimate. Logical buffer counts are not physical bytes read.
This improvement did not eliminate earlier-cache-state HTTP postcode timeouts.

## Compatibility, rollback and recommendation

Real cursor checks completed across the full corpus: 273 matching first pages,
51 successful continuation/cross-backend exchanges, no content mismatches and
two correctly rejected changed-query bindings. Some requests timed out during
this stage; their counts are retained. Synthetic integration separately covers
complete walks, sparse refinements, stale/partial generations and reader write
denial with transaction read-only defaults disabled.

The private candidate was then restarted with native search **while the derived
database was stopped**. All **287 reference-hashed requests passed**. Public API
and source containers remained healthy with unchanged start times and zero
restarts. Source visibility and read-only permissions were unchanged, and the
hidden 59.9-million-entry rollback edition was preserved. All trial services were
stopped afterward; the derived volume was retained. No public route or backend
was changed.

The next experiment should use **selective routing**, beginning with the address
filters that avoided 16 native failures here. Retain native handling for broad
postcode-only and simple-name searches, then compare the selector over the entire
frozen corpus, including its excluded load cases. Routing should happen before
execution, without retrying a failed expensive request through a second backend.

Before a public switch, also establish reproducible cold/evicted-cache behavior,
backup/restore and extension-upgrade recovery, a higher-concurrency/arrival-rate
test, and distribution requirements. Desktop packaging remains a separate gate.
The completed trial establishes a useful accelerator and a working rollback;
it does not justify changing every query to ParadeDB.
