# Address index pilot and complete-edition results

17 October 2026. Follows the [lookup-service comparison](lookup-services.md).
Implementation, matching and provisioning: [address search](../address-search.md).
[Source-free measurements](address-index-results.json) include the complete reports.

## Selected layout

A shared dictionary stores normalized displayed street/locality/town/county tuples.
Each edition/tuple has a sorted integer occurrence-ID array. Original record blocks
and the existing name/postcode projections are unchanged. This supports address-only
searches, either street/locality component, trailing prefixes, combined filters,
all-edition paging and people/business types without repeating address text in every
source occurrence. House/building/subbuilding indexing is later work.

The initial 65,536-record pilots covered each edition; larger 2,097,152-record pilots
covered 2004 and 2005. Whole compressed blocks were evenly spaced deterministically.
These are clustered samples, not unbiased population samples. The 1999 sample showed
that its main thoroughfare is usually in `street_2`, so both street components must be
searched. Pilot writes are temporary and rolled back. Distinct temporary table names
prevent prepared-statement reuse from accidentally resolving permanent tables.

The larger 2005 pilot used 147,111,936 dictionary bytes and 48,775,168 membership bytes
(about 187 MiB combined). Extrapolating this linearly would substantially overestimate
full storage: dictionary reuse increases as the complete editions are scanned. Pilot
and complete timings span implementation iterations and are not controlled speed
comparisons.

## Complete builds

Build order was 1999, 2005, 2004, 1998. New dictionary keys are shared by later builds;
construction times include reading/decompressing source blocks, resolving tuples,
staging integer IDs, grouping sorted postings, analyzing tables and count checks.

| Edition | Source occurrences | Edition/location groups | Build seconds |
|---|---:|---:|---:|
| January 1998 | 27,983,545 | 666,822 | 183.07 |
| April 1999 | 46,355,850 | 804,261 | 215.07 |
| July 2004 | 63,373,974 | 785,854 | 526.20 |
| June 2005 | 51,350,897 | 856,157 | 464.35 |

All four complete sources total **189,064,266 occurrences**. Including the four
installed samples and invented demo, nine indexed datasets represent **189,077,392**
occurrences. Each source occurrence contributes exactly one tuple membership during
construction; publication checks aggregate membership cardinalities against the source
count. Source count mismatches: **0**. Source blocks are checksum-checked while read.
This is construction/count validation, not a second independent full index verifier.

| Final shared storage, including samples/demo | Bytes |
|---|---:|
| Location dictionary and its indexes | 548,413,440 |
| Sorted occurrence memberships and indexes/TOAST | 1,035,042,816 |
| Complete-index publication metadata | 24,576 |
| **Total** | **1,583,480,832 (1.475 GiB)** |

There are **1,733,324 distinct street/place tuples** and **3,120,236 edition/tuple
membership groups**. These are not distinct premises, households or people. The 2004
build added only about 1.5 MiB to the dictionary already populated by 1999 and 2005.

Relation sizes exclude staging, sort files, WAL, backups and source media. A temporary
two-integer staging row per occurrence allows disk-based grouping. Local free space
was monitored during reads and SQL work, with cancellation below a 5 GiB reserve;
about 20 GiB remained after all builds. A remote/Docker-internal filesystem still
needs its own operator monitoring.

## Query work and rejected approaches

The initial semijoin fetched source projections for all matching address IDs; broad
street and London searches reached the two-second deadline. Ordering address candidate
IDs before indexed projection lookup lets LIMIT stop those lookups after a page.
Combined queries now intersect address postings with **index-only source IDs** first.
Only page records need their full projections and original blocks fetched.

A prototype forward block-to-location map used about 210 MiB for 1999 and did not
resolve the selective-query bottleneck. It was discarded and its generated tables
removed. The retained layout needs no forward copy of the source projection.

Listing-type availability is checked with bounded ordered index probes per edition.
An EXISTS semijoin over all partitions chose a large scan during the spike; the final
query avoids that plan. Business-only address searches also intersect source IDs,
rather than testing many residential candidates before reaching business records.

Address requests have a five-second total deadline, a four-second read-only search
statement limit, and four active requests per API process. Additional admitted
requests wait within that total budget. Ordinary name/postcode queries keep their
existing limits. This bounds expensive work without truncating matches or pagination.

## Final all-edition HTTP workload

PostgreSQL 18.6 ran in Docker capped at **2 CPUs and 1 GiB RAM** on a local macOS ARM64
host. The Go API/indexer ran natively outside that cap. No internet latency was added.
Fields were repeatedly queried and caches were warm; these are not physical cold-cache
measurements. Regression tests overlapped portions of the workload. Hosted capacity
and sustained mixed-user load remain unmeasured.

Thirteen cases exercised exact/prefix streets, town, county, a large town, street/town,
name/county, prefix name/county, locality, street/postcode, business/county, a broad
business town, and an absent place. Each nonempty case checked five pages of 50:
**3,000 source occurrences**, **0 repeated occurrences across pages**, **0 returned-row
filter mismatches**, **0 HTTP errors**. Checks independently recompute predicates from
the original returned records; they do not independently prove completeness. All-edition
search excludes redundant samples and retains original edition attribution.

Observed examples, first page followed by subsequent pages:

| Case | First page ms | Subsequent page range ms |
|---|---:|---:|
| Town | 135.34 | 40.67–47.00 |
| Exact street | 919.87 | 220.04–243.57 |
| County | 661.44 | 366.70–409.99 |
| Large town | 2,265.22 | 2,122.50–2,262.46 |
| Name/county | 1,794.46 | 1,776.24–1,890.82 |
| Broad business town | 2,983.83 | 1,562.52–1,754.96 |

Four callers then made 12 repeated town requests: **29.83–132.54 ms**, zero errors,
with returned filters checked. This small repeated-key workload is not a claim that
four simultaneous national searches will have those latencies. Earlier separate
edition workloads also checked address paging/filters against original records.

To repeat locally with your own imported/indexed data:

```sh
mkdir -p private/address-index
python tools/benchmark_addresses.py --dataset all \
  --url http://127.0.0.1:58318 \
  --output private/address-index/benchmark.json
```

The tool writes counts and timings only, not source records, names, addresses or IDs.
CI uses invented records. Go race/integration tests cover multi-edition paging,
combined nearby filters, raw-field exclusion, resource rollback, post-source-commit
index failure/recovery and pilot reuse on one connection. Browser and packaged native
app checks cover address-only search, paging, bookmarks, capability coverage and import.
