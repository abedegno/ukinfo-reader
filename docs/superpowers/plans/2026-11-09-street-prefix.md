# Street-prefix serving implementation plan

> Execute inline with TDD; request independent review before private qualification.

**Goal:** Implement the measured redundant street-prefix condition and verify the exact binary privately.
**Architecture:** Preserve scalar matching and cursor ordering; add a bound, escaped regex OR across street/street_2 only for a trailing street wildcard. Native paths and exact streets stay unchanged.
**Spec:** docs/research/2003-street-prefix-spike-2026-11-09.md on docs/v14-hosted-qualification; homelab docker/ukinfo/street-prefix/README.md.

## Constraints

Deployed ce6117f server plus this change only. No production/source database mutation or full-index rebuild. NAS builds max 2 CPU/2 GiB; private PG/API/client max 2 CPU/2 GiB. Real queries/records/cursors/hashes remain NAS. Keep original failures and 1 s C4/500 ms C1 gates. No SSD, storage migration, Bitcoin pause or automatic deployment.

## Tasks

- [ ] Failing SQL tests: bound literal prefix, second-street OR, exact/no-street controls, combined postcode, continuation bindings and scalar-only opt-out. Observe red.
- [ ] Minimal paradeSQLBounds change; focused and full Go checks.
- [ ] Synthetic HTTP native/candidate full-pagination equality: punctuation/Unicode/second/both/empty street, tied names and business kinds, on disposable NAS fixture DBs.
- [ ] Independent review; resolve important findings.
- [ ] NAS build from exact commit, image/source provenance. One unchanged 127-case/635-request instrumented private HTTP cohort; no timed retry.
- [ ] Three real HTTP cursor-page comparisons for failed street case against unchanged baseline on the same private index; export numeric checks only.
- [ ] Verify cleanup/restored quotas/public image/health, preserve gates, document and push only owned changes in both repos.
