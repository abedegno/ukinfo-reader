# First-name ranges with exact surname and town

The six-edition private qualification of `hosted-2026-11-14.4` passed its initial
paired probe and 1,536 sampled record comparisons, then failed four of 355 search
preparation requests. One failure combined an exact surname/town with broad
first-name matching. No edition was published by that attempt.

Bounded profiling placed most time in derived candidate selection. Custom planning,
redundant exact indexed terms, and an extra literal prefix filter did not establish
a useful improvement. They were kept as operator experiments.

The successful experiment selected the **complete** surname/town population in a
materialized CTE and applied the existing first-name predicate outside it. Twelve
full ordered-position comparisons across six editions matched, with zero errors.
On the second, reversed-order round, two slower older-edition statements fell from
233/251 ms to 1.46/1.71 ms; the selected v14 statement fell from 178 ms to 0.91 ms.
First executions of the alternative ranged from 1.9 to 87 ms. These measurements
used shared, uncontrolled NAS caches and establish neither cold acceptance nor
capacity. Aggregate evidence and private operator scripts live in homelab under
`docker/ukinfo/v14-release/`.

The serving change applies only to people searches with exact, non-wildcard surname
and town, plus a first-name prefix or multiword given-name condition. Exact
single-word first names keep their existing plan. Radius queries keep their
existing path. All other predicates and cursor bounds remain inside the candidate
selection; the shared first-name matching expression, order and final limit follow
the CTE. The inner query has **no limit or candidate cap**. Grouped pair drainage
retains its existing exact-pair query.

The tradeoff is materializing the full anchor population. Integration coverage
includes sparse matches beyond the first internal batches, given-name word
boundaries, all three ordering modes, native/derived cursor exchange, grouping,
and a separate 50,000-row exact name/town population with rare matches at its end.
CI integration and a fresh full NAS qualification must pass before deployment.
The remaining postcode failures and v14 publication require separate validation.
