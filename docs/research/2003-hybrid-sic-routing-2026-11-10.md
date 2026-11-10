# Private hybrid SIC routing HTTP trial — 10 November 2026

**The unchanged 635-request 2003 cohort passed:** every frozen response hash
matched, with zero errors; C1 p95 was 136 ms and C4 p95 256 ms against the original
500/1,000 ms limits. Forty additional HTTP checks verified responses and cursor
compatibility in both flag directions. Production is unchanged; v14 stays hidden.
This is a private 2003 qualification, not a release or all-edition qualification.

## Change and fixed controls

The [matched SIC SQL comparison](2003-sic-layout-matched-2026-11-10.md) favoured
the unpartitioned index for the measured SIC shapes, while the earlier postcode
comparison favoured postcode partitioning. Source `66876ae74cb6d2b623d37733f7018dc405427f55`
on `spike/sic-layout-routing` therefore adds a default-off hosted experiment,
`--experimental-sic-unpartitioned`. Only eligible business SIC searches without
postcode, nearby or grouping constraints, with normal/name order, can select it.
The main path and other search predicates/order/seek remain unchanged.

An auxiliary readiness registry must match every selected edition's generation,
entry count, version and complete watermark, and every fixed auxiliary alias must
exist. Incomplete/missing readiness selects the entire original path before
execution; metadata errors or an active alternate query error return an error
without retrying on main. The request-local table override does not change source
generation or cursor fingerprints. Default-off and ineligible requests never
query the optional registry. The flag is limited to hosted ParadeDB serving.

NAS built the exact source into `ukinfo-sic-routing:2026-11-10.1` (amd64), image
`sha256:6a2fa352a421073cb53d1b886f09a7e804cbe12b568c96a94db0b1afb463ef03`.
The frozen tool-base identity and image layer ancestry were checked. Code-only
build took 314 s at at most 2 CPU / 2 GiB. No local image or database was copied.

Only a private owner helper installed the registry/view and SELECT grants to the
existing reader. The main alias still selects retained `layout.partitioned`;
the SIC alias selects retained `layout.baseline`. Both full copies have the
previously verified 59,886,841-row digest and four-segment proof. Partitioning here
means pg_search `partition_by=postcode`, not PostgreSQL table partitioning.
No new role, index build or data copy was required. Owner credentials were never
used by the real API or load client.

The private PG had 2 CPU / 6 GiB, MemorySwap 6 GiB; API 0.5 CPU / 512 MiB;
client 0.25 CPU / 128 MiB. SQL settings matched the previous test: shared_buffers
128 MB, work_mem 16 MB, generic plans, 4 s statement timeout, maxparallel 0,
I/O timing off. Quotas and process identity were checked throughout. No extra
CPU, relaxed threshold, altered frozen query or adaptive replay was used.

## Correctness and route proof

Local Go regression tests and vet passed. The two selected disposable database
integration suites passed on the exact candidate test binary in 80 s. Invented
fixtures cover missing/stale/partial readiness, missing aliases, multi-edition
all-or-none routing, native/main/alternate pagination and cursor exchange, and
metadata/active-query errors with no main retry. Synthetic aliases share fixture
tables to test semantics; real private aliases separately prove distinct layouts.

Real checks use the three previously selected SIC cases plus the original failed
postcode/name case. Flag-off verified four frozen first-page hashes and walked
three pages/case; flag-on verified the same four hashes and replayed all twelve
pages using flag-off tokens. Finally flag-off accepted eight flag-on continuation
tokens. All 40 requests returned 200 with matching full responses and next-page
availability. Each case had 150 distinct displayed records across its three pages
(600 displayed records in total, not asserted globally unique). SIC overlay counts
were exactly 0/12/0 for reference/candidate/reverse phases.

After these checks, only the private PG restarted and a fresh flag-on API served
the full cohort. This clears shared buffers, **not** OS/source/storage caches;
cursor preparation exercised those caches. This is not a cold-storage test.

## Frozen HTTP observations

The same 127 cases (39 business, no saved HTTP cursor cases) ran once initially,
then two shuffled rounds each at C1 and C4. All 635 responses matched their
original hashes; no error, unverified case or budget exhaustion occurred.

| Stage | Requests | Errors | Median | p95 | Maximum |
| --- | ---: | ---: | ---: | ---: | ---: |
| Initial | 127 | 0 | 12.528 ms | 404.640 ms | 1,355.889 ms |
| C1 | 254 | 0 | 6.091 ms | 135.803 ms | 310.496 ms |
| C4 | 254 | 0 | 9.897 ms | 256.086 ms | 613.620 ms |

Offline classification of the unchanged cases predicted 14 eligible cases × 5
requests = 70 SIC-route decisions. Actual counters matched exactly: 590 ParadeDB,
35 fallback, 5 radius-stream, plus 70 `paradedb_sic_unpartitioned` overlays. The
new counter is a subset, not an exclusive additional backend. Existing base counts
match the preceding cohort and rule out an accidental switch to native fallback.

Candidate phases totalled 27.206 s / 590 calls, record hydration 1.938 s / 645,
inventory 1.049 s / 590. Phase sums can overlap. Private PG whole-load deltas were
16.654 CPU seconds, 4.281 CFS-throttled seconds (33/214 periods), 403,333,120
container-accounted read bytes and 335,872 write bytes. Memory after load was
448,401,408 bytes, not a peak. These include background work; I/O is not unique
physical-device traffic and CFS time is not per-request waiting time.

## Decision, limitations and cleanup

This trial clears the original 2003 HTTP gate for this candidate/configuration.
It does not erase the preceding partitioned-only C4 failure at 1,024.685 ms or
other failed runs. Separate cache conditions and cursor preparation prevent a
causal speedup claim from comparing their p95 values. The matched SQL comparison
provides separate evidence for the layout choice. Another 228 frozen cases were
outside this edition scope; all-edition coverage, mixed-edition readiness and
v14 publication remain unqualified. Keep the flag off outside private testing.
Next qualify broader edition/all-scope behavior before considering promotion.
PR #48 stays draft; this separate source spike is not deployed or merged.

All thirteen owned helpers stopped, private PG stopped and restored to 1.5 CPU /
1,536 MiB, MemorySwap 3 GiB, and the maintenance lease released. Fresh checks
confirmed public health, unchanged serving image and live runtime identities/
start times/limits. No storage migration or Bitcoin change occurred.

Configuration and safe numeric evidence live in homelab
`docker/ukinfo/hybrid-sic-routing/`; NAS evidence root is
`/home/jonw/ukinfo-backups/hybrid-sic-routing-2026-11-10`.
Queries, rows, IDs, original ordinals, response hashes, raw plans, credentials
and cursor tokens stay on the NAS and are excluded from git.
