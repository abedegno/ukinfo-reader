# Saved broad-failure diagnosis — 10 November 2026

**The isolated captures did not reproduce the timeout:** all seven distinct
saved failing cases returned 200 and matched their original frozen hashes. They
ran in 32–995 ms; the original broad gate remains failed. Per-query evidence and
22 later plans narrow the investigation, but do not establish the original cause.

The [broader regression](2003-hybrid-broad-regression-2026-11-10.md) completed
1,775 requests but failed its zero-error gate with seven initial and one C1
HTTP 503s. Failures concern person postcode/name ordering and cross-edition
town/name/grouping searches. This follow-up traces the actual candidate calls;
it is not another full workload or a release qualification.

## Fixed scope and controls

Select every original initial/C1 failed case, deduplicate in original order,
then run each once through the frozen one-request hash checker. Maximum eight
HTTP requests; original query/path/limit/hash unchanged. Selection never depends
on diagnostic success. Before serving, verify six source/index identities and
read-only roles; the retained target has no optional SIC registry, so the
experimental flag continues through the original main index path.

The diagnostic binary uses source `66876ae74cb6d2b623d37733f7018dc405427f55` plus
four asserted tracing callbacks and a separate private audit file. Product source
and ordinary telemetry are unchanged. Hooks observe the actual candidate SQL and
bound parameters, query duration/row count/SQLSTATE and pool acquisition time.
They write only to a mode-0600 NAS file, bounded to 16 MiB; overflow invalidates
capture. Source metadata, raw SQL/arguments/plans, PIDs, case ordinals, cursors
and response hashes remain on NAS. No raw database error text or connection
string is logged or exported to OTEL. Synthetic tests cover disabled behavior,
file permissions, fixed error classes, raw-error exclusion and cap signaling.
The test failed without the hooks, then the overlay's telemetry suite passed.

Code-only NAS build uses pinned Go 1.26.10, maximum 2 CPU / 2 GiB; frozen base
image identity and RootFS ancestry checked. No local image/database copy or full
index/data copy. Private API 0.5 CPU / 512 MiB; diagnostic helper 0.25 CPU /
128 MiB. Existing source/target databases remain 2 CPU / 6 GiB, without restart,
cache flush, settings/quota changes, owner credentials, role creation or DDL.
The separate retained private hybrid PG remains stopped at its original limits.

During each request, sample active target wait types at about 20 ms, filtering
by this private API's address, role and application name. Do not inspect another
client's queries. Original client/API/SQL deadlines remain; the helper places a
25 s outer bound on each one-request tool. Pool timing covers the scoped private
API's acquisitions and may include private background work. Trace overhead,
sampler activity and existing cache state can perturb latency.

Only after all HTTP captures, take the first actually observed statement per
case/table, at most 48. Validate the generated SELECT/schema/table/parameter
shape and replay its exact bindings: one generic estimate, then one generic
EXPLAIN ANALYZE/BUFFERS, unchanged 4 s statement timeout. No custom-plan variant,
query rewrite or retry. Keep errors. Estimates may warm metadata before analysis;
these later plans cannot reconstruct the original first-access conditions.
Root/child buffer counts overlap and are not additive physical-byte counts.

Maintenance lease and image/start/quota snapshots guard the run. Stop only owned
API/helper on all exits; release the lease after health/runtime checks. No host
ports, deployment, v14 publication, storage migration or Bitcoin change.

## Observations

| Saved failure shape | Capture HTTP | Candidate queries / editions | Sampled I/O / other-active observations |
| --- | ---: | ---: | ---: |
| se2005-pcf175-2005-06; postcode; name; none | 796.011 ms | 1 / 1 | 25 / 6 |
| se2004-pcp218-2004-07; postcode; name; none | 994.596 ms | 1 / 1 | 36 / 9 |
| all; first_name, surname, town; catalogue; none | 646.326 ms | 6 / 6 | 8 / 45 |
| all; first_name, surname, town; catalogue; listing | 614.666 ms | 6 / 6 | 0 / 44 |
| all; town; catalogue; listing | 885.440 ms | 12 / 6 | 42 / 13 |
| all; town; edition; none | 32.300 ms | 1 / 1 | 0 / 1 |
| se-pcf78-1998-01; postcode; name; none | 36.525 ms | 1 / 1 | 0 / 1 |

All seven original initial failures have saved error code `query_timeout`.
The C1 error repeats one of those cases, hence seven distinct cases from eight
failed requests. The 2004/2005 postcode captures had one candidate query each,
956/713 ms, and numerous private-target I/O samples. Their later identical generic
plans took 116/40 ms. Order and cache state differ; this is evidence of state
sensitivity, not a controlled speedup or proof of the original I/O cause. Samples
in this first diagnostic identify private target sessions, not a timestamp/PID
join to each query; do not convert sample counts to I/O time.

Pool acquisition maximum was 9.650 ms across these captures, with no errors.
The two name/town all-edition captures each queried six editions, at most two
sampled active at a time. Some zero-result statements still took substantial
work: later 1999 plans took 215/313 ms with zero heap fetches and 23,100 shared
buffer hits; selected v14 zero-result plans took 216/190 ms with 13,321 hits.
These counters are accesses, not distinct pages/bytes or proof of per-row filtering.
The edition-ordered town query touched just the newest edition in this capture;
increasing global fanout would not address every observed shape.

