# Street-prefix serving implementation plan

> Execute inline with TDD; request independent review before private qualification.

**Goal:** Implement the measured redundant street-prefix condition and verify the exact binary privately.
**Architecture:** Preserve scalar matching and cursor ordering; add a bound, escaped regex OR across street/street_2 only for a trailing street wildcard. Native paths and exact streets stay unchanged.
**Spec:** docs/research/2003-street-prefix-spike-2026-11-09.md on docs/v14-hosted-qualification; homelab docker/ukinfo/street-prefix/README.md.

## Constraints

Deployed ce6117f server plus this change only. No production/source database mutation or full-index rebuild. NAS builds max 2 CPU/2 GiB; private PG/API/client max 2 CPU/2 GiB. Real queries/records/cursors/hashes remain NAS. Keep original failures and 1 s C4/500 ms C1 gates. No SSD, storage migration, Bitcoin pause or automatic deployment.

## Tasks

- [x] Failing SQL tests: bound literal prefix, second-street OR, exact/no-street controls, combined postcode, continuation bindings and scalar-only opt-out. Observe red.
- [x] Minimal paradeSQLBounds change; focused and full Go checks.
- [x] Synthetic HTTP native/candidate full-pagination equality: punctuation/Unicode/second/both/empty street, tied names and business kinds, on disposable NAS fixture DBs.
- [x] Independent review; resolve important findings.
- [x] NAS build from exact commit, image/source provenance. One unchanged 127-case/635-request instrumented private HTTP cohort; no timed retry.
- [x] Three real HTTP cursor-page comparisons for failed street case against unchanged baseline on the same private index; export numeric checks only.
- [x] Verify cleanup/restored quotas/public image/health, preserve gates, document and push only owned changes in both repos.

## Recorded outcome

Implementation and bounded validation executed; checked steps mean performed,
not a passing release. Server b485487; CI selector 7a6946c has identical server
content and all 21 checks passed. Both NAS integration tests and all 635 frozen
responses matched; C1 p95 394 ms, C4 p95 1,299 ms (fails 1 s). Three candidate HTTP
pages returned 150 distinct rows. Baseline content comparison is incomplete:
a separate diagnostic matched pages 1/3 and got query_timeout on page 2. No timed
load retry or deployment. All private services stopped, original limits restored,
public health/image unchanged. Reports and preserved setup/launcher failures:
[qualification report](https://github.com/abedegno/ukinfo-reader/blob/docs/v14-hosted-qualification/docs/research/2003-street-prefix-http-2026-11-09.md),
homelab docker/ukinfo/street-app/. Runtime/source-stack/launcher corrections did
not change the committed candidate binary or fixed 635-request cohort.
