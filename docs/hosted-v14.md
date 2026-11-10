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

**9 November — latest:** the [separate 2003 HTTP qualification](research/2003-http-qualification-2026-11-09.md)
confirmed fresh Top-K/generic plans and all 635 frozen response hashes. C1 p95
passed at 370 ms; C4 p95 missed its 1 s target at 1.306 s. The 127-case subset
includes 39 business cases but no HTTP cursor cases; other editions remain outside
this check. All private services/lease are closed and quotas restored. No live
index change or v14 publication occurred. The [fixed tail profile](research/2003-http-tail-profile-2026-11-09.md) then
reproduced C4 p95 at 1.330 s with all responses correct. Candidate-query time and
private PG CPU throttling dominate that profile. The subsequent
[CPU-budget experiment](research/2003-cpu-budget-2026-11-09.md) stopped after one
HTTP 503 in its first control; no higher-CPU arm ran. That control also showed
substantially more database reads. The [single-query plan inspection](research/2003-ordered-query-plan-2026-11-09.md)
then confirmed Top-K with costly columnar work on a broad street-prefix query.
The [private street-prefix spike](research/2003-street-prefix-spike-2026-11-09.md)
passed 28 synthetic cases and three real pages; its fixed SQL observations fell
from 604/558 ms to 178/195 ms. This supports a serving implementation trial,
with unchanged HTTP/cursor and broader edition qualification still required.
The [serving implementation](research/2003-street-prefix-http-2026-11-09.md)
then passed all 635 frozen responses and synthetic/full candidate paging checks,
but C4 p95 remained 1.299 s. The preserved baseline timed out on a diagnostic
continuation page; full real-page comparison remains incomplete. PR #48 is a draft;
no deployment or v14 publication is qualified. The [subsequent SIC trial](research/2003-sic-tail-2026-11-09.md)
kept all 635 responses and street/SIC paging correct but still failed C4 at 1.485 s.
Its extra SIC runtime predicate was removed; the expanded regression tests remain.
Actual SQL and paired plans showed mixed benefit, and versioned engine source
corrected the earlier interpretation of deferred query-expression nodes.
The [existing-route SIC comparison](research/2003-native-sic-comparison-2026-11-09.md)
then found nine native failures in 66 searches versus zero for ParadeDB; nine
page pairs matched. At unchanged existing database limits, ParadeDB C4 p95 was
405/405 ms versus native 4,567/4,648 ms on these three selected cases. This small
diagnostic supports retaining ParadeDB routing, not publishing v14 or qualifying
the different private index layout. Next establish the full frozen 2003 workload
on the existing target at its operating limits.
That [full 2003 operating-budget run](research/2003-operating-budget-http-2026-11-09.md)
completed all 635 requests: C1/C4 p95 passed at 120/230 ms, but one initial
postcode-only person search in name order timed out. The overall gate remains
false; 634 successful responses matched. Next inspect that request path's actual
SQL/plan before another serving change. Production and v14 visibility unchanged.
The [postcode/name SQL diagnostic](research/2003-postcode-name-plan-2026-11-10.md)
then confirmed the existing prefix constraint and Top-K with a warm113ms plan.
Warm candidate selection took156–234ms versus8–9ms record loading; original
timeout cause remains unknown. Two instrumented requests and one analysis do
not qualify a release. No SQL change, database restart or deployment occurred.
The [matched-budget private layout test](research/2003-postcode-layout-matched-2026-11-10.md)
then compared retained full copies at2CPU/6GiB in one private PG, restarting only
it before each arm. Unpartitioned timed out once and took3,989ms once; partitioned
took2,319/337ms. Three page pairs matched. OS caches were uncontrolled; the next
gate is the complete frozen2003HTTP workload on the partitioned target at these
limits. Public configuration and v14 visibility remain unchanged.

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
