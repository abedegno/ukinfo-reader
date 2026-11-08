# Hosted selection and import of v14

The raw archival reader validates 43,799,502 v14 source occurrences. The hosted
selection is explicitly different: the disc's own `data_sources.htm` describes CO1
as the publisher's record-removal request mechanism, and its original constructor
names postcode control bit `0x40` CO1. The bit's physical decoding was checked
against original routines. Complete Windows UI filtering has not been executed;
the hosting policy below is explicit rather than an inferred UI equivalence.

The [complete audit](validation/v14-2008-03-hosted-co1-selection.json) counts
29,410 flagged physical records supplying 54,869 occurrences. The hosted export
excludes every flagged residential occurrence, including both electoral and
telephone components. The archival reader and original files remain unchanged.

| Scope | Source | Excluded | Hosted |
| --- | ---: | ---: | ---: |
| Electoral | 26,963,431 | 40,694 | 26,922,737 |
| Residential telephone | 10,744,063 | 14,175 | 10,729,888 |
| Business | 6,092,008 | 0 | 6,092,008 |
| Total | 43,799,502 | 54,869 | 43,744,633 |

## Export contract

```sh
python -m ukinfo_reader.search_export \
  --data private/v14-reader --selection exclude-source-co1 --identify
```

The streaming command uses the same arguments without `--identify`. It emits
protocol 2 with dataset key `v14-2008-03:co1-excluded`, an explicit `selection`
object, original file fingerprints, selected type counts and a fresh digest over
only selected rows. Unknown listing types, missing/non-boolean residential CO1
flags, source counts, exclusion counts or emitted counts that differ from the
verified policy prevent a valid terminal frame. The Go importer rolls back an
incomplete stream. Explicit samples retain their sample labels and cannot satisfy
the publication checks. The ordinary CLI and default archival export remain raw.

No intermediate complete JSONL file is needed. The NAS importer pipes selected
rows directly into the existing Go catalogue importer, retaining original source
IDs and source flags. Complete catalogue verification checks that stored blocks
and projections match the selected stream's digest. No independent identity
matching or retrospective cross-edition removal is inferred from this source bit.

## Hosted status and acceptance

**8 November — latest:** the isolated [postcode-partitioned index pilot](research/postcode-index-layout-pilot-2026-11-08.md)
passed its fixed gate on one million 2003 rows: first-query median 2.02 to 0.96 s,
private-container reads 700 to 351 MiB, 64 successful queries and 25 matching page
comparisons. The [full-edition follow-up](research/full-edition-postcode-layout-2026-11-08.md)
completed its full export but stopped on a helper OOM during baseline import.
No performance measurements ran in that attempt. A distinct recovery with bounded
COPY buffering imported both full tables. Its surviving NAS build was reattached
after a Mac crash; baseline indexing is complete, with candidate verification/indexing
still pending. Performance results remain pending. The live BM25 query/indexes remain unchanged and v14 stays hidden.

Earlier investigation reports retain the [failed I/O reproduction](research/postcode-io-profile-2026-11-08.md),
the [cache-confounded Bitcoin comparison](research/bitcoin-contention-comparison-2026-11-08.md)
and [rejected numeric-ID rewrite](research/postcode-io-comparison-2026-11-08.md).
Bitcoin is running. The NAS has no SSD pool; no storage migration occurred.

As of 7 November 2026, the complete selected source and derived indexes are
committed on the NAS. All 1,536 spread field samples match both original records
and independent source projections across six editions. v14 remains private while
full search qualification runs; the public service has **five editions and
248,951,107 source occurrences**. Publishing the selected v14 would bring it to
**six editions and 292,695,740 occurrences**. These are listings, not unique people.
The existing editions and hidden 2003 rollback copy are retained.

The [aggregate progress report](validation/v14-2008-03-hosted-progress.json) records
completed checks and every earlier failed attempt. No failed run is treated as
passed acceptance.

| Required check | Current result |
| --- | --- |
| Complete selected import and source digest | Passed: 43,744,633 entries |
| Source address/research/business and derived indexes | Complete |
| Direct and independent field samples | 1,536/1,536 matched |
| Exact-image invented-data page/cursor equivalence | 374 comparisons passed |
| SQL decoding boundaries and source-generation invalidation | Passed |
| Two targeted shapes and their continuation pages | Four matching pairs; zero errors |
| Frozen full cohort, up to ten pages per case | Passed: 355 cases, 896 matching pairs, zero errors/mismatches |
| Cursor continuation and query binding | Failed: one native and one candidate error; two binding checks rejected correctly |
| Full-cohort calibration | Passed: 355 cases, zero errors/mismatches |
| Existing-edition reference hashes | Failed: 247/250 matched; three query timeouts |
| Two fixed candidate load rounds at concurrency 1 and 4 | Not qualified: diagnostic measured rounds met limits, but initial cohort had one error |
| Publication and real release-client checks | Required: public inventory, 12 internal searches and 77 release checks |

Public/candidate deadlines, quotas, resource limits, serving executable and normal
load client remain unchanged. Each API has 1 CPU/512 MiB; each PostgreSQL service
has 2 CPU/6 GiB. A failed gate prevents publication. A post-publication failure
attempts transactional hiding before reading inventory, then verifies the original
five-edition baseline. Unconfirmed recovery is reported explicitly.

