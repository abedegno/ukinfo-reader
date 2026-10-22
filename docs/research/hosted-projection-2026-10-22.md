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

The first flat-SQL projection run used 36 cases: 88 page comparisons, 4 baseline failures and 15 projection failures, with zero mismatches among successful comparisons. It exposed order-driven primary-key walks and predicates that did not reliably enable partial classification indexes. This preliminary run is retained; it is not merged into final-run statistics.

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

| Workers | Page comparisons | Identical successful pairs | Compact errors | Projection errors | Compact median / p95 | Projection median / p95 |
| --- | ---: | ---: | ---: | ---: | --- | --- |
| 1 | 196 | 190 | 4 | 4 | 329 / 2,724 ms | 49 / 1,236 ms |
| 4 | 184 | 170 | 13 | 3 | 360 / 4,036 ms | 38 / 965 ms |
| 8 | 172 | 153 | 19 | 6 | 312 / 4,194 ms | 54 / 3,165 ms |

These timings include candidate SQL plus record reads, including failures, and
exclude validation-only identity reads. **All successful pairs matched exactly.**
Page-comparison counts vary because a failed case stops its subsequent pages;
these aggregate medians are not a fixed-population cross-concurrency comparison.
The result artifact also reports the fixed 108 first-page attempts per stage.

Business classifications show the clearest benefit. At four workers, classification
plus address median fell from 1,474 to 166 ms, with errors falling from seven to
one across 68 paired page attempts. Postcode-only queries were already fast and
showed little consistent benefit. Address-only failures remain, and some mixed
classification/address attempts regress. This is promising evidence for targeted
hosted projections, not a release gate pass or a replacement serving format.

The 128-ID HTTP probe recheck had six failures on each API in 14 pairs. Seven
pairs succeeded identically, one succeeded only on each API and five failed on
both. Its median increased from 1,707 to 3,520 ms. **It remains disabled by default**;
the public API continues to use its existing address strategy.

## Visibility check and maintenance

A successful guarded ordinary VACUUM/ANALYZE was repeated with no import/backup
snapshot or prepared transaction present. Catalogue estimates remained 94.54%
all-visible for source entries and 92.56% for research keys, versus 100% for the
new projection. Those percentages alone did not establish a live-row deficit.

A read-only page-header sample returned aggregate flags only: of 256 sampled
source pages, 13 were uninitialized allocations and all 243 tuple-bearing pages
were marked visible. Of 256 research pages, 15 were uninitialized and all 241
tuple-bearing pages were visible. This supports unused allocation holes as an
explanation for the lower percentages; it is a sample, not a full visibility-map
audit. No record bytes were returned and no compaction/rewrite was performed.
The post-maintenance one-worker recheck is reported separately in the artifact.

## Decision

Keep the compact catalogue as the serving format. Keep both experimental access
paths opt-in/private. A production candidate should first remove the remaining
address/classification regressions, cover grouped/lexical/geographic behaviour,
and prove cold/scattered record reads and rotating-corpus capacity. A smaller
business-classification projection is worth testing before adding a 13.34-GiB
full-edition projection to every edition. No database-engine or backend-language
change is supported by these measurements.

## Reproduction

- [Build/finalization scripts and bounded operator benchmark](../../tools/hosted-projection/README.md)
- [CI work gates and private diagnostics](../search-performance.md)
- [HTTP paired runner](../../tools/search-load/README.md)

The result artifact contains only aggregate measurements. Query values, reference hashes, original records and credentials remain private.
