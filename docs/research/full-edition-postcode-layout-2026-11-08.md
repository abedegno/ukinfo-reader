# Full-edition postcode layout experiment — 8 November 2026

**Status: running privately; results pending. Nothing deployed.**
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
and cleanup of owned services/lease. The controller runs detached on the Mac with
sleep inhibition and logs aggregate progress, so it can finish without this chat
remaining open. Network/controller failure still needs operator inspection; do
not start a second run or replay the dated commands while its lease is active.

This experiment does not deploy automatically, even if its gates pass. The public
site remains five editions / 248,951,107 occurrences, with v14 hidden and Bitcoin
running. The small pilot's improvement must not be presented as a full-edition
result until the new reports are complete.
