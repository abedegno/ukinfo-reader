# Bitcoin pause comparison — 8 November 2026

The user approved a brief Bitcoin pause to investigate the storage waits in the
[earlier postcode profile](postcode-io-profile-2026-11-08.md). Bitcoin was paused
for about 70 seconds, then resumed before the control phase. Both phases used
the same unchanged serving process, frozen 355-case cohort, references, CPU/memory
limits, pools and deadlines. No database restart, cache flush or production change
occurred. An independent resume watchdog and a `finally` recovery path were armed.

| Phase | Requests | Errors/mismatches | Initial p95 | C1 p95 | C4 p95 | Derived disk reads |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Bitcoin paused | 1,775 | 0 | 162 ms | 186 ms | 416 ms | 787 MiB |
| Bitcoin resumed | 1,775 | 0 | 145 ms | 139 ms | 458 ms | 14 MiB |

All 3,550 responses matched their frozen references. The earlier failed run still
had 11 errors and 5,290 MiB reads; it is not reclassified as passing. These are
single-page load comparisons, not full publication/cursor acceptance.

## Interpretation

**Bitcoin is not established as the cause.** The resumed phase benefited from
warmer caches, and its disk-read volume was about 55 times lower. It still ran
with active Bitcoin reads: 691 MiB over 61 seconds, about 11.3 MiB/s, substantially
below the earlier post-profile snapshot of 118.9 MiB/s. Background activity was
not constant. Neither warm phase recreates the earlier cache/storage conditions.

Derived-query I/O wait samples fell from 204 in the original reproduction to 13
while paused and three after resumption. CPU throttling remained present in both
new phases despite no errors. The evidence supports investigating reads that miss
the cache and shared-storage latency; it does not justify keeping Bitcoin paused,
changing BM25 or treating warmed tests as release qualification.

## Closeout

Bitcoin is verified running and unpaused. Both private workers and the private API
are stopped, the maintenance lease is released, and production still serves five
editions / 248,951,107 occurrences. v14 remains hidden. All reports are retained
under `/home/jonw/ukinfo-backups/postcode-contention-2026-11-08`.

Homelab preserves the fixed controller, numeric-only analysis and resume watchdog
in `docker/ukinfo/postcode-profile/`. Eight helper tests cover counter filtering,
the SQL address boundary and recovery targeting/idempotency. Real queries, records,
IDs, hashes and cursors remain on NAS.
