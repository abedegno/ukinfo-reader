# Source-cold and concurrent search follow-up

The hosted hybrid selector now includes ungrouped alphabetical postcode and
business-classification searches, plus default-order nearby searches combined
with address filters. The complete native business projection also supports
alphabetical ordering. Source records, cursor formats, quotas and deadlines are
unchanged. No database migration or derived rebuild is needed.

## Diagnosis and rejected intermediate paths

Cold alphabetical postcode searches spent their budget enumerating and grouping
the source postcode population. The two main cases read about 18,000 and 16,000
occurrences before discovering name order. Batching dictionary reads alone did
not remove that source work. Nearby/address searches also expanded and sorted
large native candidate sets. The existing derived index already contained the
fields required to avoid those steps.

The first narrow postcode routing experiment was mixed: two of three source-cold
requests timed out. Later profiling separated source and target work; derived
cache state was uncontrolled. These failed attempts remain in the private evidence.
The final claim below is **source-cold**, with shared target caches, rather than a
fully cold-system guarantee.

Concurrency diagnosis corrected an ordinal ambiguity. The matched load subset
has 292 entries; its case 230 maps to case 232 in the complete 294-case workload.
It is an alphabetical business-classification search, covering about 453,000
businesses. The older source-cold case 230 was a different name/address query.
The operator metadata now exports the parent ordinal and fixed search-shape flags.

The repeated failures were execution work, rather than connection starvation.
One baseline C4 run accumulated only 13 ms of source pool waits and 535 ms of
derived pool waits across 876 attempts, while two ordered statements each reached
their four-second backstop. Baseline plans scanned about 2.47 million business
occurrences and repeatedly resolved names for the 453,000 matching businesses.

Enabling the native business projection reduced the classification scan, but a
prototype still encountered about 443,000 distinct name pairs and fell back to
expensive lexical discovery. Its C4 runs still had two timeouts each, and one
deeper page timed out during the full comparison. It was not sufficient as the
hosted concurrency fix. The final native fallback uses a complete projection sort
after pair overflow, avoiding a class rescan for every returned name. The hosted
alphabetical classification path uses the derived lexical index: a bounded
diagnostic completed in 323 ms with 51 heap fetches. A diagnostic ran during the
intermediate correctness comparison; final load measurements were isolated.

## Correctness and unchanged limits

The frozen 294-case workload ran twice, up to ten pages, against the previous
hosted build and final candidate. All **1,470 request pairs matched**, with no
errors or content mismatches. Both executables read the same private source clone
and the same SELECT-only derived serving database. Resource and connection limits
were unchanged: 8 source connections, 4 target connections, normal two/four-second
SQL budgets and two/five-second HTTP budgets according to query type.

An independently built **full native oracle**, frozen before these optimizations,
provided two pages for each of the four cold cases and the business-classification
case. Its offline 120-second diagnostic budgets are not serving changes or
performance measurements. Twenty page/cursor checks matched those references;
four additional checks passed in native rollback mode. Native rollback's first
business request took 4.12 seconds, so it retains substantially less headroom than
the selected hosted path.

## Source-cold results

Each probe independently stopped the private source and its APIs, evicted that
clone's file pages and required no more than 1 MiB resident before restarting it.
There was no global cache flush or production-source eviction. The target remained
live, shared and uncontrolled; warm comparisons precede the final cold probes.
Drive/controller caches and filesystem metadata are outside the measured boundary.

| Full-workload case | Previous source-cold | Final source-cold |
| --- | --- | --- |
| 56, alphabetical postcode | Timeout | 589 ms |
| 166, alphabetical postcode | Timeout | 554 ms |
| 9, alphabetical postcode | Timeout | 534 ms |
| 95, nearby/address | Timeout | 1,081 ms |
| 232, alphabetical classification | Not measured in the old cold cohort | 653 ms |

The four original cold cases also passed two independently reset candidate rounds
before the business-sort refinement, at 0.55–1.36 seconds. All final cold responses
matched the frozen native first-page hashes. These observations do not establish
every possible radius, postcode, page or fully cold target's latency.

## Fixed concurrency workload

The same 292 previously matched cases ran twice per phase. Each run has 292 initial
checks and 584 measured requests at its selected concurrency. C4 repeats used
balanced backend order; errors and initial checks remain in the aggregate files.
The cohort is never recalibrated to discard a newly failing case.

| Build / phase | Requests | Errors | p95 | Throughput |
| --- | --- | --- | --- | --- |
| Previous C1 | 584 | 0 | 97 ms | 29.1 req/s |
| Final C1 | 584 | 0 | 109 ms | 36.5 req/s |
| Previous C4 A | 584 | 2 | 216 ms | 55.7 req/s |
| Final C4 A | 584 | 0 | 284 ms | 64.8 req/s |
| Final C4 B | 584 | 0 | 253 ms | 68.3 req/s |
| Previous C4 B | 584 | 2 | 226 ms | 57.2 req/s |

The final candidate completed **1,168 C4 requests without errors**, while both
baseline runs reproduced the business-sort timeout twice. Throughput improved,
but p95 latency increased for the mixed workload. This is a reliability improvement
with a measured latency tradeoff, not an across-the-board speedup or an Internet
capacity estimate. No pools, timeouts or admission limits were increased.

## Reproduction and operating scope

The [aggregate artifact](cold-search-2026-10-26-results.json) is generated by
[the summary tool](../../tools/cold-search-preview/summarize.py), which validates
case frequencies, source references, all cold-residency gates, repeated C4 runs and
unchanged production lifecycle. Queries, hashes, cursor tokens and records stay on
the NAS. The homelab controllers preserve prototypes and final evidence separately.

Pinned-extension integration checks complete nearby/postcode paging, cross-backend
cursors, source geography metadata, district/sector centres, retired postcodes,
empty radii and Top-K plans. Native integration compares business projection pages
with the legacy path, checks one batched vocabulary read for small classifications,
and verifies the overflow signal preserves a complete sort. CI exports deterministic
work reports; its invented fixtures do not stand in for these real NAS measurements.

Default-order postcodes/classifications remain native, as do pure nearby searches,
nearest ordering and grouped nearby searches. Missing/stale derived generations
retain whole-query native fallback before execution. Selected target failures
still fail closed instead of retrying a second expensive backend.
