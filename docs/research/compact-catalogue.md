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
| Ingestion wall seconds | 94.97 | 50.82 |
| Warm 1-caller requests/s | 3,294 | 1,723 |
| Warm 8-caller requests/s | 8,414 | 3,024 |
| Warm 32-caller requests/s | 8,383 | 3,005 |
| Scheduled 100/s p95 | 4.744 ms | 5.497 ms |
| Scheduled 100/s p99 | 5.678 ms | 7.414 ms |
| Scheduled errors/drops | 0 / 0 | 0 / 0 |

Compact storage is **79.1% smaller**, with a **46.5% shorter import** in this pilot.
It also has substantially lower saturated throughput: fetching and decompressing blocks
costs CPU. The observed 100/s latency remained low. Neither layout has been tested on a
public VPS, across the internet or under a sustained production workload.

PostgreSQL 18.6 and each API ran inside the same Docker container capped at **2 CPUs and
1 GiB RAM** on a macOS ARM64 host (Apple M4 Pro, 14 host CPUs, 24 GiB host RAM). A separate load container had 2 CPUs and 1 GiB RAM.
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
The importer recorded 1,285.59 seconds (21.43 minutes) through partition attachment,
including 1,225.93 seconds loading,
4.94 seconds checking identities and 54.26 seconds building indexes. This includes waiting for the producer while decoding and checking source fingerprints.
Commit and post-commit `ANALYZE` are outside that statistic; supervised end-to-end wall
time can therefore be longer.

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
this is its observed database size, rather than a separately measured fresh-only footprint.
WAL, temporary sorts, source files, runtime binaries and backups are additional disk use.
The importer streams records through a pipe and removes private identity-sort scratch
files before index building. The native import supervisor and desktop wizard stop at a
5 GiB local reserve; operators must separately monitor remote/Docker volume capacity.

### Full-record verification

After commit, a separate verifier read all **401,179 blocks** and every search projection
in **535.25 seconds**. The digest matched the Python producer's independent source-stream
SHA-256 `7b903685a41b521e3a2ecbe117c43be70d7a170d2c4374b12241076873dcb9b8`.
All 51,350,897 records and normalized keys matched. The source contains 149,829 occurrences
with warnings and 11,882 without a displayed postcode; these values remain preserved.

The native bundled decoder also identified and fingerprinted all four supported editions.
The compact catalogue's complete-edition preservation check currently covers **2005**;
1998, 1999 and 2004 have sample ingestion coverage, not whole-edition PostgreSQL imports.

### Full-corpus HTTP workload

A separate workload uses 1,550 cases: 1,500 ordinal positions
`1 + ((i * 104729) % 51350897)`, for i=1..1500, plus 20 common names from the pilot
and 30 absent keys. The sampled source fields rotate among postcode, exact name and
name-plus-first-name filters, retaining person/business types. First-page IDs, counts,
next-page presence and original-field filters matched SQL projections with zero
mismatches. Those projections were separately checked against every decoded source
record by the whole-catalogue verifier. Private body hashes validate timed responses.

The full database/API share the same 2-CPU/1-GiB container cap described above. Serving
uses a SELECT-only role with read-only transactions. The load container has its own
2-CPU/1-GiB cap and shares the database container's network namespace, so this measures
local TCP rather than an external network. Each scheduled stage lasts **60 seconds**
with all cases warmed first; 1/8/32-caller stages last ten seconds each.

| Scheduled workload | Requests | p95 | p99 | Errors / drops |
|---|---:|---:|---:|---:|
| 100/s for 60 seconds | 6,000 | 8.783 ms | 11.574 ms | 0 / 0 |
| 300/s for 60 seconds | 18,000 | 7.019 ms | 10.283 ms | 0 / 0 |

The 32-caller runs observed approximately 704 and 714 requests/s. The 100/s run preceded
the 300/s run; cache and host state were not controlled between runs. The faster latency
in the later run is therefore not a scaling improvement claim. CPU quotas on this shared
Mac host do not reproduce the cores or storage of a low-cost VPS.
[Full aggregate results](full-catalogue-results.json) include storage, independent
verification, HTTP correctness, all percentiles and environment limits.

This is still a short, repeated-key local test. It includes transfer and response-body
hashing but excludes browser JSON parsing, WAN latency, TLS, user authentication and
public edge quotas. It does not establish cold-cache or multi-edition capacity.

## Hosting implications

This establishes that the complete edition fits comfortably in a modest persistent
PostgreSQL volume; it does not establish a safe VPS size or public request limit. A free
static host can serve frontend assets, but cannot remove database storage, RAM and backup
costs. Plan paid persistent storage for the database and retain the same API on desktop.
Keep PostgreSQL private; use a read-only serving role, TLS, edge quotas and bounded API
admission. More API processes each add connections and decompression work.

The next capacity gate is a test on the intended host with cold and warm caches,
longer sustained arrival rates, slow queries and recovery measurements.
Provider pricing and final hosting selection remain open. Do not treat short local
requests-per-second numbers as internet capacity or an availability guarantee.
