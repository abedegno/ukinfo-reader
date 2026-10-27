# Tail latency and broader search coverage

The hosted candidate reduces repeated derived-query planning, keeps narrow sorted
surname searches native, and selects the existing derived index for broad sorted
surnames. A separate coverage suite exercises wildcard names, street/county-only
queries, 50 km nearby searches, partial nearby centres and cross-edition grouping.
The original 294-case comparison and 292-case load cohort remain frozen.

## Changes and operating boundary

The target pool uses `force_generic_plan`. Its parameterized candidate queries
explicitly select the pinned pg_search 0.26.3 index and retain Top-K ordering. The
source pool keeps its existing planning policy. The [PostgreSQL plan-cache
reference](https://www.postgresql.org/docs/18/runtime-config-query.html#GUC-PLAN-CACHE-MODE)
explains the custom/generic distinction; this setting is justified here by the
paired workload and prepared-plan tests, rather than a general recommendation
for other PostgreSQL queries.

A source probe reads at most 4,097 compact occurrence-index rows for an exact
surname with alphabetical ordering, no first name, grouping or refinements. It
selects the derived backend only above 4,096 occurrences, before checking target
readiness. Small/empty searches keep native execution and do not query the target.
The probe does not cap results. Original records and source publication metadata
remain authoritative. Operator metadata labels this conditional path `selective`.
Native exact-name ordering also batches numeric name/first pairs and dictionary
labels, retaining the complete old ordering when its 65,536-pair probe overflows.
Native-only desktop/rollback execution therefore keeps the same result contract.

For nearby/address queries, the adapter resolves only the centre and checks each
complete batch of up to 100 address candidates against the source geography.
Postcode locations are looked up by those dictionary IDs, before loading records.
It continues through rejected batches until page/lookahead or exhaustion, without
an iteration or result cutoff. It avoids the original full-radius join and the
large derived membership predicate. Missing locations remain excluded. Source
release, exact spherical distance, quality, termination status and district/sector
centre semantics are unchanged. Centre-only cache entries cannot satisfy a native
full-radius lookup. Native nearby/nearest/grouped execution is unchanged.

Original record retrieval reads at most four compressed blocks per statement,
with at most two reads in flight within the unchanged eight-connection source pool.
A deep page may require nearly fifty distinct blocks. The batches retain the same
HTTP and SQL deadlines, source-record order and complete response; failed statements
are never retried and partial records are never returned. An invented multi-batch
fixture verifies reverse block order, duplicate IDs, source identity and large
integer preservation. A mid-batch cancellation returns no partial records and
releases both reader connections. Compressed rows are fully received and the SQL
statement closed before decoding, keeping Go CPU time outside its lifetime.

Native wildcard-surname/given-name searches across editions also overlap at most
two independent edition probes when their vocabulary expansions fit the existing
bound arrays. Each contributes a complete page/lookahead, then an exact numeric
merge preserves name/first-name/record/dataset order. Large SQL-side expansions,
other orderings, postcode/address/nearby searches and grouped searches retain the
original combined query. Any failed probe cancels its sibling and rejects all
partial candidates. This path is labeled `source_prefix_parallel`; backend and
source planning policy remain native/auto. Invented tests compare every cursor
page with an independent unbounded SQL population and check sibling cancellation.

Derived grouping retains source visibility/readiness checks while skipping the
native address access-plan probe that will not execute. Canonical representative
selection keeps its one-probe early exit when the first edition has every group;
otherwise later editions are checked in pairs and merged by min(dataset ID, source
ordinal), not completion order. Occurrence totals use at most two scalar-edition
aggregates at a time; edition labels retain database collation order. Counts and
representatives remain authoritative source values. An independent unbounded SQL
aggregate verifies the totals and edition list, and a late-edition tie fixture
checks canonical source identity.

The broad grouped-address case exposed an additional cold aggregate stall after
the earlier I/O fixes. Three source-cold responses after splitting counts took
2.45, 2.10 and 2.01 seconds and matched the frozen pre-change hybrid first-page
response. A separate full-native oracle attempt exceeded its ten-minute offline
limit without completing a page; its failed partial file is excluded. This grouped
check is explicitly a **pre-change hybrid comparison**, additional to the 64
full-native page/cursor checks, not a full-native oracle or a baseline capacity
measurement. The normal serving deadlines are unchanged.

The fixed `paradedb_radius_stream` plan label makes this path visible in existing
OpenTelemetry/Splunk search-plan charts. Pools, CPU/memory limits, quotas, HTTP/SQL
deadlines, cursor formats, source records and derived schema are unchanged.

## Investigation and retained failures

Two matched-load C4 repeats showed the target-only generic-plan experiment reduce
p95 from 298 to 198 ms and from 231 to 202 ms, with no candidate errors. Full
hash checks use the frozen response references; cases are not recalibrated away.
The combined pre-selector prototype also passed all 1,470 original request pairs,
but its new source-cold exact-surname case took 4.49 seconds. The bounded selector
reduced that case to 1.38 seconds and matched the pre-change native reference.

Equal-rate checks use 20 scheduled requests/second and 584 arrivals, with both
backend orders and generator drops/errors retained. An early candidate run had a
768 ms p95 spike, while its repeat was 228 ms; baseline runs were 384/317 ms.
Those observations do not establish an across-the-board equal-rate improvement.
The final candidate must be assessed using its own complete measurements.

The new suite exposed two maximum-radius timeouts. SQL-array membership involved
about 59,000 postcode IDs. An explicit extension term-set experiment also timed
out; it was rejected. An intermediate full-map candidate stream passed all twelve scenarios
and forty independent native page/cursor checks. Its first full-radius and sector
observations were 1.88 seconds and 151 ms; caches were shared/uncontrolled.
These were smoke observations, not cold-storage or capacity guarantees. The final
combined run then exposed an intermittent source-record timeout on business page ten
and 3.3–4.3 second cold-radius responses. A further isolated diagnostic reproduced
a four-second geography statement timeout: it joined about 59,000 postcode IDs
before finding any directory candidates. The final candidate checks only candidate
locations instead. The earlier failures remain part of the evidence.

The first candidate-only lookup still scanned the 2.65-million-row reference for
just fifteen postcode IDs. Dictionary keys use C collation; the existing ONSPD
postcode primary key uses database-default collation. Matching the index's
collation restores fifteen exact index probes: the source plan dropped from about
41,500 shared buffers to 103, without a migration or rebuild. PostgreSQL documents
this [index/collation requirement](https://www.postgresql.org/docs/18/indexes-collations.html).
The invented work-budget test checks two expected nearby points and no more than
64 shared buffers for a three-code batch amid 10,000 unrelated locations.
Focused source-cold requests then took 1.62/1.32 seconds; those diagnostic runs
are separate from the final fixed-cohort acceptance.

A later sixteen-block-read prototype retained one postcode-only page-ten timeout.
The replay's candidate query used its covering indexes; original compressed blocks
dominated the cold response. The final four-block/two-reader implementation
addresses that measured I/O path. Source planning remains auto; diagnostic custom
and generic source-plan runs did not justify changing it globally.

## Separate coverage suite

`ukinfo-parade-trial --stage extend-coverage` derives twelve deterministic queries
from the frozen inputs, without executing them to choose successful cases. It
writes queries only to a private NAS file, plus a value-free manifest. Missing
scenarios fail preparation; a newly failing query is retained. The original load
cohort and its case ordinals/frequencies are never changed by this extension.

The independent oracle is built from `hosted-2026-10-27.1`, before these changes.
It executes the full native path for up to two pages per scenario. One broad
sector reference exceeded the original offline two-minute diagnostic deadline;
a separately recorded ten-minute offline build completed the full reference.
These oracle budgets are not serving changes or performance results. The three
complete reference files contain twenty pages, checked with both candidate and
native cursors (forty checks). The failed partial reference is not published.

Pinned-extension CI uses invented data only. It checks 46-page lexical/grouped
traversals, both cross-backend cursor directions, wildcard combinations and a
known postcode 33 km away. Ten thousand unused but located postcode dictionary
entries exercise the large-set path. A wholly outside 320-record population must
stream through multiple rejected batches and still return an empty result.
Independent expected counts guard against both backends sharing truncation.

Real queries, response hashes, cursors, coordinates and records remain on the NAS.
The private source clone is reused without a new database copy. Source file pages
are evicted only while its processes are stopped; there is no production-source,
derived-target or global cache flush. Target caches are shared/uncontrolled.

## Final private acceptance

The [value-free aggregate](tail-search-2026-10-27.json) records the complete final
protocol. All original 294 cases appeared in both rounds, through up to ten pages;
there were **zero candidate errors and zero content mismatches**. Baseline failures
remain in the outcome counts. The separate twelve-case comparison and all 64
full-native page/cursor checks passed. Later paired pages remain selected by both
servers succeeding; no failed case was recalibrated out of the fixed load cohort.
The grouped first-page check is separately labeled as pre-change hybrid.

| Fixed 292-case load | Previous build | Candidate |
| --- | ---: | ---: |
| C1 p95 | 109 ms | 100 ms |
| C1 throughput | 36.6 requests/s | 42.1 requests/s |
| C4 p95, first pair | 278 ms | 214 ms |
| C4 p95, reverse-order repeat | 230 ms | 208 ms |
| C4 throughput, first pair | 62.1 requests/s | 79.7 requests/s |
| C4 throughput, repeat | 69.3 requests/s | 80.6 requests/s |

Every measured case occurred twice per closed-loop phase, plus the unchanged
initial 292 checks. All candidate initial/measured requests succeeded. The separate
expanded suite does not replace or reweight this load population.

At 20 scheduled arrivals/second, both runs delivered all 584 requests with no
candidate errors or generator drops. Candidate p95 was 232/195 ms; baseline was
328/587 ms. Earlier candidate spikes remain documented above. There are only two
repeats, the target cache/maintenance is uncontrolled, and these results do not
establish a universal equal-rate improvement or a maximum hosting capacity.

All ten independent source-only cold probes passed the three-second gate and
matched their frozen references. The original six were 0.57–1.08 seconds; the new
wildcard/given-name query was 1.69 seconds; the 50 km unit/sector cases were
1.05/1.15 seconds; broad grouped-address search was 2.39 seconds. The latter uses
the explicitly labeled pre-change hybrid snapshot. API/SQL deadlines, public source and
derived-target service identities, source records and derived schema stayed unchanged.

Native/race integration, pinned-extension compatibility and execution-work tests,
Go race/vet, and isolated Collector privacy/export checks passed. The earlier
Collector startup failure was retried separately and passed; no production
Collector credentials or configuration were used by that test. Hosted acceptance
and the contained scheduled-runner rehearsal are recorded in homelab after the
reviewed image rollout; their resource profile differs from this private trial.
