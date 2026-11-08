# Full-edition postcode layout experiment — 8 November 2026

**Status: recovery reattached after Mac crash; build continues on NAS. No performance results yet. Nothing deployed.**
The complete export succeeded: 59,886,841 rows / 8,625,509,916 bytes in 462 seconds.
The helper was then OOM-killed (exit 137, 384 MiB limit) during baseline import,
after its last progress report at 708,837,376 bytes. Docker confirms the OOM;
the precise import-memory cause still needs investigation. No indexes, pagination
comparisons or performance measurements completed. Automatic cleanup stopped the
helper, private PostgreSQL and maintenance lease; the public origin remains
healthy with its original image and Bitcoin running. All artifacts, including
the completed export, remain retained.

The user authorised this follow-up to the promising
[one-million-row pilot](postcode-index-layout-pilot-2026-11-08.md).
It compares two complete copies of the 59,886,841-entry 2003 edition on the
existing NAS HDD pool. There is no SSD pool or storage migration.

Both copies use the installed PostgreSQL/pg_search image, identical schemas,
indexed fields and a four-segment target. Only the candidate adds
`partition_by='postcode'`. A sealed SELECT-only reader exports the existing
projected generation once. Both copies must match its complete ordered binary
digest and full ID range before measurement. Original data and digests stay NAS.

The fixed workload covers postcode, surname, given-name/location,
surname+given-name/location, street/town, town, county/surname, trailing name
wildcards and business prefix/SIC searches. Missing source fields are explicitly
reported and skipped rather than dropping their filters. A SELECT-only role
executes queries with four-second SQL budgets. Correctness checks compare up to
three pages, lookahead, total ordering, duplicates and an empty control.

Eight postcode cases use balanced first-query/repeat passes. Only the private
server is stopped for file-cache advice; no global cache drop or live restart.
The identical mixed workload then runs at one and four concurrent clients, with
balanced layout order and all failed timings retained. Physical I/O, errors and
per-shape tail latency determine the outcome. Cache advice does not control
hardware/RAID caches, and these sampled SQL workloads do not measure HTTP capacity,
source hydration, geography, cross-edition merging or deduplication.

## Reproducible operations

Configuration, protocol, tests, controller and automatic numeric analysis live in
homelab under `docker/ukinfo/full-layout/` (initial commit `a67307d`). The dated
controller and reports use `full-layout-2026-11-08`; all records/query values/IDs
remain under `/home/jonw/ukinfo-backups/full-layout-2026-11-08` on NAS.

The code-only helper built on NAS in 36.8 seconds; all 14 tests passed with the
real Linux driver. The image is `ukinfo-full-layout:2026-11-08.1`, identity
`sha256:d7f20770a45ac9cdd739e2bcb1cf6dbc94ba2a8e19c9146686a907535929d1f9`.
An independent read-only review found and checked fixes for cache-container name
collisions, workload coverage, free-space monitoring and failure cleanup. Fourteen
pure tests also passed locally, with the driver import stubbed for those tests.

The private database/helper have a combined limit of 2 CPU/1,920 MiB, no published
ports and a new internal database network. Preflight found about 22 TiB free on
NAS. A 100 GiB free-space watchdog covers long stages, with explicit stage timeouts
and cleanup of owned services/lease. The controller ran detached on the Mac with
sleep inhibition and aggregate progress logging; it exited after this failure. Network/controller failure still needs operator inspection; do
not start a second run or replay the dated commands while its lease is active.

This experiment does not deploy automatically, even if its gates pass. The public
site remains five editions / 248,951,107 occurrences, with v14 hidden and Bitcoin
running. The small pilot's improvement must not be presented as a full-edition
result until the new reports are complete.

## Memory diagnosis and separate recovery

Inspection of the exact installed psycopg 3.3.8 code confirmed its Linux default
COPY writer enqueues without flushing between writes. A slower server can
therefore leave an expanding client-side buffer. This matches
[upstream issue 1437](https://github.com/psycopg/psycopg/issues/1437); the automatic
bound appears under unreleased 3.3.9 in the
[release notes](https://www.psycopg.org/psycopg3/docs/news.html).

An invented-data regression with that installed Linux driver reproduced 1,400,000
queued bytes, failing the required 65,536-byte bound. The replacement writer
flushes each <=64 KiB chunk and passed the same test while preserving every byte.
It retains the pinned driver's completion/error handling. No application library
or memory limit is changed. Actual restored-data integrity and real import memory
remain separate checks, not inferred from this transport test.

Homelab commit `7b4e972` adds the bounded writer, process/container memory telemetry
and a distinct recovery controller. Run `full-layout-recovery-2026-11-08` uses
fresh private PostgreSQL/services/volume and helper image `.2`; the failed `.1`
attempt remains retained. The completed export is mounted read-only, its entire
size/digest is verified before reuse, and no source-reader credentials or
production network are provided to this helper. An invented-row binary COPY
round trip precedes full import. The existing full-copy, cursor, layout, cache
and mixed-concurrency gates are unchanged. No timed measurement has been retried.

The recovery helper built on NAS in 82.4 seconds, passing all 15 tests with the
real Linux driver. Image `.2` is
`sha256:557e959e56b4da74dab147bf0d247da53140126dad5220462fb07cbdb74915c9`.
The actual private PostgreSQL binary COPY smoke check passed. The retained
8,625,509,916-byte export passed its complete checksum verification in 52.9 seconds.
Fresh baseline import has started; full-copy and performance results remain pending.
It deploys nothing.

The resumed import subsequently passed **1,404,043,264 bytes**, beyond the original
last-reported 708,837,376-byte failure point, with process peak RSS holding at
44,880 KiB (about 44 MiB). The helper still has its original 384 MiB limit.
Container usage can approach that limit because it also includes reclaimable
input-file cache; sampled anonymous memory remained about 30 MiB. This is evidence
for bounded client buffering under the real import, not a completed full restore
or a performance result. Public origin health and serving identity were rechecked;
Bitcoin remains running and unpaused.

## Mac crash and adoption

The Mac crash removed temporary local logs and the controller, while the NAS
helper, private PostgreSQL and maintenance lease continued running. The baseline
completed full-copy verification, index creation and vacuum: four segments,
59,886,841 documents and 3,062,773,029 visible segment bytes. The partitioned table
also completed import and its exact count/ID-range checks; its full checksum
verification is currently running in the original detached build process.

A reviewed, tested `adopt.py` in homelab reattached to that same process, with no
re-export, import/index restart or measurement replay. It preserves the original
six-hour build budget, restores memory/space monitoring, refuses partial/active
workloads, and continues the existing benchmark only after both full build proofs
pass. The NAS records adoption under `control/adoption-1/`; durable, gitignored
Mac logs/state now live under `private/operations/` rather than `/private/tmp`.
Five adoption guard tests pass. Public origin health and serving image were
rechecked unchanged, and Bitcoin remains running/unpaused. No deployment occurred.
