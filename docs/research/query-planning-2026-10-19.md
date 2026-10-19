# Query-planning review — 19 October 2026

## Scope

Follow-up to a name/town search that exceeded the NAS query deadline on uncached HDD
reads. This change improves combined location searches, candidate selection and
repeated refinement work. The Go/API deadlines, public quotas, database privileges
and indexed catalogue representation stay unchanged. No additional database/index
storage or source re-import is needed.

## Implemented

1. Probe the address index for a complete small candidate set before expanding town
   postings. Up to 1,024 complete address candidates can constrain the source query.
2. Permit name, postcode, nearby and classification anchors to drive address checks.
   A source probe must be complete within 4,096 occurrences and 128 distinct blocks.
   Packed postcode entries can therefore qualify above the previous 128-entry cutoff.
3. Bound record allocations by reading at most 100 candidates per batch, in source-ID
   order to reuse adjacent blocks. Overflow uses the full indexed intersection;
   it never returns a truncated successful search.
4. Reuse decoded blocks within one HTTP request: at most 4 MiB and 128 blocks. Reuse
   one candidate plan across that request's refinement batches. Neither cache persists
   after the request or shares records between visitors. Different filters cannot reuse
   a candidate plan; each batch retains its own cursor and page size.
5. Apply given-name and middle-initial checks while resolving small first-name
   vocabularies, and premises refinements during selective address checks. Large
   vocabularies keep the complete SQL expansion and source-field verification.

## Verification

Synthetic PostgreSQL/race checks cover candidate-budget fallback, packed source blocks,
address-only and postcode/address searches, sparse premises filters, business listings,
all-edition paging, grouping and ordering. Cache tests cover byte/count bounds,
eviction, edition/request isolation, actual block-read reuse and candidate-plan reuse.
Surname-window traversals check every listing across pages, including unused dictionary
terms. Existing source-preservation and projection tests remain separate.

All 42 bounded search/export cases returned byte-identical responses to the retained
reference API after the final change. One historical nearby fixture hash differed on
both APIs, whose current responses matched each other; the historical hash was not
rebaselined. Full Go race/integration tests and static analysis passed.

Private full-catalogue comparisons retain query values and source responses outside
Git. Benchmarks are sequential observations with ordinary caches, not controlled cold
comparisons, sustained-load measurements or promises for every NAS search.

## National grouping and sorting remain limited

The review reproduced deadline errors for national common-name grouping, broad-prefix
grouping and national street alphabetical ordering. A dictionary-first name-pair
experiment did not meet the performance gate and was removed. The established ordered
query path is retained. Its national grouping still needs many research-key reads;
request-local block reuse does not eliminate that work.

Further work needs a measured access-path/index design or storage change. Broad-query
errors remain explicit; this change does not raise deadlines or present partial scans
as complete results. The earlier [wider-query measurements](review-validation.md#wider-national-queries-remaining-performance-limits)
remain relevant as historical evidence, not a fresh capacity result.
