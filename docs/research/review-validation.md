# Product-review implementation: validation

Local verification for the October 2026 [recommendation implementation](../review-implementation.md).
Queries, original records, response hashes and research notes remain private. This
report describes automated evidence and its limits, rather than public hosting capacity.

## Research-index build

All four complete source imports are indexed, preserving their original compressed
records. Each build checks the source-block and occurrence counts before committing
its new partition and ready marker. Build measurements use the native Go indexer on
an Apple M4 Pro development machine, with PostgreSQL 18.6 in a Docker container limited
to two CPUs and 1 GiB. The native indexer itself is outside that limit. The load test
below instead puts **both Go and PostgreSQL** inside the same limit.

| Edition                 | Source entries | Businesses | Added GiB | Build seconds |
| ----------------------- | -------------: | ---------: | --------: | ------------: |
| `se-pcf78-1998-01`      |     27,983,545 |          0 |      6.74 |         365.7 |
| `se4-pcf94-1999-04`     |     46,355,850 |          0 |     11.16 |         622.7 |
| `se2004-pcp218-2004-07` |     63,373,974 |          0 |     15.26 |        1285.4 |
| `se2005-pcf175-2005-06` |     51,350,897 |  6,152,296 |     12.67 |        1044.9 |

Total additional research-index storage: **45.84 GiB**. [Raw aggregate build measurements](research-index-builds.json).

The optional index is additional to the base catalogue, address indexes, coordinate
reference, WAL and temporary build/recovery space. Physical database size also includes
allocated/free pages; the original 38.26 GiB base-catalogue figure is not a disk budget
for every optional feature. Check actual `ukinfo-server stats` before choosing storage.

## Correctness checks on original catalogue records

[Aggregate traversal results](review-implementation-validation.json) come from
[`benchmark_research.py`](../../tools/benchmark_research.py). For bounded postcode queries
selected from every complete edition, it exhausts the established occurrence endpoint,
independently recomputes the documented listing tuple from returned source fields, then
checks every group and its paged source occurrences. Name/publication ordering traversals
must preserve the exact occurrence set and nondecreasing primary semantic order. Nearby
ordering also checks the exact occurrence set and recomputes distances from postcode
coordinates. Business classification checks compare exact/prefix/combined filters at
page size two with exhaustive business postcode-district results and their original resolved fields.

These checks establish equivalence to the existing catalogue occurrence endpoint for
the bounded cases. They are **not** a second full source-file decoding audit, an exhaustive
test of every query, proof of historical factual accuracy or a count of distinct people.
Complete-source preservation and bounded original-function decoder comparisons remain
separately documented in [the format evidence](../editions.md).

The business district pass found an actual plan problem: PostgreSQL could start with a
national industry-prefix range and then probe source IDs for a small postcode district.
The implementation now keeps selective source name/postcode/geography anchors ahead of
correlated classification checks; classification-only searches can still start from the
classification index. The original failing query and full district traversals pass after
the change. Three district reference sets contain **8,657 businesses**; one source SIC
prefix has **874 matches**, checked across two-entry pages. Unknown descriptions remain
excluded by description filters. There is no modern SIC mapping or category inference.

## Mixed HTTP workload

The selected mix has **42 private cases**: 34 search requests and 8 page exports.
It includes ordinary source pages, search-wide groups, alphabetical order, nearest
order, district-wide industry/SIC prefixes and combined business-name filters. Each
case was validated before recording its private exact response-body SHA-256; every
timed response must match. Stages run for ten seconds each.

| Callers | Requests | Errors | Requests/s | Median ms | p95 ms | p99 ms |
| ------: | -------: | -----: | ---------: | --------: | -----: | -----: |
|       1 |      744 |      0 |       74.3 |      10.3 |   27.0 |   38.0 |
|       8 |    1,800 |      0 |      179.0 |      35.5 |   94.2 |  112.0 |
|      32 |    1,883 |      0 |      184.8 |     222.4 |  308.2 |  347.1 |

The separate 30-second scheduled-arrival stage at 10 requests/s completed
300 requests with **0 errors and 0 generator drops**; p95 was
**29.0 ms**. [Raw aggregate measurements and resource samples](review-mixed-load.json).

### Wider national queries: remaining performance limits

The bounded mix above is not representative of every search. Separate national probes
retained these failures instead of treating them as empty or incomplete successful
results. Two sequential observations per class used the same backend cap after the
mixed run, without resetting caches.

| Query class                  | HTTP outcomes | Observed ms |
| ---------------------------- | ------------- | ----------- |
| common name group            | 503, 503      | 4010–4033   |
| broad prefix group           | 503, 503      | 4074–4257   |
| common name alphabetical     | 200, 200      | 1793–2593   |
| common name publication      | 200, 200      | 1572–1669   |
| national street alphabetical | 503, 503      | 4017–4025   |
| national industry prefix     | 503, 200      | 1217–2014   |

[Aggregate broad-query observations](review-broad-probes.json). The all-edition common-name
and broad-prefix grouped queries exceeded the four-second ordered-SQL budget; the
national street alphabetical query did too. One classification-only prefix hit the
ordinary SQL deadline before a later warmed request succeeded. These are real remaining
limits on this small backend, not proof that national grouping is fast. Add a postcode,
address or edition constraint; ordinary occurrence mode is available. Do not use the
selected mixed-run throughput as a service-wide capacity claim. Broad national grouping
and address ordering need further access-path work or a separately measured larger host
before promising that workload to internet users.

The host-side load generator and a small loopback nginx forwarding container are outside
the backend quota. This measures the API/SQL backend with a local HTTP hop; it does not
measure production TLS, remote network latency, a cheaper VPS's cores/storage, or edge
rate limits. TLS/quota behavior has a separate isolated acceptance test in
[`check_hosting_edge.py`](../../tools/check_hosting_edge.py). No container resource
increase is made for the benchmark. Queries are warmed and then repeated; neither the
OS nor PostgreSQL cache is flushed. Reported percentiles include failures, and failures
must be considered alongside latency. Short runs and selected private keys do not set
a service objective or establish sustainable public capacity.

## Desktop and release gates

Native package gates cover first-launch setup, invented-data import/search/export,
research persistence, cancellation/management and owned PostgreSQL shutdown. The backup
gate now starts with version-1 research and a note, verifies the upgraded model and
original-byte compatibility copy, performs a cold backup and separate restore, and checks
notes, saved queries, the compatibility copy, search results and the retained original.
This path passed locally on Apple Silicon; the release matrix checks native platform builds.

Clean customer browser-download/quarantine/install tests on actual ARM/Intel Macs,
representative usability/screen-reader studies, a full private-catalogue restore with
sufficient separate storage, and an actual selected-host sustained load/restore rehearsal
remain external acceptance work. Source publication decisions and public deployment
are separate; releases stay drafts.
