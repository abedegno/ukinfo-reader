# Broader SIC-routing binary regression — 10 November 2026

**The broader load gate failed:** all 1,775 requests ran, but seven initial
requests and one C1 request returned HTTP 503. C1/C4 p95 passed at 176/393 ms;
that does not override the zero-error gate. All 344 pageable cursor cases passed
with no errors/mismatches. Production is unchanged and v14 remains hidden.

## What this qualifies

The [private hybrid trial](2003-hybrid-sic-routing-2026-11-10.md) passed its 2003
cohort using the retained partitioned main and unpartitioned SIC copies. Those
private copies contain only 2003. This follow-up uses the existing six-edition
unpartitioned target to check the same binary across the entire frozen workload.
It requires no full database/index copy and makes no production configuration change.

That six-edition target has no optional SIC registry. Flag-on must therefore use
the ordinary main path. This is a missing-registry compatibility/regression test;
it does **not** qualify a combined six-edition hybrid index configuration. The
reference for cursor checks is flag-off ParadeDB using the same binary, not the
native backend despite the frozen tool's historical side names.

## Controls fixed before execution

Exact source `66876ae74cb6d2b623d37733f7018dc405427f55`, image
`sha256:6a2fa352a421073cb53d1b886f09a7e804cbe12b568c96a94db0b1afb463ef03`;
no rebuild. All 355 original frozen case definitions and hashes, unmodified:
initial 355, two rounds at C1 and C4 (710 each), 1,775 requests. Original gates:
zero errors/unverified/mismatches/budget exhaustion, C1 p95 <=500 ms and
C4 p95 <=1,000 ms. No adaptive replay or relaxed limit. Client timeout 8 s;
application deadlines unchanged, ParadeDB SQL timeout 4 s.

Private API 0.5 CPU / 512 MiB, load client 0.25 CPU / 128 MiB. Existing source and
derived databases remain at 2 CPU / 6 GiB each; neither restarts nor changes
limits/settings. Retained target settings are shared_buffers 1 GB, work_mem
32 MB, maxparallel 0 and I/O timing off. The launcher's metadata session reports
plan_cache_mode auto; the exact API binary overrides its ParadeDB pool to
force_generic_plan (source main.go), as in the 2003 trial. The buffer/work memory
settings differ from that trial; its latency is not a controlled direct comparator.
No cache flush or cold-storage claim.

Before serving, sealed read-only connections verify six source/target entry
counts, complete watermarks, version 1, main aliases and absent optional registry.
A small standard-library Go helper uses the exact application generation format:
ordered exported fields, raw manifest JSON, UTC time and SHA256. The pre-existing
known generation fixture is checked before hashing private metadata via stdin.
Each target dataset ID, generation and count must match its source; matching
counts alone would allow stale generations to silently use native fallback.
The source inventory includes the selected v14 dataset only in this private test;
all six generations cover 292,695,740 occurrences. No full row count scan or
original-data export is required for this metadata check.

The metadata helper is a locally compiled 3.34 MB Linux amd64 code-only binary,
not a local image/database copy. Source metadata, generation values and all
query/response evidence stay on NAS. Only code hashes and aggregate counts are
exported. No owner credential, DDL, new role or index/data copy.

After timing, flag-off reference starts. The frozen cursor checker processes all
355 case definitions, comparing eligible first pages and one continuation using
both own-side and opposite-side tokens; it also requires two changed-query
binding rejections. A wrapper explicitly rejects any request error, missing
first-page match, failed continuation or binding check even when the tool's exit
code alone reports success. Two contract tests cover incomplete/error/over-budget
loads and cursor failure paths; observed red before implementation, then green.

Unique internal bridge aliases isolate the frozen tool's fixed hostnames from
older services. Only APIs also join the database network; clients have no database
credentials. No published ports. Maintenance lease and immutable image/runtime
checks guard execution. All owned services stop in closeout; the retained private
hybrid PG stays stopped and untouched. Stopped evidence containers/network remain.

## Observations

