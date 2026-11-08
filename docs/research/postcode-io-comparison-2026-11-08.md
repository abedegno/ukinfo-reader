# Postcode-ID I/O comparison — 8 November 2026

The private spike compared the existing BM25 postcode query with complete sets
of authoritative source postcode IDs. Production query building, order, cursor
predicates, 51-candidate lookahead, generic-plan policy and source/target 2/4-second
SQL budgets were retained. The Go code was a private build overlay, never deployed.
Bitcoin stayed running. No cache flush, database restart or resource change occurred.

## Fixed protocol

Consider five frozen failing cases and up to 24 additional postcode districts,
selected at four spread source-entry positions per edition across all six editions.
Admit only complete vocabulary sets of at most 1,024 IDs; count and skip overflow
rather than truncating. Alternate ABBA/BAAB order by case. Bracket each query with
container I/O counters and collect actual prepared EXPLAIN work. Only after all
measurements compare both ordered queries across up to three real page boundaries.

Twenty-three cases qualified: four frozen and 19 additional; six exceeded the cap.
There were 92 measured executions. All 69 later page comparisons matched, without
verification errors. Those warmed comparisons do not erase earlier timed failures.

| Variant | Measured queries | SQL timeouts | Median | p95 | Total derived reads |
| --- | ---: | ---: | ---: | ---: | ---: |
| Existing text predicate | 46 | 0 | 99 ms | 2,040 ms | 1,181 MiB |
| Complete postcode-ID set | 46 | 6 | 971 ms | 4,225 ms | 2,022 MiB |

Numeric timings conservatively include the separately measured source-resolution
cost (24 ms median). These are instrumented candidate queries, not complete HTTP
requests. All six failures were SQLSTATE `57014`. Successful plans for both variants
remained generic Top-K plans; failed statements have no complete plan work counters.

Among the 18 cases where every measured execution succeeded, paired logical
buffer work was 253,476 blocks for the existing query and 144,378 for IDs (43%
lower). Failed plans are excluded from **both** sides of that comparison. Counting
their unavailable buffers as zero would overstate the apparent improvement. A
regression test catches that analysis error; the initial summary is retained.

Balanced first executions read 866 MiB for the original query (12 cases) versus
735 MiB for IDs (11 cases), about 15% lower, below the predeclared 25% threshold.
Each variant had sufficient nonzero-read evidence, but populations and cache state
were not identical. Container deltas include other target work. Background Bitcoin
read volume also varied; these figures do not prove a universal performance ratio.

**Decision:** reject the ID-set serving change. It failed error, latency and
physical-read criteria. Do not run the conditional full workload or ship a rewrite
on the strength of reduced logical buffers. Existing BM25 and its query remain.

## Storage assessment

The existing derived PostgreSQL volume occupies **61,174,796 KiB (58.34 GiB)** as
reported by `du`, including its current data/index/WAL allocation. It is on the
shared HDD RAID. Hardware inventory shows four rotational 16-TB drives and only
one 128-GB NVMe device with a system-partition layout. The user confirms there is
**no SSD pool**. No spare SSD data pool was found, created or assumed.

UGREEN lists a built-in 128-GB system SSD and two additional M.2 slots supporting
a separate SSD storage pool for the documented DXP4800 Plus. The system SSD is not
an available data-volume destination. See the [manufacturer specification](https://nas-uk.ugreen.com/products/ugreen-nasync-dxp4800-plus-4-bay-nas).

A future dedicated SSD data pool could isolate derived-search reads from HDD
traffic, but no improvement has been measured on SSD here. Buying hardware is
optional, not an established requirement. Continue investigations on the current
storage; do not repurpose the system disk or change services without evidence.

If a data SSD is added later, the migration should preserve the same PostgreSQL/
extension image, CPU/memory caps, permissions and reader roles. Confirm the real
mounted pool path and headroom first. The current 58-GiB footprint needs room for
WAL, growth and rebuilds; 150 GiB free is a conservative planning allowance, not a
measured minimum. Keep the original source on HDD and retain the old derived volume.
A physical copy requires a cleanly stopped derived server; never copy a live data
directory as if it were a valid database. Do not combine a storage move with an
extension upgrade or v14 publication. Validate privately on the destination before
promotion, and retain a rollback path to the original volume. No such move is
currently scheduled or authorised by this report.

## Evidence and closeout

Homelab preserves the runbook, tests, private Go overlay and operator recipes in
`docker/ukinfo/postcode-io/`. The NAS-only image built in 213.3 seconds at 2 CPU/
2 GiB. Target/source database caps stayed at 2 CPU/6 GiB; the diagnostic used
1 CPU/512 MiB. No local Docker image or database copy was created.

All query values, original rows, IDs, cursors, hashes and raw plans remained NAS.
Reports are retained under `/home/jonw/ukinfo-backups/postcode-io-2026-11-08`.
The owned diagnostic is stopped and lease released. Production is unchanged at
five editions / 248,951,107 occurrences; v14 stays hidden. Earlier failures remain
failed. No full release qualification is claimed.
