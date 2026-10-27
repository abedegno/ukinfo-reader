# Research and measurement records

These reports preserve design decisions and observations from particular versions,
hardware and datasets. Their proposals may already be implemented or superseded;
use the [current documentation](../README.md) for setup and supported behaviour.
Short warm tests do not establish cold-storage or public-service capacity.

## Storage and query execution

- [Source-cold postcode/nearby searches and concurrency follow-up](cold-search-2026-10-26.md) — complete native references, fixed workload, identified business-sort tail and repeated C4 checks.
- [Grouped, edition and alphabetical postcode search follow-up](ordered-search-2026-10-26.md) — independent source references, fixed workload, concurrency and source-cold checks.
- [Equal-budget hybrid control and hosted rollout](parade-rollout-2026-10-26.md) — complete fixed cohort, remaining tails and deployment evidence.

- [Hybrid routing, complete workload and measured cold-cache follow-up](parade-hybrid-trial-2026-10-26.md)
- [All-edition ParadeDB serving trial, concurrency and offline rollback](parade-serving-trial-2026-10-26.md)
- [Full 2003-edition native PostgreSQL / ParadeDB comparison](real-search-spike-2026-10-26.md)
- [Synthetic PostgreSQL search-index comparison](postgres-search-spike-2026-10-25.md)
- [Storage backend spike](search-storage-spike-2026-10-17.html)
- [Compact PostgreSQL catalogue and full-edition measurements](compact-catalogue.md)
- [Address index](address-index.md)
- [Broad-prefix pagination](broad-prefix-pagination.md)
- [Full-catalogue query workloads, stage costs and capacity](query-workload-2026-10-21.md)
- [Bounded address probes and the full 2003 hosted projection](hosted-projection-2026-10-22.md)
- [Town/county postings and business-only classification projection](place-business-spike-2026-10-22.md)
- [Business classification API integration and full HTTP acceptance](business-api-2026-10-22.md)
- [Adaptive query-planning review](query-planning-2026-10-19.md)
- [Feature validation and national-query limits](review-validation.md)

## Search experience

- [Lookup-service research](lookup-services.md)
- [Search polish](search-polish.md)
- [Postcode and nearby search](onspd-nearby.md)
- [Names and premises refinements](search-refinements.md)
- [Premises exploration](premises-exploration.md)
- [Product review, 19 October 2026](product-review-2026-10-19.md)
- [Product-review implementation follow-up](../review-implementation.md)

Reports and result files contain aggregate measurements. Private query fixtures,
responses and source media are excluded from this repository.

- [Tail latency and broader search coverage](tail-search-2026-10-27.md): target plan reuse, bounded surname selection and complete large-radius streaming; frozen original and separate extended suites.
