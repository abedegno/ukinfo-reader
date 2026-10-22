# Business classification API integration — 22 October 2026

This integrates the smaller business projection from the
[preceding spike](place-business-spike-2026-10-22.md) into ordinary API searches.
[Aggregate evidence](business-api-2026-10-22-results.json) contains no private
queries, records, IDs, cursors, hashes or credentials.

## Implementation

The optional `business_search` table stores only business search/classification
keys, grouping identity and canonical address references. Its edition partitions
have primary-key and covering nonempty SIC/industry indexes. The owner-only
builder derives them from complete source/research/address indexes, checks the
business count, attaches the partition and publishes versioned completion metadata
atomically. A failed build rolls back. Reapplication is idempotent. The original
compressed record blocks and displayed fields remain authoritative.

All five published NAS editions were prepared. There are 6,152,296 businesses in
2005 and 2,471,038 in 2003; the other three editions have verified empty builds.
Together the partitions occupy 1,598,152,704 bytes (about 1.49 GiB), plus small
parent/metadata overhead. This is additional production storage.

Only ordinary business searches with SIC/industry filters and complete relevant
edition coverage use the new path. Missing privileges, missing/unfinished/mismatched
metadata or a detached partition retain the complete existing path. Name,
postcode, canonical street/locality/town/county filters, original-record retrieval,
refinements, ordering and cursor semantics are preserved. Grouped, specially
ordered and nearby queries continue on their existing paths.

The preparation also exposed an existing mixed-edition issue: address readiness
was checked before excluding editions containing no listings of the requested
kind. That check now applies to relevant editions, consistent with research-index
readiness. A people-only edition cannot block a business address search merely
because it lacks an irrelevant address index.

## Full HTTP acceptance

Two unexposed APIs used the same SELECT-only serving role and production-sized
catalogue. Baseline used hosted `.4`; candidate used the integrated code. Existing
2 CPU / 6 GiB database, 1 CPU / 512 MiB API, eight-connection pool, request/SQL
budgets, admission controls and source indexes were retained. There were no
parallel builds during measurement or cache flushes.

The 109 cases cover 2003, all editions, sampled positive 2005 classifications,
empty-business editions, address/postcode/name combinations, exact/prefix
classifications, displayed-field refinements and existing ordering/group fallback.
Each case follows up to three 50-record pages, with alternating baseline/candidate
order and each server's own cursor. Successful complete response bodies and page
availability are compared.

| Fixed first-page cohort | Attempts | Baseline errors | Candidate errors | Baseline p95 | Candidate p95 |
|---|---:|---:|---:|---:|---:|
| Whole workload | 109 | 21 | 0 | 4,416 ms | 710 ms |
| Guaranteed-positive 2003 cases | 18 | 2 | 0 | 4,416 ms | 273 ms |
| All-edition scope | 39 | 16 | 0 | 5,003 ms | 825 ms |
| Guaranteed-positive 2005 cases | 18 | 1 | 0 | 4,311 ms | 479 ms |

Across 189 page comparisons, the baseline had 25 errors and the candidate none.
All 164 mutually successful comparisons matched. Candidate-only successes do
not independently prove source correctness; invented full-body/cursor tests,
source-count validation, positive sampling and the following independent reference
test provide complementary checks. The workload completed within its explicit
request/time budget. Later-page populations differ after failures, so their
aggregate latency is secondary evidence.

## Bounded concurrent correctness

A separate operator captured current-API response hashes for 24 positive 2003/2005
queries, retaining no response records. Candidate requests were verified against
those independent hashes. Each concurrent stage lasted eight seconds:

| Clients | Requests | Errors or mismatches | Median | p95 | Observed requests/s |
|---|---:|---:|---:|---:|---:|
| 4 | 208 | 0 | 112 ms | 450 ms | 25.5 |
| 8 | 212 | 0 | 286 ms | 569 ms | 25.6 |

Including initial reference checks, 444 candidate requests passed. These are
short, generally warm, private-network measurements, not public Internet or
cold-HDD capacity guarantees. Cache state, checkpoints and unrelated NAS activity
were uncontrolled. The public service retains its quotas and admission limits.

## Regression gates and operations

- Full race-enabled backend suite; complete API body/cursor comparisons across
  prefixes, combined fields, relevant editions, refinements and fallbacks.
- Actual API SQL under custom/generic plans: one correctly pruned business
  partition, covering index-only access, zero heap fetches, correct page size and
  a 1,000-buffer work ceiling.
- Schema/index-shape validation; missing research rows reject a build and roll back
  its partition and metadata; complete builds can be reapplied.
- Mismatched edition metadata and detached partitions fall back. Edition removal
  verifies the attached business partition and removes its metadata; catalogue
  storage accounting includes the new data.
- Owner-only SQL construction, SELECT-only serving, no new public endpoint,
  no source rewrite, no total-match cap and no increased timeout or quotas.

Build with `index-business-search` or the reproducible
[owner SQL procedure](../../deploy/production/business-search.sql). For rollback
or controlled comparisons, `serve --disable-business-search` selects the existing
classification path without removing source or projection data.