| Stage | Requests | HTTP 503s | Median | p95 | Maximum |
| --- | ---: | ---: | ---: | ---: | ---: |
| Initial | 355 | 7 | 19.090 ms | 1,394.773 ms | 5,003.702 ms |
| C1 | 710 | 1 | 6.835 ms | 176.019 ms | 4,001.800 ms |
| C4 | 710 | 0 | 13.229 ms | 392.784 ms | 3,005.148 ms |

The other 1,767 requests returned 200 and matched their original frozen hashes;
there were zero digest mismatches, unverified cases or budget exhaustion. Scope
was 127 explicit 2003 cases, 57 all-edition cases and 171 other edition scopes.
No request was replaced or retried to make the gate pass.

Load counters recorded 1,475 ParadeDB decisions, 225 fallback, 40 radius-stream
and 5 source-prefix-parallel decisions. SIC overlays were zero, as required by
the absent registry. These counters describe plan decisions and may overlap;
do not sum them as an exclusive request partition. Source/index identities were
matched for all six editions, preventing stale readiness from silently selecting
native fallback. Eight candidate-phase errors totalled 35.326 s; successful
candidate phases totalled 140.814 s / 1,467 calls. Inventory was 9.564 s / 1,475,
hydration 4.833 s / 2,357. Phase sums overlap and are not wall-clock attribution.

All 344 pageable first-page pairs matched. Of those, 268 had no continuation and
76 passed own/cross-flag continuation checks. Both changed-query token-binding
checks were rejected. This is 994 HTTP checks: 688 first-page requests, 304
continuation requests and 2 binding rejections. Zero request errors or response
mismatches; identical bounded routing counters on both flag settings. The other
11 frozen cases are nonpageable routes; all 355 were included in the load test.

A networking-disabled helper classified all eight failed requests from saved
reports, without HTTP/SQL replay or exporting case ordinals or predicates:

| Stage | Scope and person-search shape | Failed requests | Maximum |
| --- | --- | ---: | ---: |
| Initial | One edition, postcode only, name order | 3 | 4,126.814 ms |
| Initial | All editions, first name + surname + town | 1 | 5,001.182 ms |
| Initial | All editions, same name/town fields, listing grouping | 1 | 5,003.702 ms |
| Initial | All editions, town only, listing grouping | 1 | 5,001.609 ms |
| Initial | All editions, town only, edition order | 1 | 4,149.652 ms |
| C1 | One edition, postcode only, name order | 1 | 4,001.800 ms |

These are field-presence shapes, not exact predicates; no unique failing-case
count across stages is inferred. All failures concern person searches, not SIC.
The report records HTTP 503s; elapsed times near deadlines are consistent with
timeouts but do not alone establish which per-request database operation caused
each response. No cold-cache or root-cause claim follows from first-pass timing.

## Interpretation and operations

Cursor portability and missing-registry behavior passed across the broader
workload, but load reliability did not. Preserve this failure alongside the
passing private 2003 hybrid result; neither replaces the other. Before release,
focus on the saved postcode/name-order and all-edition town/grouping shapes,
using bounded request/plan evidence to separate first-access I/O, index work and
multi-edition orchestration. Repeatedly rerunning a warmed cohort would not prove
reliability. Combined cross-edition hybrid routing remains a separate requirement.

Fresh closeout checked all five helpers stopped, maintenance lease released,
retained private hybrid PG still stopped at original limits, and unchanged live
source/target/API image, start times and quotas. Public health passed and public
inventory remains five editions / 248,951,107 occurrences; v14 is not published.
No deployment, database restart, storage migration or Bitcoin change.

Configuration as code and safe numeric evidence: homelab
`docker/ukinfo/hybrid-broad/`. NAS evidence root:
`/home/jonw/ukinfo-backups/hybrid-sic-broad-2026-11-10`.
Queries, record IDs, source generation hashes, original ordinals, raw plans,
response hashes and opaque cursor tokens are excluded from git.
Independent prelaunch review checked metadata generation fidelity, isolated
aliases, immutable image, strict cursor gates, privacy and cleanup. It found and
resolved the counts-only readiness gap before launch.

Final independent review verified accounting/privacy/cleanup and clarified that
metadata-session automatic planning is distinct from the API's forced generic plans.
