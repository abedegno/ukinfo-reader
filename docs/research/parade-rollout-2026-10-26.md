# Equal-budget control and hosted rollout — 26 October 2026

The equal-SQL-budget control supports the address-only hybrid policy. It does
not eliminate the native grouped/ordered tails or cold-storage latency.

[Aggregate results](parade-rollout-2026-10-26-results.json) contain counters,
latencies and structural plan diagnostics; query values, records and response
hashes stay on the NAS.

## Equal-budget control

Both API variants used the same executable and the existing isolated physical
source copy. A private Go build overlay changed only the source pool default from
2,000 to 4,000 milliseconds. The derived pool remained at 4,000 milliseconds.
Address, ordered and nearby source transactions **already used 4,000 milliseconds**
in the previous trials. The previous report's blanket description of source SQL
as two seconds has been corrected. HTTP deadlines were unchanged at two seconds
for simple searches and five seconds for address/ordered/nearby searches.

The frozen 294-case cohort ran twice, alternating backend order, with up to ten
pages. All 588 first-page attempts per backend completed the protocol, plus 757
later pages. No case was excluded. Later pages remain a selected population
because traversal stops when either side fails or runs out of pages.

| Population | Native p50 / p95 | Hybrid p50 / p95 | Native / hybrid timeouts |
|---|---:|---:|---:|
| First pages, 588 each | 26 / 1,780 ms | 19 / 859 ms | 15 / 5 |
| Later pages, 757 each | 82 / 619 ms | 87 / 647 ms | 2 / 0 |
| All attempts, 1,345 each | 60 / 871 ms | 48 / 741 ms | 17 / 5 |

There were **zero content mismatches in 1,327 mutually successful pairs**. Thirteen
pairs succeeded only on hybrid, one only on native, and four failed on both.
One-sided success cannot establish result equivalence for that pair.

For the 636 eligible address pairs, native timed out 11 times; hybrid had **zero
timeouts**, with 625 matching pairs and 11 hybrid-only successes. All five hybrid
timeouts used the native route. This supports keeping plain name/postcode,
grouped, nearby and edition-sorted queries on native rather than switching every
shape to ParadeDB.

This was a warmed follow-up, with no new cache eviction. It is not an equal-total-
resource engine comparison: hybrid adds the separately limited derived database.
No new concurrency/load or cold-cache experiment is implied by these numbers;
those results remain in the [previous report](parade-hybrid-trial-2026-10-26.md).
Production source-pool and HTTP deadlines are unchanged by the control overlay.

## Recovery scope

Backup/restore and extension-upgrade recovery exercises were explicitly omitted
at the operator's request. The derived index can be regenerated from original
media; its measured complete build took 5 hours 17 minutes. Original-source
backup policy remains unchanged. The exact extension image stays pinned; an
upgrade requires separate validation. Native indexes and the previous application
image remain available for routing/image rollback.

## Remaining slow-query investigation

A separate read-only tracer captured the three slowest statements for seven
prespecified native cases. Raw SQL, parameters and full plans remained in NAS
process memory. Only statement rank, case ordinal, status and allowlisted plan
nodes/counters were exported. Diagnostic analysis was bounded to ten seconds;
production SQL/HTTP deadlines were not raised.

- Alphabetical postcode cases 56 and 166 aggregate 15,554 and 13,527 name/first-name
  pairs respectively, then perform two label lookups per pair before ordering.
  The warmed analyses visited 104,577 and 91,611 shared buffers with no physical
  reads. Their first captured statements took 420 and 107 ms. This identifies
  substantial repeated work that cold random I/O can amplify; the follow-up
  itself did not independently reset caches.
- Broad grouped/edition-order cases 91 and 92 still timed out in the native
  four-second transaction. Even separate ten-second analyses did not complete.
  Estimated-only plans show materialized address expansion estimated at 458,185
  rows and tens of thousands of candidates before final aggregation/sorting.
  These are planner estimates, not measured row counts. A lower limit on returned
  rows does not bound this prerequisite work.
- Address-detail cases 63 and 95 succeeded. Case 95's captured address-selection
  statement took 809 ms, then 34 ms on analysis; another statement performed
  thousands of repeated membership lookups. These remain native endpoints.
- The previously slow address search (212) also timed out natively; its estimated
  address expansion was 117,455 rows. Derived custom/generic analyses both used
  `TopKScanExecState`, completing in 857/678 ms with 34,894 shared buffers, no temp
  spill and 51 heap fetches. Four other selected derived regressions also used
  Top-K. Custom analyses ran first and warmed caches; their timing difference
  does **not** establish a generic-plan improvement.

The next native optimization should bound candidate expansion before repeated
label/group lookups while preserving complete ordering and grouping semantics.
The derived default-order projection also merits testing without unnecessary text
columns; the observed 51 heap fetches make it a concrete experiment, not a proven
fix. Neither speculative rewrite is included in this rollout. Grouped and edition-
ordered searches remain native; all five hybrid timeouts in the control came
from that native population.

## Hosted activation and monitoring

The NAS now runs `ukinfo:hosted-2026-10-26.1` with address-only hybrid routing at
<https://ukinfo.6153112.xyz>. The verified derived volume was promoted in place
under the existing internal service name; its PostgreSQL 18.6 / pg_search 0.26.3
image digest remains pinned. It has no host database port and restarts persistently.
Old private controllers refuse to stop a database marked as serving.

The new API image was deployed in native mode first, then switched to hybrid.
Each stage matched **all twelve** prespecified, evenly spread saved references,
including their complete response digests. These checks used only the internal
Docker service with the public Host header, no proxy and no redirects. Values,
response bodies and hashes stayed on the NAS. The source database's image,
start time and restart count were unchanged across both stages.

Public health, UI and an invented-name search returned HTTP 200 with CSP and
`nosniff` headers. No saved searches were replayed through the Internet hostname.
Existing API quotas, trusted Cloudflare peer, origin and source reader were
preserved. Seven acceptance requests used ParadeDB according to local counters.
Native rollback remains a configuration switch and API recreation; the previous
application image is retained. No source migration or destructive cleanup occurred.

The Collector exports the target pool's five new aggregate metrics, plan selection
and phase histograms through the existing vendor-neutral OTLP pipeline. Three
managed Splunk charts were applied for plan selection, target utilisation and
waits, alongside existing phase-latency/error views. These show operating signals;
this short acceptance window is not a production concurrency or reliability test.

Final checks showed the API, source, derived database, tunnel and monitor healthy,
with zero container restarts; the Collector was running with zero restarts. The
local monitor reported health, metrics, disk, backup freshness and timeout rate
all OK. Splunk registered the new target-pool series and a `parade_candidates`
phase series. Existing alert delivery remains disabled; no new notification
recipients were configured.
