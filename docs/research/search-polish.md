# Search interface polish measurements

17 October 2026. Local development checks against the retained schema-2 catalogue:
189,064,266 complete-source occurrences across four editions, plus existing sample/demo
imports. All-edition searches exclude samples when their complete edition is available.
No source records are included in this report. Source media and catalogue contents were
unchanged; no additional catalogue or index was built.

PostgreSQL 18.6 retained its 2 CPU / 1 GiB Docker cap. The Go service and Python HTTP
client ran natively on the development Mac; neither was CPU-capped for these checks.
Traffic was loopback HTTP, not WAN/TLS. Each row below used five sequential first-page
requests (`limit=50`). Cache state was not reset; these are small warm/local checks,
not capacity estimates or a statistically rigorous comparison. Machine-readable aggregates
are in [search-polish-results.json](search-polish-results.json).

| Query, across all editions | Median ms | Maximum ms | Returned occurrences | Editions on page |
| --- | ---: | ---: | ---: | ---: |
| Exact common surname | 7.47 | 26.09 | 50 | 3 |
| WS13 district | 7.57 | 19.12 | 50 | 4 |
| WS13 7 sector | 6.12 | 6.98 | 50 | 4 |
| Smi* surname prefix | 24.45 | 31.56 | 50 | 3 |
| W11 district | 7.94 | 32.05 | 50 | 4 |
| W1 1 sector | 4.09 | 6.98 | 19 | 1 |

A further 60 closed-loop requests cycling these six cases with four concurrent callers
completed without HTTP errors: nearest-rank p95 **35.35 ms**. This short check does not
measure sustained throughput, cold reads, high-cardinality worst cases or public hosting.
The initial implementation rejected a broader two-character surname prefix at an
8,192-term vocabulary expansion limit. That restriction was subsequently removed:
broad expansions now stay inside PostgreSQL and feed the same result cursors. See
[broad-prefix pagination checks](broad-prefix-pagination.md). The table above records
the original polish measurements, before that follow-up.

An initial district query took about 262 ms. Ordering postcode-prefix results by the
existing `(dataset, kind, postcode, name, first, ordinal)` index removed the need to sort
all matching occurrences by name before selecting a page. Final all-edition searches
interleave editions using bounded per-edition index scans before a global top-N merge.
A direct unbounded global sort was rejected after common-surname requests hit the query
timeout. The retained implementation avoids that sort. Cache effects are uncontrolled,
so the initial and subsequent measurements must not be treated as a speedup ratio.

## Correctness gates

- PostgreSQL integration tests use invented district/sector collisions (`W11` vs `W1 1`,
  `WS1` vs `WS13`), source partial postcodes, and multiple editions.
- Prefix and all-edition keyset paging checks account for each occurrence exactly once;
  source IDs can repeat across editions without collisions.
- Samples of the same edition are excluded when complete imports exist.
- Browser tests cover actual Go/PostgreSQL search, page-size changes, bookmarked page
  restoration, source labels, readable record details, original-record and CSV downloads.
- Synthetic UI grouping checks retain both edition citations and keep incomplete addresses
  separate. Catalogue-wide deduplication is deliberately not claimed.
