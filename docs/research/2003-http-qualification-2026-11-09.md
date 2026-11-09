# Private 2003 HTTP qualification — 9 November 2026

**Completed: plan and response correctness passed; C4 performance gate failed.
Nothing deployed.** The candidate remains private.

## Fresh plan verification

The old detector required exact `TopK`; the installed extension reports
`Custom Scan` / `TopKScanExecState`. A shared detector now checks both exact fields
and recursively examines nested plans. Positive/negative fixtures reproduced the
failure and passed after correction, including rejection of misleading methods.
Old timed observations and their false flags remain unchanged.

Fresh EXPLAIN-only checks used the real SELECT-only API role against the physical
candidate and its private API view: 50 cases twice, 100 generic plans, all 98
non-control instances using Top-K. Raw plans remain on NAS. This proves these
fresh plans, not the historical timed requests' unretained executor details.

## Existing HTTP regression corpus

All **127 explicitly 2003-scoped cases** from the frozen 355-case corpus ran with
unchanged query strings and response hashes. The other 228 belong to other scopes;
they were not removed because of results. The subset includes 39 business cases,
search/address/order/nearby/export/record coverage, and **no frozen cursor cases**.
Earlier SQL page comparisons do not substitute for a fresh HTTP cursor traversal.
This does not qualify cross-edition merging, deduplication or v14 publication.

The private API used the exact deployed executable, verified by binary hashes.
Private metadata/views expose the already-built candidate without re-importing
rows or rebuilding its index. The dedicated reader has no write, schema-create,
temporary-table, superuser or bypass-RLS powers. Source reads use the existing
sealed reader. Owner credentials were available only to the setup job.

A frozen address-search probe matched its expected response and incremented the
ParadeDB metric before load. The cohort recorded 590 ParadeDB plan decisions,
35 native fallbacks and 5 radius-stream decisions. These are overlapping plan
counters, not a per-request backend classification. Native-only paths remain
native by application design; results cannot be attributed solely to the index.

| HTTP stage | Requests | Errors/hash mismatches | Median | p95 | Maximum |
| --- | ---: | ---: | ---: | ---: | ---: |
| Initial sequential | 127 | 0 | 140 ms | 1,069 ms | 2,483 ms |
| C1, two fixed rounds | 254 | 0 | 5.8 ms | 370 ms | 713 ms |
| C4, two fixed rounds | 254 | 0 | 13.6 ms | 1,306 ms | 2,591 ms |

All 635 cohort requests matched their frozen responses. No timeout, budget
exhaustion, unverified hash, case exclusion or timed retry occurred. A separate
one-request index-use probe also passed. C1 met the fixed 500 ms p95 threshold;
C4 exceeded its 1,000 ms threshold. The archived qualification is **false**.

The C4 tail is concentrated in address searches (122 requests, p95 1,526 ms) and
ordered searches (112 requests, p95 1,195 ms). Other families were much smaller;
these counts do not establish broad per-family performance. No stage/CPU-throttle
profile was retained, so the cause is not established. Inspect those request
phases and resource contention before choosing another optimization.

## Runtime conditions and retained failure

Private quotas totalled 2 CPU/2 GiB: database 1.25 CPU/1,408 MiB, API .5 CPU/512 MiB,
worker .25 CPU/128 MiB. The deployed API normally has 1 CPU: this qualification's
lower API CPU quota can affect tails and must not be hidden when interpreting
results. Shared HDD/cache conditions were uncontrolled; no cold-host cache purge,
SSD pool, Bitcoin pause or production configuration change occurred. Client HTTP
transfer/JSON/hash time is included. Public rate-limit/WAF behaviour is outside
this internal load test; application query/HTTP deadlines were unchanged.

The first private bootstrap failed before HTTP requests because metrics were
configured on a non-loopback listener. The deployed server correctly rejected
that configuration. Its failed container, logs, false summary, image and completed
plan proof remain retained. HTTP attempt two kept metrics on loopback and read
only numeric counters by exec inside the API container after readiness checks.
It reused the completed plan checkpoint without replaying a timed HTTP stage.

All owned jobs/API/private PG and the maintenance lease are stopped. Original
private PG quotas were restored and public health/serving image verified unchanged.
Private adapter objects and reports remain retained. No source/live derived rows,
indexes, Cloudflare rules or public edition visibility were changed.

## Reproducibility and next step

Homelab keeps the detector under `docker/ukinfo/full-layout/plan_proof.py` and the
protocol, controller, tests and HTTP continuation under
`docker/ukinfo/layout-qualification/`. Initial code commit: `c8e828d`; guarded
loopback continuation: `1610d1b`. Eight helper tests passed on Linux, plus two local
controller readiness tests. Independent read-only reviews checked scope, cleanup
and continuation guards.

NAS root: `/home/jonw/ukinfo-backups/layout-qualify-2026-11-09`. The initial plan
proof is `reports/plans.json`; HTTP evidence and final false qualification are in
`reports/http-attempt-2/`. Image two is
`sha256:4849a87a5f1d230c034e2244fb1094401fd0a3e8d78c271d9266747183b229a9`.
Local aggregate logs/state persist under gitignored `private/operations/`.

Next: profile the slow address/ordered request phases and CPU throttling, then
predeclare any changed resource allocation or code variant before measuring it.
Keep the 1 s C4 gate and original observations. HTTP cursor traversal and eventual
multi-edition staging also remain required before a broad rollout.
