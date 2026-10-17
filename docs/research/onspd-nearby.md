# ONSPD reference and nearby-search checks

17 October 2026. See [setup and accuracy](../geography.md).

## Reference ingestion

The hash-pinned corrected August 2026 ZIP was imported directly, without extracting its
1.48 GB single UK CSV or changing any of the 189,064,266 complete-source occurrences.
Only Great Britain rows were retained. SQL aggregate checks matched the importer's
2,652,624 retained rows and 2,573,343 usable postcode points; no unexpected country codes
were present. The source archive also contained 63,293 excluded Northern Ireland rows
and 13,173 excluded other/Crown dependency rows.

The reference table and indexes use **880,787,456 bytes (about 840 MiB)**. The original
ZIP is 253,928,895 bytes. Temporary/WAL headroom is additional. The local disk reserve
remained above 5 GiB throughout; about 20 GiB was free at the final check.

## Full-catalogue HTTP checks

The final implementation was exercised against all four complete editions, plus existing
sample/demo imports. Redundant samples were excluded by the all-edition selector. Each
case below followed five 50-result pages. Source identities were checked for duplicates,
returned distances for radius bounds and finiteness, positional quality for allowed values,
name/type filters where supplied, and the expected reference release. No directory records
are stored in this report.

| Case | Page times, ms | Occurrences | Errors/duplicates/filter mismatches |
| --- | --- | ---: | ---: |
| Lichfield area | 157.94, 56.85, 35.52, 36.92, 34.42 | 250 | 0 |
| Lichfield surname | 40.80, 51.17, 52.11, 65.87, 48.05 | 250 | 0 |
| London 25-mile area | 1929.86, 53.66, 56.17, 49.12, 51.04 | 250 | 0 |
| Edinburgh businesses | 86.78, 29.51, 25.40, 31.01, 24.37 | 250 | 0 |
| London surname prefix | 356.32, 137.06, 75.65, 66.37, 68.36 | 250 | 0 |

A further **12 cached London 25-mile requests with four concurrent callers** returned
50 occurrences each with no HTTP errors. Median 37.32 ms, nearest-rank
p95 63.66 ms. These are short closed-loop checks, not a throughput or capacity claim.

The API postcode cache was initially empty for these cases; the OS/database caches were
not reset. Initial lookup cost is visible (about 1.93 seconds for the large London circle).
Subsequent pages reuse postcode links. Earlier experiments passed all postcode IDs into
each radius-only entry query and produced roughly 400–500 ms cached pages for the dense
case; ordered 512-postcode batches reduced that work. Cache state and host activity were
not controlled enough to treat this as a formal speedup ratio.

PostgreSQL 18.6 retained its Docker **2 CPU / 1 GiB** cap. Go and the Python client ran
natively on the development Mac without CPU caps; browser tests also ran during part of
this check. Traffic was local HTTP, not TLS/WAN. Results do not establish cold-server,
long-duration, many-centre, adversarial or VPS capacity. The complete archive has not
been traversed through every possible radius query: these are first-five-page checks.

[Machine-readable aggregate results](onspd-nearby-results.json).

## Correctness and portability gates

- Synthetic reference ZIPs exercise checksum rejection, wrong positional quality,
  duplicate release rejection, transactional rollback and active-release preservation.
- Radius-only searches account for every expected occurrence through the final page;
  a separate 600-postcode fixture puts matches beyond an empty first batch.
- New edition imports invalidate cached links; incomplete displayed postcodes never
  use hidden/raw full postcodes to acquire a coordinate.
- Retired coordinates are usable; non-geographic, missing and Northern Ireland centres
  are rejected. A known ~111-metre pair independently checks the distance scale.
- Enriched JSONL exports retain original nulls and separate coordinate provenance.
- Cache tests exercise concurrent reuse, generation changes and bounded eviction.
- Browser tests cover radius-only forms, distances, quality explanations, attribution
  and bookmarked radius restoration, alongside all existing search/export flows.
- The staged native PostgreSQL cube module loaded and imported two invented reference
  points in a disposable app-managed cluster, then shut down cleanly.