## Independent private reference

The slow native source reference is a correctness oracle, with no published port,
administrative credentials or candidate data access. It reads the complete source
address population, independently applies source geography where needed, and
preserves the original native predicates, grouping, ordering and cursor semantics.
Invalid, incomplete, duplicate or oversized populations fail closed; results are
never truncated to meet a deadline. Source indexes must cover every selected row.

For the two broad town query shapes, preparation reads a consistent, read-only
repeatable-read snapshot. Its compact projection contains every matching person
occurrence across the six editions, using source labels and group keys. Source
metadata is checked after preparation and before use; changes invalidate the
reference. No candidate result, record ID or response hash selects this population.

The current preparation covers 1,179,789 occurrences in 103,789,874 accounted
bytes, within a 128-MiB cache cap, and took 540.743 seconds against a 600-second
bound. Six projection columns use 22 fixed-width wire bytes per occurrence rather
than 46 in binary arrays; PostgreSQL decodes them before the original native SQL.
A checksum-guarded private dependency overlay clears retained parameter references.
An invented-data regression demonstrated 4,194,308 retained bytes before the fix and
zero afterward. Protocol lifetime safety, integer boundaries, all 374 synthetic
pages/cursors and four real targeted pairs passed on the exact private image.

The native SQL/request deadline is 120 seconds and its client deadline 130 seconds;
the candidate client remains eight seconds. The source-only URI uses custom query
planning, proven in a nonprivileged read-only session. No role default, public
setting or candidate access path changes. Full comparison uses two disjoint,
ordered batches of 178 and 177 cases, preserving every original case, request
alternation and page limit. Each comparator must exit successfully and report
success; merged percentiles are computed across all observations.

## Retained validation history

| Attempt | Retained outcome |
| --- | --- |
| Initial legacy check | 245/250 matched; five timeouts |
| Initial fixed warmup | 355 requests per backend; native 29 errors, candidate three |
| Fresh initial legacy check | 250/250 matched |
| Initial paired/calibration checks | 24 native errors in 742 pairs; seven native calibration errors; no candidate errors or mismatches |
| First slower reference | Five native errors in 851 pairs; six preparation errors and one legacy timeout |
| Complete source-address reference | Two native timeouts in 878 pairs; two preparation errors; no candidate errors or mismatches |
| Request-bound projection and custom-plan probes | Two native 120-second failures in each probe; no full qualification |
| Prepared snapshot, first memory attempt | Native OOM; archived 473 pairs, 264 native errors and one candidate error |
| Prepared snapshot, lower soft-memory attempt | Native OOM on grouped case 91 page four; 475 pairs, 211 equal, 264 native errors; zero candidate errors/mismatches; seven preparation errors |
| Packed transport | Source preparation, 1,536 samples and four targeted pairs passed; three preparation errors; full run operator-stopped for driver review, not OOM-killed or fully qualified |
| Private driver fix | Exact-image synthetic, source and targeted checks passed; all 896 paired pages matched and calibration passed; cursor and legacy errors block publication |

The earlier source proofs, failed observations, stopped containers and partial
reports remain on the NAS. Real queries, records, IDs, coordinates, cursors and
response hashes stay there; repository reports contain aggregates only.

Measurements use shared NAS caches and the prescribed cohort preparation. Candidate
serving performance is measured with the unchanged load executable after strict
correctness passes. Six-edition cold performance and sustained Internet capacity
have not been qualified; the original isolated five-edition cold fixture is
unchanged. Homelab holds the reproducible import, validation, publication and
recovery controls.

## Search index and remaining diagnostics

ParadeDB BM25 remains the hosted search index. The independent native reference
uses the source catalogue to verify its results; tuning that reference is separate
from tuning the serving path. No additional source covering index was built during
these diagnostics.

A complete source street population contains 750,243 occurrences (647,376 people).
Warm source SQL experiments returned identical 51-row ordered results with per-row
and batched dictionary lookups, taking 11.646 and 7.625 seconds respectively. A
separate parameterized-array experiment measured custom planning/execution at
0.402/3.408 seconds and generic planning/execution at 0.046/4.136 seconds. These
are diagnostic equivalents, not full API acceptance measurements. PostgreSQL
reported 360,322 entry-scan block reads; these can be served from the operating
system cache and do not establish physical-disk bytes read.

The serving API's existing cumulative metrics contain three failed record-loading
phases and two failed ParadeDB-candidate phases. These counters identify affected
operations; they do not map every failure to an individual request. Further work
must address those serving failures as well as the remaining reference error.

A later one-pass phase probe reproduced a serving failure on `/api/address` with
all-edition scope: five record loads succeeded in 3.671 seconds total before a
sixth hit the five-second request deadline. Another all-edition address request
spent 3.801 seconds in seven record loads and completed near the deadline. The
main BM25 search in that pass spent 0.801 seconds selecting candidates and 0.021
seconds loading records. The helper used 0.1 CPU, so client timings are diagnostic
only; this pass is not acceptance or a replacement for the failed runs.

Address views currently scan each edition's postcode candidates sequentially and
check full source records against the anchor's premises identity. This path is
separate from BM25 person/business search. It needs a serving-path optimisation;
an index used only by the independent verifier cannot establish that it is fixed.
