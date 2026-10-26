# Grouped and ordered search follow-up, 26 October 2026

The optional hosted hybrid path now accelerates address-filter searches with
grouped default/alphabetical ordering and ungrouped edition ordering. The native
alphabetical postcode path batches vocabulary reads. Source identities, canonical
group keys, representatives, counts, refinements and cursor formats are retained.
No source schema migration, extension upgrade or derived rebuild is required.

## Changes and rejected approaches

Grouped results stream complete name/first-name pairs and keep only the page plus
lookahead's smallest canonical group keys. They cannot emit an incomplete dense
pair: after lexical discovery, numeric batches of 2,048 drain its remaining
occurrences once per edition. The canonical source finalizer still loads group
representatives and counts. Sparse refinements continue until a page is known.

Edition sorting visits newer dates first, queries every tied date, then stops
before older dates only when they cannot enter the result page. Address discovery
uses at most two concurrent edition scans and the existing four-connection target
pool. Failures cancel sibling work and return no partial results.

For native alphabetical postcode queries, a probe collects up to 65,537 numeric
name pairs. At most 65,536 pairs use a single distinct, storage-ordered vocabulary
read followed by the original lexical selection. An overflowing probe falls back
to the original complete SQL. This is an access-path threshold, not a result cap.

Simply enlarging native probes did not solve the timeouts. A native SQL rewrite
using shared label CTEs and per-edition address probes was worse: one diagnostic
request took 36.6 seconds with substantial temporary I/O; the two critical cases
still failed a 60-second diagnostic bound. The rejected code-only patch is kept
under [experiments](../../tools/ordered-preview/experiments/native-v2.patch).
Repeated small lexical scans also failed on dense pairs, motivating numeric
draining. None of those failed approaches is in the serving path.

## Private protocol and correctness

The fixed 294-case real workload ran twice, up to ten pages, against the previous
and proposed hybrid executables. Both read the same source clone and SELECT-only
derived serving database with unchanged serving deadlines and resource limits.
Every attempt, including errors, remains in the aggregate comparison. There were
1,416 request pairs, 1,410 successful equal responses, five new-only successes,
one old-only success and **zero content mismatches**. First-page timeouts fell
from five of 588 to one of 588; this is an improvement, not a zero-timeout result.

The two formerly failing cases are grouped and edition-sorted address searches:

| Fixed case | Previous build, both rounds | Proposed build, round 1 / 2 |
| --- | --- | --- |
| 91, grouped | Timeout at about 4.06 s | 267 / 120 ms |
| 92, edition order | Timeout at about 4.02 s | 33 / 37 ms |

The one new-only timeout was nearby case 95, which retains the native path; its
other round succeeded. Alphabetical postcode cases 56 and 166 improved median
page latency from 117 to 82 ms and 104 to 74 ms respectively, but some tails
regressed. Shared caches and changing storage latency prevent attributing every
individual timing difference to code.

Native full sorting could not complete the critical cases even with a private
120-second oracle deadline. Instead an offline source oracle independently
enumerated the **complete source ordering prefix** through a third-page boundary,
then applied original native filters/grouping/sorting. Candidate IDs never form
an allow-list. Prefix safety bounds abort rather than truncate. Four source
reference pages matched eight checks covering candidate and native cursors.
Separately, 283 initial matches and 62 cross-backend continuations had zero
mismatches; two changed-query cursor checks were rejected. These are bounded
correctness checks, not proof over every possible query or later page.

## Concurrency

The 292 cases successful and equal during calibration formed the fixed load
cohort. Cases 91/92 were retained in the full comparison and source oracle, and
excluded only from this matched load. Each build ran twice in balanced order;
each run included 292 initial requests and 584 requests at each concurrency.

| Build / run | C1 p95 | C1 errors | C4 p95 | C4 errors |
| --- | --- | --- | --- | --- |
| Previous A | 114 ms | 0 / 584 | 255 ms | 2 / 584 |
| Proposed A | 106 ms | 0 / 584 | 264 ms | 2 / 584 |
| Proposed B | 138 ms | 0 / 584 | 189 ms | 2 / 584 |
| Previous B | 114 ms | 0 / 584 | 231 ms | 2 / 584 |

There is no demonstrated concurrency-capacity increase. Case 230 accounts for
seven of the eight C4 errors; proposed run A also failed case 9. No responses
mismatched. The full workload and errors must accompany any targeted speed claim.

## Source-cold protocol

Before each old/new request, both private APIs and the source clone were cleanly
stopped. Linux per-file eviction then required no more than 1 MiB resident source
pages before reopening the clone. No global cache flush or production eviction
was used. The shared derived target stayed live: its cache is uncontrolled.

For the critical cases, previous requests timed out near five seconds; proposed
case 91 completed in 2,462 ms and case 92 in 598 ms. Checks also include postcode
cases 56/166 and remaining-tail cases 95/9/230:

| Fixed case | Previous source-cold | Proposed source-cold |
| --- | --- | --- |
| 91, grouped | Timeout | 2,462 ms |
| 92, edition order | Timeout | 598 ms |
| 56, alphabetical postcode | Timeout | Timeout |
| 166, alphabetical postcode | Timeout | Timeout |
| 95, nearby | Timeout | Timeout |
| 9 | 3,770 ms | 4,397 ms |
| 230 | 439 ms | 458 ms |

Cold alphabetical postcode and nearby search remain unresolved. Case 9 also
remains close to its deadline. The [aggregate results](ordered-search-2026-10-26-results.json)
and completion gates are generated by the accompanying
[summary tool](../../tools/ordered-preview/summarize.py). All 14 source-residency
checks passed, with the public source/API and target lifecycle unchanged.

## Repeatability and CI

The pinned pg_search 0.26.3 integration fixture includes 320 distinct groups under
one name pair, 260 sparse-refinement rows, tied dates and hidden-sample rules.
It checks complete paging, cross-backend cursors, original counts/representatives,
missing-group fail-closed behaviour and source-oracle equivalence. The native
fixture checks 360 postcode pairs and a single batched vocabulary read.

CI publishes deterministic work reports alongside correctness checks. The dense
fixture requires one lexical discovery and one numeric pair drain; Top-K numeric
plans must keep heap fetches bounded by the requested batch. These are not
index-only scans: the measured 100-row drain made 100 heap fetches. Assertions
avoid unreliable shared-runner latency thresholds.

Real queries, hashes, cursors and responses stay on the NAS. Only aggregate
reports are exported. The homelab `ordered-trial` controllers preserve the frozen
cohort, previous binary, private references and cache evidence. The public API,
original source database and target lifecycle were unchanged during the trial.
Nearby and grouped edition sorting still use native search. Desktop keeps its
existing backend; native mode remains the hosted rollback.
