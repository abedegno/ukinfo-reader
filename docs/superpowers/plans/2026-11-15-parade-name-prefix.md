# Measured all-edition name-prefix route

Retain PostgreSQL as source authority. Route only all-edition person searches
with a surname prefix, one-word non-wildcard given name and default ordering to
the existing ParadeDB result path. Exclude address/postcode/radius, research,
house/building/flat/middle and business filters; preserve every other route.

The private source-cold ABBA returned identical complete records: native
1222/1208 ms, target 310/231 ms. Source files were zero-resident before each
private restart; connections/metadata were prechecked, target cache uncontrolled
and not evicted. This supports a candidate change, not release acceptance.

1. Add routing tests, observe RED, implement the narrow eligibility predicate.
2. Extend existing real-ParadeDB invented-data integration to assert actual route
   use, complete 35-record multi-edition population, word-boundary rejection,
   deep pages, tied record IDs and native/derived cursor exchanges. Keep full
   source record comparisons and exact exhaustion.
3. Run local unit/source PostgreSQL checks and required CI including pg_search;
   independent review then merge exact reviewed source with immutable release.
4. Use a fresh versioned build/qualification epoch with unchanged five-edition
   cases/deadlines/numeric gates. Only accepted evidence can permit rollout;
   then fresh six-edition qualification, selected v14 cold controls and separate
   guarded data publication follow. v14 is still hidden.
