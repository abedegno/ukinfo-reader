# Postcode/name SQL capture — 10 November 2026

**The captured query already uses the postcode prefix constraint and Top-K. No
SQL change was justified by this diagnostic.** Both instrumented requests
succeeded with shared caches already exercised. The original 4,090 ms timeout
and failed qualification remain unchanged; its request had no phase trace.

## Actual request and plan

Select the sole failed initial case from the retained 127-case
[operating-budget run](2003-operating-budget-http-2026-11-09.md). It is a
postcode-only person search, alphabetical order, no grouping, limit50. Its
omitted match mode defaults to exact. The postcode is an area/district search,
not a full postcode. Predicate values/IDs/ordinals remain on NAS.

Use the same pinned ecf25b2 image/b485487 candidate, API .5 CPU/512 MiB, helper
.25 CPU/128 MiB and unchanged existing source/derived databases at2CPU/6GiB.
Same read-only connections and2s/4s source/target SQL budgets. No database restart,
index build, global setting, cache flush, local image/database copy or deployment.

The successful capture selected only same-role sessions belonging to the exact
private API container address. Source uses its private tag; target uses the
server's actual ukinfo-parade-search application name. Entire captured candidate
SQL is checked against the fixed Go builder shape and argument mapping before
preparing it. No guessed alternative SQL was analysed.

The actual statement retains kind, scalar postcode bounds and precision check,
the bound postcode literal-prefix regex, id/all predicate, name/first/id ordering
and51-row lookahead. One generic EXPLAIN-only, then one generic EXPLAIN ANALYZE
under4s: generic plans2/custom0. No custom execution or adaptive SQL variant.

| Observation | First instrumented request | Successful SQL capture |
| --- | ---: | ---: |
| HTTP status / client time |200 /423ms|200 /380ms|
| Candidate selection |234ms|156ms|
| Record loading |7.9ms|8.8ms|
| Inventory |5.9ms|5.7ms|

The client also polls activity under a .25 CPU quota. Client times therefore
include instrumentation/scheduling overhead; phase sums need not equal client
latency and are not independent capacity measurements. Both requests verified one
ParadeDB decision each. No successful-content hash comparison was performed in
these diagnostics; the original full-cohort comparisons remain the correctness
evidence.

The single actual plan took113.047ms, TopKScanExecState over4segments,51 returned
rows and51 heap fetches. It reported14,289 shared-buffer hits, zero shared-buffer
read blocks and zero temp blocks. Parent/child buffer counts overlap: do not add
them. These counters do not establish total physical I/O or distinct bytes read,
especially for extension-managed storage. No explicit Sort node or spill appeared.
Diagnostic session work_mem32MB, track_io_timing off; no setting was changed to
obtain that value.

Sparse activity samples saw7 target and1 source active/running-or-other samples.
This does not prove CPU saturation or rule out I/O. The capture's record phase
decoded16 source blocks despite a fresh API record cache. Its warm record loading
was small; that cannot exclude cold source reads in the original untraced request.

## Retained instrumentation corrections

| Attempt | HTTP requests | Plan analyses | Outcome |
| --- | ---: | ---: | --- |
| Initial preflight |0|0|Guard incorrectly required explicit match=exact; stopped before connection/request |
| Default-match continuation |1|0|Request succeeded; target capture empty because server overrides DSN application_name |
| Target-session continuation |1|1|Container-address/actual-application filter captured and verified SQL; plan succeeded |

Both harness assumptions were corrected, with failing-then-passing regression
tests. Five tests now cover omitted/explicit match, rejection of changed shapes,
source tag and target override. The first helper, all control files, failed
reports and closed containers remain retained. The second capture request was
declared before running; it repaired instrumentation rather than retrying a
qualification or selecting a faster SQL policy. Total2 capture requests,1
estimated plan,1 actual analysis. No635-request cohort replay.

## Decision and next evidence

In these warm requests, candidate selection costs much more than record loading.
The existing prefix constraint and Top-K were already present, so adding another
redundant postcode condition is not supported. The original timeout's exact
phase/cause is still unknown. Do not label it fixed from a warm113ms plan.

Next compare this captured statement against the retained postcode-partitioned
layout at matched resource budgets, collecting first-request I/O/cache telemetry.
Keep shared-cache and true cold-storage claims separate; never flush/restart live
databases for that comparison. This is a diagnostic direction, not approval to
deploy or publish v14. Prior failed HTTP/page/all-edition gates remain unchanged.
The [matched-budget retained-layout comparison](2003-postcode-layout-matched-2026-11-10.md)
subsequently completed: both partitioned arms succeeded; baseline timed out once
and nearly reached4s once. Three page pairs matched, supporting a full private
HTTP test at matched limits without changing any release gate.

Homelab `docker/ukinfo/postcode-name-plan/` preserves protocol, guarded
continuations, tests and numeric summaries. NAS roots under
`/home/jonw/ukinfo-backups/`: postcode-name-plan-2026-11-10,
postcode-name-default-match-2026-11-10 and
postcode-name-target-application-2026-11-10. Raw SQL/arguments/plans/responses stay
there. Seven owned API/helper/offline containers and the lease were verified
stopped; public health/image and source/target images/start times/limits unchanged.
