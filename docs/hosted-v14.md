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
