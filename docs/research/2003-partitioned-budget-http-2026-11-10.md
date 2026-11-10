# Partitioned-target HTTP cohort at matched limits — 10 November 2026

**All 635 responses matched; the performance gate still failed.** Initial
requests had no errors, C1 p95 passed at 334 ms, but C4 p95 was 1,024.685 ms
against the unchanged 1,000 ms limit. The 24.685 ms miss is retained as a failure,
not rounded down or retried. Production is unchanged and v14 stays hidden.

## Fixed scope and controls

Follow the [matched-layout SQL comparison](2003-postcode-layout-matched-2026-11-10.md)
with one frozen 2003 API workload. Same ecf25b2 image/b485487 street-only
candidate, without the rejected SIC predicate. Same load executable and worker;
the worker differs from operating-budget-http only in its private API URL.

All 127 original cases exactly match the prior scoped file, including order,
strings, hashes and digest modes: 39 business cases, zero saved HTTP cursor
cases. 127 initial requests, then C1 and C4 two fixed shuffled rounds of 254 each. Exactly
635 requests, 20-minute budget, unchanged client/API/SQL deadlines. No search
warmup, index probe, exclusions or adaptive repeat. The other 228 cases stay outside
scope. Initial requests are not guaranteed cold under shared OS/storage caches.

Private target 2 CPU / 6 GiB, MemorySwap 6 GiB; API 0.5 CPU / 512 MiB; client 0.25 CPU / 128 MiB.
Startup verifies sealed source/private hosts, nonprivileged read-only roles,
ready 59,886,841-entry metadata, pg_search 0.26.3 and the alias selecting the private
layout.partitioned copy. It rechecks exact SQL settings against the matched test:
shared_buffers 128 MB, work_mem 16 MB, generic plans, 4 s target timeout, maxparallel 0,
track_io_timing off. Metadata checks do not execute a search. No owner credential,
DDL, import, index build, local image or database copy.

The API also joins the source network to load authoritative records; client is
on the internal private network only. Live source/derived identities, start
times and quotas remain unchanged. Public source, target and API configuration and
public URL are not changed.

## Recorded result

| Stage | Requests | Errors | Median | p95 | Maximum |
| --- | ---: | ---: | ---: | ---: | ---: |
| Initial |127|0|125.5 ms|1,133.5 ms|2,383.5 ms|
| C1 |254|0|6.0 ms|334.0 ms|438.5 ms|
| C4 |254|0|14.6 ms|1,024.7 ms|1,902.8 ms|

No hash mismatch, unverified case or exhausted budget. The load runner exits 0
because its correctness/budget checks pass; the wrapper's performance pass flag
is false. Exact routing counts match the previous existing-target cohort:
590 paradedb, 35 paradedb_fallback, 5 paradedb_radius_stream. Radius is an additional
counter, not a mutually exclusive backend total. No accidental native-only or
original-target substitution could satisfy the alias and count checks.

Candidate phases total 85.972s / 590 calls, records 2.183 s / 645 calls and inventory
1.215s / 590 calls. Phase sums can overlap and are not exclusive wall-time shares.
Private PG CPU delta 39.184s, CFS throttled 16.702s across 102/610 periods, container-
accounted reads 1,499,545,600 bytes. These whole-load counters include background
work and cannot identify a particular C4 request or unique physical-device I/O.
Throttling time is not equivalent to per-request waiting time.

## Offline tail evidence

A separate network-disabled helper reads saved reports only. Cohorts are exactly
equal. It exports structural shapes/numbers, never values, IDs, ordinals or hashes;
no new HTTP/SQL request is made.

Eleven cases had at least one of their two C4 observations over 1 s. All eleven
have business SIC filters and no postcode. Leading shapes:

| Shape | Cases with an observation over 1 s | Shape maximum |
| --- | ---: | ---: |
| SIC prefix, name order |1|1,903 ms|
| SIC prefix + town, catalogue order |4|1,700 ms|
| Exact SIC + town, catalogue order |3|1,316 ms|
| Exact SIC, name order |2|1,301 ms|
| Organisation + exact SIC + town |1|1,228 ms|

These are per-case maxima from two observations, not robust case percentiles or
an exact count of requests above1s. All other shapes had maxima below 1 s. The
earlier postcode/name timeout case now returned 200, matched, and took 127.330 ms
in its initial observation. Shared caches differ, so this does not prove the
public incident or a cold-storage problem is fixed.

The prior existing-target cohort's C4 p95 was 230 ms, with one initial timeout;
this run had zero errors but a slower SIC tail. Different layouts, PostgreSQL
settings and uncontrolled cache/host conditions prevent attributing that whole
difference solely to partitioning. It is a reason for a focused matched-layout
SIC comparison, not evidence to promote one layout globally.

## Decision and closeout

Do not widen the corpus or deploy from this failed gate. Next compare the retained
unpartitioned and partitioned copies on these SIC shapes under identical settings
and load, preserving result/order checks. Only then consider whether postcode
and non-postcode searches need different index access paths. Do not repeat the
rejected redundant-SIC predicate experiment without new evidence.
The [matched-layout SIC comparison](2003-sic-layout-matched-2026-11-10.md) then
verified132 timed SQL results and nine page pairs. Unpartitioned C4p95 was480/480ms
versus partitioned1,213/2,061ms under identical private settings. This supports a
private routing trial, not clearing this failed HTTP gate or deploying globally.

Homelab docker/ukinfo/partitioned-budget-http preserves protocol/controllers,
offline ranking and numeric evidence. NAS root:
/home/jonw/ukinfo-backups/partitioned-budget-http-2026-11-10. Raw predicates,
records, IDs, cursor tokens, hashes and runner evidence stay NAS.

Fresh verification: all three API/client/offline helpers stopped, private PG stopped
and original 1.5 CPU / 1,536 MiB / MemorySwap 3 GiB restored, lease released, public health/
image and live source/derived identities/start times/limits unchanged. Prior
failed HTTP/page/all-edition gates remain retained. No deployment, full data copy,
storage migration or Bitcoin pause occurred.
