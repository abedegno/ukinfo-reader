# Hybrid routing and cold-cache tests — 26 October 2026

**The address-only selector improves the complete workload and warmed load in
this trial. Cold storage remains a practical limit: some successful requests take
one to four seconds. At this trial’s conclusion, the public service remained native.**

The later [equal-budget control and hosted rollout](parade-rollout-2026-10-26.md)
supersedes the rollout recommendations below; recovery exercises were explicitly
skipped at the operator’s request.

This follows the [all-edition serving trial](parade-serving-trial-2026-10-26.md).
The new policy selects ParadeDB for street/locality/town/county filters, retaining
native execution for names and postcodes without address filters, classification
alone, grouped/nearby queries and edition ordering. Selection occurs before target
inventory or execution; failures are never retried on a second backend. Native
and hybrid use the same executable and source copy, differing by the serving flag.

- [Aggregate results, including failures, cache measurements and individual checks](parade-hybrid-trial-2026-10-26-results.json)
- [Operator stages and cache methodology](../../tools/parade-preview/README.md)
- [Serving policy and configuration](../paradedb-preview.md)

## Data, isolation and limits

The source copy contains the same five serving-visible editions and
**248,951,107 entries**. All generations, exact coverage and **1,280 spread field
samples** passed verification against original records and independent compact,
address and research indexes. The original 294-case workload was copied NAS to
NAS and checked byte-for-byte without exporting its values or hashes. No case
excluded from earlier load calibration was removed from the full comparisons.

