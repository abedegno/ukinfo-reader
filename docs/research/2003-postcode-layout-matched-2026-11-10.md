# Matched-budget retained postcode layouts — 10 November 2026

**The postcode-partitioned index is the stronger candidate for a full API test
at these limits.** In the fixed four-arm SQL diagnostic, both partitioned arms
completed; the unpartitioned copy timed out once and finished near the 4 s budget
once. All three page pairs matched. The original HTTP failure and release gates
remain unchanged; nothing deployed.

## Scope and fixed controls

Use the actual SQL and parameters captured in the
[postcode/name inspection](2003-postcode-name-plan-2026-11-10.md). Replace only
its fixed relation with layout.baseline or layout.partitioned. These are the
retained complete 59,886,841-row copies in one private PostgreSQL container, with
previously verified full-source digest equality and four segments per index. They are
not the currently serving target. Partitioning here is pg_search's
partition_by=postcode index option, not a new PostgreSQL partitioned table.

Both variants share 2 CPU / 6 GiB cgroup limits, MemorySwap 6 GiB and identical settings:
shared_buffers 128 MB, work_mem 16 MB, force_generic_plan, statement_timeout 4 s,
max_parallel_workers_per_gather 0, track_io_timing off. The helper has 0.25 CPU / 128 MiB.
Only the private database is stopped/started before each A-B-B-A arm. That clears
PostgreSQL shared buffers; OS/RAID/controller/drive caches remain uncontrolled.
No cache advice, global cache flush, live restart, source export or index build.

A private setup-only owner helper creates a dedicated login with SELECT on just
the two retained tables. It verifies exact reloptions before transactional role
creation and checks no TEMP/CREATE/INSERT access. Measuring helpers get only its
sealed read-only credential on the internal private network; no owner/live DSN.
The small role and control file remain for evidence. Public source/target/API
configuration is unchanged.

Four fixed first-data-reading statements: baseline,partitioned,partitioned,
baseline. Metadata/role/settings/IO snapshots and PREPARE precede each single
generic EXPLAIN ANALYZE/BUFFERS. No preceding data query or EXPLAIN-only warmup,
custom-plan variant, adaptive SQL or retry until green. Exactly4analysis attempts;
each records generic 1 / custom 0. After all timing, perform three page queries per layout
with application-equivalent seek clauses and 51-row lookahead.

## Observations

Wall time includes EXPLAIN planning/execution/transfer but excludes PREPARE and
metadata. A timeout is a censored failure, not an exact completed execution time.
Docker counters bracket the entire helper, including metadata/background work;
they are container-accounted bytes, not unique physical-device bytes.

| Arm | Outcome | Wall | Plan execution | Plan shared read blocks | Container-accounted reads |
| --- | --- | ---: | ---: | ---: | ---: |
| Unpartitioned A1 |57014 timeout|4,127 ms|not returned|not returned|472.1 MiB|
| Partitioned B1 |ok|2,319 ms|1,689 ms|4,924|213.3 MiB|
| Partitioned B2 |ok|337 ms|169 ms|4,924|1.3 MiB|
| Unpartitioned A2 |ok|3,989 ms|3,827 ms|13,876|338.7 MiB|

B1/B2/A2 planning took 586/166/150 ms. Completed plans all used TopKScanExecState,
four segments, 51 returned rows/heap fetches and no temp blocks. Their shared hit
counts were 293/293/406. Parent/child counters overlap and must not be added.
No explicit partition-pruning field was exposed by these plan summaries; exact
index options were independently verified from reloptions.

In the completed plans, the partitioned copy reported about 65% fewer shared
read blocks than unpartitioned A2. These are PostgreSQL buffer misses, which can
be served by OS cache. The large B1/B2 difference despite equal shared-read
counts demonstrates why these are not disk-latency or physical-byte measures.
Two observations per layout do not support robust percentiles or a causal
physical-I/O reduction estimate.

Private PG CPU deltas were 272/185/148/324 ms; supported CFS counters reported no
throttling. Combined with the observed read activity, this is consistent with an
I/O-sensitive first-access path in this private test, rather than CPU quota
exhaustion. It does not identify the cause of the original untraced HTTP timeout.

pg_stat_io also captured private-server aggregate changes, including metadata
and background activity. B2's immediate snapshot had no delta despite 4,924 plan
read blocks: statistics can lag. Zero read_time is uninformative with IO timing
disabled. Unsupported Docker deltas are null, not measured zeroes. Raw snapshots
and full plans remain NAS; numeric summaries preserve these limitations.

## Correctness, decision and cleanup

After timing, all three pages per layout returned 50 displayed rows plus lookahead, without
duplicate displayed IDs in each 150-row walk. All six projected fields, ordering
and lookahead rows matched between layouts. The first 50 IDs matched the saved
successful HTTP capture. This verifies SQL positions, not hydrated records,
cross-edition results, cursor tokens or full API response hashes.

The comparison completed, correctness passed, but all_first_touch_plans_succeeded
is false because A1 timed out. No historical failed gate was reclassified. Next
run the frozen 127-case/635-request HTTP cohort on the private partitioned target
at these matched limits, including its initial requests and original thresholds.
Do not promote on this single-query SQL result or hide the slower B1 observation.

Pre-launch review corrected lease retention on live drift, exact option checks
and missing-counter handling. Six contract tests pass, including seek ordering
and cleanup/release gating. A local helper-module collision was fixed before any
NAS action; no timed arm was replayed. No local database/image copy or build.

NAS root: /home/jonw/ukinfo-backups/postcode-layout-matched-2026-11-10. Homelab
docker/ukinfo/postcode-layout-matched preserves protocol/controllers/tests and
safe numeric evidence. Queries, values, rows, IDs, hashes and raw plans stay NAS.
Fresh verification: all six helpers stopped, private PG stopped and restored to
1.5 CPU / 1,536 MiB, MemorySwap 3 GiB; lease released; public health/image and live
source/target identities/start times/limits unchanged. No storage migration or
Bitcoin pause occurred.
