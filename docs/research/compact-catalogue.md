# Compact PostgreSQL catalogue measurements

17 October 2026. This follows the [architecture spike](search-storage-spike-2026-10-17.html)
with **all original record fields**, rather than the earlier narrow search projection.
The implementation is Go + PostgreSQL 18 for hosted and desktop use, with the Python
reader used during ingestion only. Source records and query fixtures remain private.

## Layout comparison: identical two-million-record stream

Seed 1989 selected byte-region clusters across the 2005 residential/business base/update
streams: 1,713,996 residential base, 52,254 residential update, 193,500 business base and
40,250 business update occurrences. This is a layout pilot, **not an unbiased population
sample**. Both imports received the same complete-field stream. Independent producer
and stored-block verification agreed on SHA-256
`1460e77101dc0244872955b1eb2b2223813c1ec7af9487a318554686d7499891`.

| Metric | Schema 1: JSONB | Schema 2: compact |
|---|---:|---:|
| Database bytes | 2,832,201,407 | 593,024,703 |
| Ingestion seconds | 94.97 | 50.38 |
| Warm 1-caller requests/s | 3,294 | 1,723 |
| Warm 8-caller requests/s | 8,414 | 3,024 |
| Warm 32-caller requests/s | 8,383 | 3,005 |
| Scheduled 100/s p95 | 4.744 ms | 5.497 ms |
| Scheduled 100/s p99 | 5.678 ms | 7.414 ms |
| Scheduled errors/drops | 0 / 0 | 0 / 0 |

Compact storage is **79.1% smaller**, with a **46.9% shorter import** in this pilot.
It also has substantially lower saturated throughput: fetching and decompressing blocks
costs CPU. The observed 100/s latency remained low. Neither layout has been tested on a
public VPS, across the internet or under a sustained production workload.

PostgreSQL 18.6 and each API ran inside the same Docker container capped at **2 CPUs and
1 GiB RAM** on a macOS ARM64 host. A separate load container had 2 CPUs and 1 GiB RAM.
The host-native Go importer was outside that CPU cap. The API admitted 32 concurrent
requests, used eight database connections and two-second SQL deadlines. Each closed-loop
stage lasted ten seconds; the scheduled stage sent 3,000 arrivals over 30 seconds.
All 210 cases were warmed first and repeatedly reused: these are hot-key measurements.
Every timed response was hash-checked. Before timing, 420 responses across both layouts
were checked for counts, pagination, IDs and every source field against the JSONB
reference from the same independently checksummed source stream, with zero mismatches.
Different valid sort orders were allowed. No network latency was added.

[Raw aggregate results](compact-pilot-results.json) include percentiles and methods.
The [HTTP load tool](../../tools/search-load/README.md) accepts private validated cases.
The pilot used COPY batches of 8,192; full ingestion now uses 65,536, so import timings
are measurements of those configurations rather than a universal speedup prediction.

## Complete 2005 edition

The complete source-verified 2005 edition imported **51,350,897 occurrences** in one
transaction: 32,062,644 electoral, 13,135,957 telephone and 6,152,296 business records.
The transaction took 1,285.59 seconds (21.43 minutes), including 1,225.93 seconds loading,
4.94 seconds checking identities and 54.26 seconds building indexes. Producer decoding,
source fingerprint work and pipeline startup can add wall time around that transaction.

| Storage | Bytes |
|---|---:|
| Whole PostgreSQL database | 12,657,317,567 (11.79 GiB) |
| Search projection heap | 2,834,718,720 |
| Search projection indexes | 4,855,791,616 |
| Record-block relations including indexes/TOAST | 4,069,384,192 |
| Shared vocabulary relations | 889,241,600 |
| Original record JSON before block encoding | 41,362,864,034 |
| Compressed payload bytes only | 3,455,082,251 |

Relation categories and database accounting need not sum exactly (system metadata and
sampling times differ). Payload size excludes PostgreSQL tuple, TOAST and index overhead.
The shared vocabulary retained terms from the preceding two-million-row pilot; therefore
the whole database number is a conservative measured footprint, not a fresh-only minimum.
WAL, temporary sorts, source files, runtime binaries and backups are additional disk use.
The importer streams records through a pipe and removes private identity-sort scratch
files before index building. The native import supervisor and desktop wizard stop at a
5 GiB local reserve; operators must separately monitor remote/Docker volume capacity.

## Hosting implications

This establishes that the complete edition fits comfortably in a modest persistent
PostgreSQL volume; it does not establish a safe VPS size or public request limit. A free
static host can serve frontend assets, but cannot remove database storage, RAM and backup
costs. Plan paid persistent storage for the database and retain the same API on desktop.
Keep PostgreSQL private; use a read-only serving role, TLS, edge quotas and bounded API
admission. More API processes each add connections and decompression work.

The next capacity gate is a full-corpus test with varied keys, cold and warm caches,
sustained arrival rates, slow queries and recovery measurements on the intended host.
Provider pricing and final hosting selection remain open. Do not treat short local
requests-per-second numbers as internet capacity or an availability guarantee.