The source was copied with streamed-WAL
[`pg_basebackup`](https://www.postgresql.org/docs/18/app-pgbasebackup.html), then
fully checked by
[`pg_verifybackup`](https://www.postgresql.org/docs/18/app-pgverifybackup.html).
The clone preserves source image, collation and planner settings: PostgreSQL 18.6,
2 CPUs / 6 GiB, 1-GiB shared buffers, 8-MiB work memory and one parallel worker per
gather. Autovacuum and archiving were disabled on the copy. Its files are separate
from the public source. Both private APIs read the clone with the restricted role.
The retained derived database is PostgreSQL 18.6 / pg_search 0.26.3, 2 CPUs / 6 GiB,
32-MiB work memory and parallel query disabled. Each API has 1 CPU / 512 MiB.

**This compares end-to-end configurations, not engines with equal resources or
equal SQL budgets.** Hybrid adds a derived database. Existing limits were retained:
ordinary simple HTTP searches have a two-second context; address, ordered, nearby
and refinement searches can have five seconds. The source pool defaults to two
seconds, but address, ordered and nearby source transactions already override this
to four seconds. The derived pool has a four-second backstop. Thus the main
address/ordered statements already had equal backstops; source metadata,
vocabulary and record reads retained the shorter pool default. A lower failure
count cannot be attributed exclusively to the index implementation.
Cancellation can also overshoot a statement deadline. No timeout was raised for
this selector or experiment.

Timings include HTTP transfer, validation/hashing and original-record retrieval.
They exclude Cloudflare, Internet latency, browser rendering and instance startup.
Other NAS activity and physical drive/controller caches remain uncontrolled.
Source-copy checks and field verification finished before timing.

## Complete paired comparison

Each of the **294 cases** ran twice, alternating backend order, with up to ten
pages. There were **588 fixed first-page attempts per backend**, plus **746 later
pages**. Later traversal stops when either side fails or has no continuation;
those pages are a selected population. All bounds completed.

| Population | Native p50 / p95 | Hybrid p50 / p95 | Native / hybrid timeouts |
|---|---:|---:|---:|
| First pages, 588 each | 26 / 1,725 ms | 18 / 902 ms | 16 / 7 |
| Later pages, 746 each | 78 / 630 ms | 80 / 615 ms | 2 / 0 |
| All attempts, 1,334 each | 54 / 790 ms | 47 / 671 ms | 18 / 7 |

There were **no content mismatches in 1,313 mutually successful pairs**. Fourteen
pairs succeeded only on hybrid, three only on native, and four failed on both.
One-sided success does not prove result equivalence for that pair.

The selector classified 172 cases as eligible for ParadeDB and 122 as native.
Across **636 eligible address request pairs**, native had 12 timeouts and hybrid
had none. The other 698 pairs had six native versus seven hybrid timeouts; both
sides use native execution for these shapes. Cache, scheduling and process state
can therefore still change outcomes even when the selected algorithm is the same.

More later pages were reached than in the previous all-query ParadeDB trial.
Compare the fixed first-page population rather than treating the old and new
all-page latency distributions as identical cohorts. This trial also uses a
physical source copy and a different cache history, so cross-trial improvements
are not isolated causal measurements of the selector alone.

## Warmed fixed-cohort load

Calibration matched **291 of 294 first pages**, excluding ordinals 91, 92 and 212
because native or both sides failed. All three remain in the complete paired and
cold-start cohorts. Load references stay private on the NAS.

Runs used native → hybrid → hybrid → native order. Every run attempted each
retained case once initially and twice at each of one/four clients: **1,455
requests per run**. Each concurrency row below has the same 582 attempts. Fixed
shuffling preserves case frequency; errors remain in percentiles and throughput.

| Run | Clients | p50 | p95 | Errors | Observed requests/s |
|---|---:|---:|---:|---:|---:|
| Native first | 1 | 12.3 ms | 223 ms | 0 | 15.8 |
| Native first | 4 | 25.4 ms | 524 ms | 4 | 27.7 |
| Hybrid first | 1 | 5.8 ms | 102 ms | 0 | 31.5 |
| Hybrid first | 4 | 10.1 ms | 236 ms | 1 | 54.5 |
| Hybrid repeat | 1 | 5.6 ms | 101 ms | 0 | 29.5 |
| Hybrid repeat | 4 | 8.2 ms | 221 ms | 1 | 59.1 |
| Native repeat | 1 | 12.1 ms | 226 ms | 0 | 17.5 |
| Native repeat | 4 | 23.7 ms | 522 ms | 4 | 28.7 |

Including initial passes, native had **nine errors in 2,910 attempts** and hybrid
had **two in 2,910**. Both hybrid errors were native-routed cases; no accelerated
address request failed. Every successful response matched its reference. These
selected, warmed, closed-loop results do not establish an arrival-rate capacity
or public-service latency SLO.

## Cold state actually measured

Restarting PostgreSQL alone does not clear its files from the OS cache. Before
each cold run the private instances were cleanly stopped. A Linux operator tool
then synchronized individual owned files, requested file-scoped eviction with
[`POSIX_FADV_DONTNEED`](https://man7.org/linux/man-pages/man2/posix_fadvise.2.html),
and measured page residency with
[`mincore`](https://man7.org/linux/man-pages/man2/mincore.2.html), using bounded
64-MiB mappings. Data files were mounted read-only, jobs had no network or
capabilities, and file ownership was required. Symlinks, hard links, RAM-backed
filesystems and surviving server lock files were rejected.

An invented-file NAS test passed before data eviction. **All 56 eviction reports
measured zero resident file pages afterward**, exceeding the configured gate of
at most 1 MiB. Original source files and host-wide cache controls were never used.
The public database was not stopped. Startup and health checks subsequently warm
some catalogue pages; filesystem metadata, program/library pages and hardware
caches are outside this measured boundary. Residency is a point-in-time snapshot.

### Complete cold-start batches

Each backend independently began a full 294-case first-page batch after eviction.
The repeat reversed both corpus and backend order. A batch warms as it proceeds;
this does **not** mean every request within it is individually cold.

| Cold-start batch | Native p50 / p95 | Hybrid p50 / p95 | Native / hybrid timeouts |
|---|---:|---:|---:|
| A, forward corpus | 70 / 3,118 ms | 89 / 2,504 ms | 9 / 6 |
| B, reverse corpus | 68 / 3,683 ms | 86 / 2,630 ms | 9 / 4 |

Across 588 attempts per backend, native had **18 timeouts**, hybrid **10**. The
566 mutually successful pairs matched. Twelve pairs succeeded only on hybrid,
four only on native, and six failed on both. All hybrid failures were on native
paths; no accelerated address request failed in either batch. Cold medians were
higher on hybrid despite fewer failures and lower p95: this is not a uniform win.

### Independently cold regression queries

Twelve prior-run ordinals were declared before new timing. Both private data
directories were reset again **before every individual backend request**, and
backend order alternated. Values remain private; edition and query-shape labels
are sufficient to describe the checks. Each row is one attempt per backend,
not a distribution or a repeated latency estimate.

| Ordinal | Shape | Native | Hybrid |
|---|---|---:|---:|
| 8 | 2005 postcode without address | 1,046 ms | 1,224 ms |
| 10 | All-edition postcode without address | 1,880 ms | 1,876 ms |
| 55 | 2004 postcode without address | 1,198 ms | 1,430 ms |
| 58 | 2004 address filter | Timeout | 1,862 ms |
| 89 | 1999 address, alphabetical order | Timeout | 1,967 ms |
| 98 | All-edition postcode without address | 1,803 ms | 1,853 ms |
| 112 | 1999 simple name | 1,105 ms | 1,133 ms |
| 138 | 1998 postcode without address | 1,070 ms | 1,195 ms |
| 165 | 2003 postcode without address | 1,136 ms | 1,254 ms |
| 167 | 2003 address filter | Timeout | 1,545 ms |
| 209 | 2003 address regression | Timeout | 1,540 ms |
| 212 | 2003 address, alphabetical order | Timeout | 4,030 ms |

All 12 hybrid requests succeeded. Native timed out on five address requests; the
seven mutually successful pairs matched. Ordinal 212 is an important limitation:
hybrid completed rather than timing out, but **four seconds is still slow**.
The source/derived SQL budget difference above also matters when interpreting
these failures. This is not proof that equal-budget native execution could never
complete them.

## Compatibility and final state

Real cursor checks recorded 282 matching first pages, 60 successful continuations
and cross-backend exchanges, no content mismatches and two correctly rejected
changed-query bindings. Errors are retained in the aggregate report. Local race
tests and pinned-extension integration passed, including routing without target
access, fail-closed active faults, sparse refinements, postcode boundaries and
suppressed raw fields.

Original source visibility, read-only permissions and the hidden rollback edition
were checked afterward. Public containers remained healthy with unchanged start
times and zero restarts. All private instances are stopped. The existing derived
volume and additional roughly 154-GiB source copy are retained for reproducibility;
the live source, ordinary backups and Cloudflare routes were not replaced.
Configuration and orchestration live in `homelab/docker/ukinfo/hybrid-trial/`.

The selector is implemented and these tests are complete. Before public promotion,
retain the native rollback path, investigate cold grouped/ordered/native-postcode
tails, and complete derived backup/restore and upgrade checks. An equal-SQL-budget
control and longer independent references for native-timeout cases would improve
engine-level attribution. The measured benefit here is the complete hybrid
configuration under its existing budgets, not a universal sub-second guarantee.
