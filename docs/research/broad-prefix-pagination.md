# Broad-prefix pagination

17 October 2026. Follow-up to the [search interface polish](search-polish.md).

The initial prefix implementation expanded matching dictionary terms into a small Go
integer array and rejected searches above 8,192 terms. That count covered shared
vocabulary, including terms without matching listings of the requested kind. It was
not a count of matching records and should not have been a user-visible search limit.

The fix retains the small-array path but sends broad expansions through a PostgreSQL
materialized CTE containing the complete matching integer array. All edition branches
share that expansion. Each edition still supplies at most `limit+1` indexed candidates;
the global merge selects the page and its continuation cursor. The vocabulary is not
truncated, and result paging retains the same ordering and cursor format. Query timeouts,
API admission limits and the maximum page size remain in place. No index, migration,
source-media copy or catalogue copy was required.

The complete dictionary expansion is computed inside PostgreSQL on each request; this
is result pagination, not a cache or a claim that dictionary work is constant for every
prefix. Very broad searches still cost more than exact searches.

## Correctness

The PostgreSQL integration regression creates 8,300 irrelevant dictionary terms ahead
of 130 matching names, then follows eight 17-result pages to exhaustion. It checks all
130 occurrences are returned once. It also checks a broad first-name prefix against
another 8,300 irrelevant terms and a broad prefix with no matching entries. These
fixtures would trigger the previous rejection.

Against the retained full catalogue, five successive 50-result pages were checked for
each case below. All returned 250 distinct source occurrences with matching filters and
further results available. No directory records are stored in the report.

| Query across all editions | Observed page times, ms |
| --- | --- |
| Surname `Sm*` | 60.64, 17.52, 33.44, 28.74, 26.05 |
| Surname `Jo*` | 66.35, 53.95, 71.67, 55.69, 68.50 |
| Surname `Smith`, first name `J*` | 233.13, 157.77, 177.75, 245.42, 211.75 |
| Business name `Th*` | 46.29, 39.45, 43.89, 39.13, 53.87 |

[Machine-readable aggregates](broad-prefix-pagination-results.json).

These are sequential warm/local HTTP checks against PostgreSQL 18.6 with the existing
2 CPU / 1 GiB Docker cap. Go and the client ran natively without a CPU cap. Cache state
was not reset; these checks do not establish cold, concurrent, WAN/TLS or hosted capacity.
All four complete source editions remain present; redundant samples are excluded from
all-edition searches. The full-catalogue check covers the first five pages, whereas the
invented integration fixture checks complete enumeration through the final page.
