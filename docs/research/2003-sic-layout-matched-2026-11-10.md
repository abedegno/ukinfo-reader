# Matched-layout SIC SQL comparison — 10 November 2026

**The unpartitioned copy was consistently faster for the three measured SIC
shapes.** C4 SQL p95 was 480/480 ms versus 1,213/2,061 ms for the postcode-
partitioned copy, under identical private settings. All 132 timed responses
matched their projected-row oracles; all nine page pairs matched. This supports
a private routing trial, not deployment or reclassification of the failed HTTP
gate. Production is unchanged and v14 remains hidden.

## Fixed scope and controls

The [partitioned HTTP cohort](2003-partitioned-budget-http-2026-11-10.md) identified
eleven cases with a C4 observation over 1 s, all SIC without postcode. Rank its saved
per-case maxima descending, tie by original ordinal, then take the first three
distinct shapes without dropping cases: SIC prefix/name order, SIC prefix+town/
catalogue order, and exact SIC+town/catalogue order. These are selected tail
cases, not a representative national workload or an independent holdout.

Both retained full 2003 copies have the previously verified 59,886,841-row digest
and four-segment proof. The exact postcode partition option is checked again.
Use the existing dedicated private read-only reader, not owner/live credentials.
Only the fixed table reference differs between the reconstructed statements.
Scalar kind/town/SIC predicates, nonempty SIC, id/all, projection/order, 51-row
lookahead and seek match the application builder for these supported shapes.
No redundant SIC term/regex predicate or newly captured HTTP statement is used.

Correctness preparation walks up to three pages per shape/layout before timing,
requiring nonempty results, ordered six-field equality including lookahead and
no duplicate displayed IDs. Valid short final pages are allowed; none is dropped.
All selected walks actually returned three full pages with lookahead: 18 SELECTs,
450 displayed rows per layout,150 distinct per case. Preparation deliberately
warms both copies; this is not a cold-storage test.

One private PG, unchanged across timed arms: 2 CPU / 6 GiB, MemorySwap 6 GiB;
shared_buffers 128 MB, work_mem 16 MB, generic 4 s statements, maxparallel 0, IO timing off.
Four read-only sessions per arm, prepared before timing. Manual named PREPARE is
used with driver auto-prepare disabled. Each arm makes three initial requests,
then five rounds of three cases at C1 and again at C4: 3 + 15 + 15 = 33 timed
SELECTs per arm. Fixed A-B-B-A, no restart/flush between
arms, no adaptive retry. C1 uses one session; C4 uses a bounded four-session pool.
SQL timers exclude connection/metadata/PREPARE and include Execute/fetch/digest;
this is not full API latency. Helper 0.25 CPU / 128 MiB; private identity/quotas checked
around every arm. Six plan analyses run only after all timings.

## Observations

| Arm | SQL requests | Errors/mismatches | C1 median / p95 | C4 median / p95 | Private PG CPU delta |
| --- | ---: | ---: | ---: | ---: | ---: |
| Unpartitioned A1 |33|0|103 / 241 ms|290 / 480 ms|4.683 s|
| Partitioned B1 |33|0|471 / 606 ms|933 / 1,213 ms|15.205 s|
| Partitioned B2 |33|0|399 / 580 ms|1,284 / 2,061 ms|17.248 s|
| Unpartitioned A2 |33|0|109 / 242 ms|278 / 480 ms|4.741 s|

With 15 observations/stage, nearest-rank p95 equals maximum. All three shapes
favoured the unpartitioned copy in both C1 and C4 arms; per-shape results are in
homelab's numeric evidence. All generic execution counts were 33/custom 0 per arm.
No SQL error, result mismatch, hidden retry or timed workload replay occurred.

Supported container read counters had zero delta in every timed arm. This does
not establish zero physical I/O or exclude extension-managed/mapped reads. PG
CPU across the two partitioned arms totalled 32.453 s versus 9.424 s unpartitioned
for equal request counts (about 3.44x). Counters include helper setup/background,
not only query execution. CFS throttled times were 1.887/6.445/9.288/1.935 s;
they are not per-request waiting times. These matched warm observations support
an index-work/CPU trade-off rather than treating every SIC slowdown as cold I/O.
Host load and order still limit a causal capacity estimate.

Six later, single generic EXPLAIN ANALYZE observations at 4 s:

| Shape | Unpartitioned execution | Partitioned execution | Shared hits, unpartitioned / partitioned |
| --- | ---: | ---: | ---: |
| SIC prefix, name order |199 ms|424 ms|943 / 1,591|
| SIC prefix + town, catalogue |100 ms|399 ms|809 / 1,002|
| Exact SIC + town, catalogue |94 ms|264 ms|314 / 562|

All used Top-K over four segments, returned 51 rows with 51 heap fetches, and
reported zero shared-read/temp blocks. Parent/child buffer counts overlap and
must not be added. These plans do not identify the engine's internal partition
mechanism or trace the original failed HTTP request.

## Decision and next test

The earlier postcode first-access test favoured the partitioned copy; this warm
SIC test favours unpartitioned. Neither layout is supported as a universal winner.
Next implement a private, opt-in routing trial: retain the partitioned path for
postcode-constrained queries and use an unpartitioned path for SIC queries without
a postcode. Keep other routes unchanged until measured. Verify full response and
pagination/cursor equality, then run the unchanged 635-request HTTP cohort. No
additional CPU or relaxed threshold is justified merely to obtain a green run.

This small SQL diagnostic does not clear the 1,024.685 ms HTTP gate, all-edition or
v14 requirements. SQL position digests are not hydrated HTTP response hashes.
Existing failed attempts and the rejected redundant-SIC predicate remain recorded.

Homelab docker/ukinfo/sic-layout-matched contains controllers, source-checked SQL,
five passing contract tests and safe numeric evidence. NAS root:
/home/jonw/ukinfo-backups/sic-layout-matched-2026-11-10. Query values, original
ordinals, rows, IDs, hashes, SQL/arguments and raw plans stay NAS. Pre-launch
review verified SQL fidelity and resource/lifecycle guards. No owner credential,
new role, DDL, index build, full data copy or local image/database copy.

Fresh verification: all six helpers stopped; private PG stopped/restored to
1.5 CPU / 1,536 MiB, MemorySwap 3 GiB; lease released; public health/image and live
source/target identity/start/limits unchanged. No deployment or storage/Bitcoin
change occurred.
