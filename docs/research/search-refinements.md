# Name and premises refinement validation

18 October 2026. [User/API guide](../search-refinements.md).

No migration, additional index, source copy or full-catalogue rewrite was required.
The candidate search was extracted into a shared typed result function so ordinary
searches, displayed-field refinement and exports use the same query and source loading.

## Correctness and UI

Invented PostgreSQL integration fixtures check:

- More than 300 nonmatching candidates before qualifying house/middle-initial matches.
- Complete, duplicate-free paging across candidate boundaries and two editions.
- Numeric/text house numbers, blank house zero, suffixes, flat/building exact/prefix
  filters and unavailable/raw-only fields.
- Independent first-name exact/prefix/given-name-boundary matching; second-token initials.
- Full/district postcode, optional street index, person/business searches, changed-filter
  cursor rejection and server exports preserving an integer above JavaScript's safe range.

Browser checks exercise controls, bookmark reload, paging, Back, filter removal/reset,
zero-result actions, missing-anchor guidance and a 390-pixel viewport. Building/flat
controls also work when the optional address index is unavailable. The native packaged
app check imports invented data and searches with combined house/given-name filters.
Go race tests and vet, the complete browser suite and packaged app checks are run locally.
CI repeats invented backend/browser checks on PostgreSQL 18.

## Representative full-catalogue checks

[Aggregate report](refinements-validation-results.json): **36 scoped walks**, **60 pages**
and **108 returned occurrences**, including overlapping queries. These are not counts of
unique people. One bounded person anchor per complete edition, plus a 2005 business
anchor, supplies house and available given-name/middle-initial cases. Each refined
result set at page size two agrees with independently filtered exhaustive same-kind
full-postcode results, for both the selected edition and all editions.

Observed page times were **2.31–69.16 ms**. PostgreSQL 18.6 ran with a Docker limit of
2 CPUs / 1 GiB RAM; native Go and Python were outside that cap. These sequential local,
warm checks do not establish cold-cache performance, broad national-query performance,
concurrency or hosted capacity. Selected real anchors did not exercise building/flat
fields; those filters are covered by invented integration/browser fixtures.

The initial check exposed a timeout in an existing all-edition full-postcode/given-name
query. Extending postcode-leading ordering to exact postcodes removed that failure in
the repeated comparison. Do not interpret this observation as a controlled speedup
ratio: cache state was not reset.

```sh
python tools/benchmark_refinements.py --url http://127.0.0.1:58318 \
  --output private/address-index/refinements-validation.json
```

The read-only tool retains only aggregate counts/timings and parameter names. Original
query values, source IDs, names, addresses and source records are not written to its
report or progress output.
