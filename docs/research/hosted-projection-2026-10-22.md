# Bounded address intersections and a full-edition hosted projection

22 October 2026. This follows the [full-catalogue query review](query-workload-2026-10-21.md). It compares access paths on the existing NAS; it does not change the supported disc formats or replace PostgreSQL/Go.

## Experiment and build cost

The private projection contains **all 59,886,841 records in the 2003 SE edition**, including businesses. It combines source search keys, address-location IDs, grouping keys and source classifications. Original compressed record blocks remain the record store.

The initial build took **30 minutes 29 seconds**, including a lock-timeout recovery. Row loading took 9 minutes 35 seconds. Its table was 5,576,499,200 bytes (5.19 GiB), with 8,708,440,064 bytes (8.11 GiB) of indexes. The cluster generated approximately 22.3 GiB of WAL during that interval; cumulative WAL generation is not retained WAL or peak disk usage. Classification indexes were subsequently widened for the final comparison; their additional cost is reported with the results.

Loading commits approximately one million rows at a time. Counts must match the complete edition, IDs must be unique, reverse locations must be present and grouping keys must be complete. The first attempt to build the primary key hit the five-second lock timeout while automatic vacuum held the private experiment table. No rows were discarded. Finalization resumed after that vacuum; the reusable procedure now disables automatic vacuum on the experiment table during construction, explicitly vacuums/analyzes it and restores normal settings.

This is an **additional single-edition search projection**, not a complete replacement format. It does not contain record payloads, a cross-edition grouping index, source-fingerprint lookup or every API ordering/geographic feature. Multiplying its measured size by the whole archive is only a rough storage estimate.

Public serving permissions and schema remain unchanged. The serving role was verified to have neither schema usage nor table SELECT on the experiment. No database/tunnel restart, public route, cache flush, source rewrite or timeout increase was used.

## Address optimization: rejected variants and bounded fallback

A full-set SQL intersection first tried reading each required reverse block once. It avoided expanding town postings, but still read the complete source set before returning a page. A subsequent 512-ID batch walker could stop early for dense matches, but sparse county searches repeatedly read too many batches.

The paired 512-batch run had 35 baseline failures and 31 candidate failures in 166 pairs, with 127 identical successful pairs and no successful-content mismatch. One county case succeeded on the baseline and timed out twice on the candidate. That variant was rejected for deployment.

The revised, opt-in implementation tries **one 128-ID prefix**, sufficient for the maximum 100-item page plus lookahead when matches are dense. It uses that prefix only if it proves a complete page or source exhaustion. Otherwise the complete posting intersection remains in use. This bounds speculative reverse reads without truncating matches or walking many sparse batches. Grouped and lexical-order access paths retain their existing strategy.

CI checks exact full pagination, source order, sparse/distant town ranges, mixed industry/SIC classifications, grouped and sorted business results, forced custom/generic source plans and bounded buffer/heap work. A cheaper exact bitmap is allowed within a small heap-page budget; a broad primary-key/sequential walk is rejected.

## Projection query design and workload

The first flat-SQL projection run used 36 cases: 88 page comparisons, 4 baseline failures and 15 projection failures, with zero mismatches among successful comparisons. Its query design allowed order-driven primary-key walks and did not explicitly establish the partial-index predicates. The tuning changes below were assessed together, not as separate causal experiments. This preliminary run is retained; it is not merged into final-run statistics.