The grouped town query issued 12 candidate statements: six initial scans, then
six pair-drain statements, including 2,048/2,048/1,797 rows from 1999. Those later
rows complete the canonical name pair before grouping; no truncation is proposed.
All 28 captured candidate statements completed. There were 380 audit events.

All 22 selected plan analyses completed at 5.672–313.276 ms, Top-K over four
segments, generic 2/custom 0 each. They reported no shared reads or temporary
blocks; heap fetches matched returned rows. Root/child buffer counts overlap;
this does not establish zero physical I/O, especially for extension-managed reads.
No explicit sort/spill or observed pool-queue problem was found in these captures.

A networking-disabled inspection of saved plan structure found deferred
`PostgresExpression` nodes in all 22 generic query trees. The version-pinned
[engine implementation](https://github.com/paradedb/paradedb/blob/v0.26.3/pg_search/src/postgres/customscan/solve_expr.rs)
evaluates these query-builder expressions into query inputs before scanning.
Their presence is not evidence of ineffective per-row filtering or a reason to
add redundant predicates without measurement. Official source was checked again.

Diagnostic image `8cb27a66a03c00fb4d380e688a95bdc51d32667386b072caddb07f23a59ae7ac`
built on NAS in 304.529 s. The exact-source build's Go tests, including synthetic
private-audit guards, passed. No original full-cohort request was made in this run.


## Interpretation and next step

These observations support investigating candidate index-read/scan work and
workload state before changing hydration, connection-pool size or deadlines.
They do not reproduce the original failure and do not justify a production SQL
change. In particular, fast warm captures cannot clear the existing zero-error
gate, and observed pool waits do not rule out a queue in a different run.

A separately declared follow-up uses the same diagnostic image for one original-
order pass through all 355 frozen cases, not the 1,775-request C1/C4 benchmark.
It records target PID/timestamps/waits and correlates samples to query intervals.
Separate one-case tool processes and sampling change observer/connection overhead;
this is a diagnostic reproduction attempt, not identical performance evidence.
Maximum 355 requests / 20 minutes, original deadlines/resources, no cache flush,
query change or retry. Its outcome is recorded separately below.

### Separately recorded original-order trace

Run `broad-initial-trace-2026-11-10` reused the exact diagnostic image without a
build. All 355 frozen requests returned 200 and matched; no timeout, mismatch,
sampling error or candidate error was reproduced. It recorded 526 candidate
queries, 4,437 audit events and 690,101 audit bytes. Pool acquisition maximum was
13.713 ms. Routing was 295 ParadeDB, 45 fallback, 8 radius-stream and 1 source-
prefix-parallel decisions, exactly one fifth of the preceding full cohort's base
counts; no accidental wholesale native fallback or SIC overlay.

Candidate phases totalled 35.186 s / 295 calls, record hydration 1.917 s / 474,
inventory 0.814 s / 295; sums can overlap and are not throughput or per-request
wall-clock attribution. Per-query PID/time/wait correlation is available in the
private raw evidence, but no failing query occurred to attribute. The original
seven initial and one C1 errors therefore remain unexplained at request level.

This pass ran after prior diagnostics and plan analyses. It neither recreates
original cache state nor repeats the frozen C1/C4 qualification. A clean 355-
request diagnostic pass does not supersede the earlier 1,775-request failure.
No query, ordering, pagination, pool, timeout or layout fix was made.

### Decision and cleanup

The strongest measured leads are state-sensitive candidate access and index work;
these observations do not establish the cause of the original timeout. Pool
queues, explicit sorts and spills were not problematic in these diagnostic runs;
that does not rule them out in other circumstances. Grouping's complete-pair
contract must remain intact. A blanket concurrency/timeout increase or redundant
predicate is unsupported by this evidence.

Next add value-free, bounded per-edition query timing/error context to ordinary
observability, so a genuinely failing request can be correlated with NAS disk,
CPU and memory signals without exporting SQL/parameters. Use that failure evidence
to choose a query/layout change, then repeat the original zero-error gate. Current
2003-only hybrid success, broader failure and combined-layout/v14 requirements
remain separate; no production rollout is qualified.

Fresh verification after both runs: all five diagnostic helpers stopped, lease
released, retained private hybrid PG stopped at original limits, live source/
target/public image/start/quotas unchanged. Public health good; five editions /
248,951,107 occurrences, v14 hidden. No deployment, source/index mutation,
additional full data copy, storage migration or Bitcoin change.

Configuration, private overlay source/tests and safe numeric evidence live in
homelab `docker/ukinfo/broad-failure-evidence/`; NAS root is
`/home/jonw/ukinfo-backups/broad-failure-evidence-2026-11-10`.
Independent prelaunch review checked binding fidelity, privacy, failure selection,
strict file cap, read-only plan bounds and cleanup. Its cap correction was fixed
and tested before launch. Earlier failed gates remain recorded.

The second run's configuration and aggregate evidence are in homelab
`docker/ukinfo/broad-initial-trace/`, NAS root
`/home/jonw/ukinfo-backups/broad-initial-trace-2026-11-10`. It was independently
reviewed before launch for fixed bounds, wait correlation, privacy and lifecycle.