The corrected query establishes eligible IDs before ordering, includes explicit nonempty predicates for partial classification indexes and covers classification searches with the remaining source/address keys. PostgreSQL documents that partial-index predicates must be provable at planning time; a parameterized range alone may not establish that implication. [PostgreSQL 18 partial-index documentation](https://www.postgresql.org/docs/18/indexes-partial.html).

Neither side requests grouping keys before LIMIT: bounded grouping-key validation runs after the timed search/record path, with its own recorded latency.

The final fixed workload contains 54 private cases. Eighteen added cases exercise guaranteed-positive SIC, postcode and address combinations from the 2003 edition. That edition has stored SIC codes but no resolved industry descriptions; industry-text cases therefore exercise genuine empty results. Descriptions are not guessed from another disc. See [the format documentation](../formats/2003-se.md).

Each run alternates baseline/projection execution order, uses two rounds and up to three consecutive 50-candidate pages, validates the lookahead IDs and grouping keys, and compares decoded original records. SQL has a four-second budget; search plus record retrieval has a five-second budget. Validation-only identity reads are measured separately. Errors and incomplete runs fail the operator command and remain in the report.

The baseline uses the compact source tables, research keys for classification filters and reverse arrays for address filters. It is a decomposed SQL comparator, **not a replay of every deployed API strategy**. The candidate uses the combined table. Both use the same dictionaries and production record decoder. These are candidate-query/record-read measurements, not HTTP capacity or proof of grouped/nearby/lexical-order equivalence. Source-ID-ordered pages can have substantially better compressed-block locality than lexical UI results; the low warm record-read medians here do not resolve the earlier cold/scattered record-I/O costs.

Concurrency runs use the same fixed corpus at 1, 4 and 8 workers sharing an eight-connection pool. Baseline and projection queries are interleaved within each run; their combined throughput is not independent API capacity. PostgreSQL retains its two-CPU/six-GiB limit and one-GiB shared buffers. The client has one CPU and 512 MiB. No import, backup or projection build overlaps the final workload runs; OS caches, checkpoints and other NAS activity remain uncontrolled. Later runs are increasingly warm.

## Measured results

The final classification-covering indexes add 43,876,352 bytes (41.8 MiB), taking
49.6 seconds to rebuild on the private table. Final table plus indexes is
**14,328,815,616 bytes (13.34 GiB)**. Tuning WAL was not independently measured.

The primary comparison uses the same **108 first-page attempts** at every stage.
Numbers are nearest-rank p50/p95 and include failed attempts.

| Workers | Identical successful pairs | Compact errors | Projection errors | Compact p50 / p95 | Projection p50 / p95 |
| --- | ---: | ---: | ---: | --- | --- |
| 1 | 102 | 4 | 4 | 184 / 1,738 ms | 19 / 2,450 ms |
| 4 | 99 | 8 | 3 | 105 / 4,149 ms | 16 / 3,418 ms |
| 8 | 93 | 15 | 6 | 257 / 4,289 ms | 23 / 4,015 ms |
| 1, post-maintenance repeat | 102 | 4 | 4 | 245 / 1,871 ms | 59 / 2,166 ms |

These timings include candidate SQL plus record reads and exclude validation-only
identity reads. **All successful pairs matched exactly.** The one-worker p95 was
worse on the projection, both before and after maintenance, despite its better
p50. At four/eight workers errors improved, but p95 remained several seconds.

Including subsequent pages produced 196, 184 and 172 comparisons at 1/4/8 workers,
plus 193 in the separate post-maintenance recheck. Counts differ because a failed
case stops its subsequent pages. Those results are retained in the
[aggregate artifact](hosted-projection-2026-10-22-results.json); they are not used
as the fixed-population headline comparison.

Business classifications show the clearest benefit. At four workers, classification
plus address p50 fell from 1,408 to 125 ms, with errors falling from seven to
one across 68 paired page attempts. Postcode-only queries were already fast and
showed little consistent benefit. Address-only failures remain, and some mixed
classification/address attempts regress. This is promising evidence for targeted
hosted projections, not a release gate pass or a replacement serving format.

The 128-ID HTTP probe recheck had six failures on each API in 14 pairs. Seven
pairs succeeded identically, one succeeded only on each API and five failed on
both. Its median increased from 1,707 to 3,520 ms. **It remains disabled by default**;
the public API continues to use its existing address strategy.

Twenty independent address reference cases were also checked on both APIs. The
baseline matched 19 reference responses with one HTTP failure; the experiment
matched 18 with two failures. Seventeen pairs succeeded on both with no content
mismatch. This also supports keeping the experiment off.

## Visibility check and maintenance

A successful guarded ordinary VACUUM/ANALYZE was repeated with no import/backup
snapshot or prepared transaction present. Catalogue estimates remained 94.54%
all-visible for source entries and 92.56% for research keys, versus 100% for the
new projection. Those percentages alone did not establish a live-row deficit.

A read-only page-header sample returned aggregate flags only: of 256 sampled
source pages, 13 were uninitialized allocations and all 243 tuple-bearing pages
were marked visible. Of 256 research pages, 15 were uninitialized and all 241
tuple-bearing pages carried the on-disk all-visible flag. This supports unused allocation holes as an
explanation for the lower percentages; it is a sample, not a full visibility-map
audit; disk headers can lag buffers. The flag/layout interpretation follows
[PostgreSQL 18 page-header definitions](https://github.com/postgres/postgres/blob/REL_18_STABLE/src/include/storage/bufpage.h). No record bytes were returned and no compaction/rewrite was performed.
The post-maintenance one-worker recheck remained mixed; it did not establish a maintenance-only fix.

## Decision

Keep the compact catalogue as the serving format. Keep both experimental access
paths opt-in/private. A production candidate should first remove the remaining
address/classification regressions, cover grouped/lexical/geographic behaviour,
and prove cold/scattered record reads and rotating-corpus capacity. Smaller place-level ID postings for town/county intersections and a business-only
classification projection are worth testing before adding a 13.34-GiB full-edition
projection to every edition. These are proposed follow-up experiments, not measured
production improvements. No database-engine or backend-language change is supported by these measurements.

## Reproduction

- [Build/finalization scripts and bounded operator benchmark](../../tools/hosted-projection/README.md)
- [CI work gates and private diagnostics](../search-performance.md)
- [HTTP paired runner](../../tools/search-load/README.md)

After saving evidence, the private experiment tables were removed with guarded
`RESTRICT` cleanup and all 12 temporary containers were removed. This removed
13.34 GiB of temporary relations and avoids adding them to full database backups.
All five published editions and 248,951,107 visible records were verified intact;
the public image remains `hosted-2026-10-21.4`.

The result artifact contains only aggregate measurements. Query values, reference hashes, original records and credentials remain private.
