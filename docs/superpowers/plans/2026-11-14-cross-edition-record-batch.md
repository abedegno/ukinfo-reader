# Cross-edition record retrieval implementation plan

> Execute inline with executing-plans; independent review before integration.

**Goal:** Eliminate serial per-edition source block reads in native pages.
**Architecture:** Flatten requested block keys across editions, fetch four per
statement with two workers, read manifests once, restore existing result order.
**Tech stack:** Go, pgx, PostgreSQL; no new dependencies.
**Spec:** ../specs/2026-11-14-cross-edition-record-batch.md

## Constraints and review focus

Preserve 2-second HTTP/SQL deadlines, four-block/two-query bounds, request cache
budget, source payloads/IDs, pagination and archive identity. Test cross-edition
ID/block collisions, missing/corrupt blocks, cancellation, duplicate requests and
cache reuse. Private clone only for real-case validation; public remains unchanged.

## Task 1: Batch native page retrieval

- [x] Add integration test exercising real HTTP pages over five invented editions;
  require two block statements instead of five, exact original payload/order and
  cursor continuation; verify failure returns no partial records.
- [x] Run the old implementation against the new test; retain expected failure.
- [x] Add cross-edition loader in record_batch.go and use it in http.go; retain
  single-edition helper behavior. Bound flattened batches globally to two reads.
- [x] Run focused and complete Go suites, race checks and vet; inspect SQL plans
  for bounded indexed access in the isolated invented PostgreSQL fixture.
- [x] Independently review changes and address findings.
- [ ] Merge through normal CI.

## Task 2: Private qualification

- [ ] Preserve failed November 13 artifacts and report the aggregate failure.
- [ ] Prepare reviewed single-use validation reusing existing completed premises
  keys; do not rebuild or overwrite the failed operation.
- [ ] Run cold retained cases and unchanged qualification with immutable images.
  Publish numeric results and stop on failure. Public rollout is a separate gate.

Verification ledger: initial invented manifest hash was invalid; that run is
not the red proof. Corrected old-reader control failed at five source block
statements versus two required. Candidate focused and expanded PostgreSQL tests
passed; full source-only catalogue suite passed in 38.829 seconds. Dedicated
pg_search integration is skipped in that source-only fixture and remains a CI
service. All Go tests, vet, targeted race tests, public-tree check and Python
unittest suite (84 tests, two skips) passed. The attempted pytest invocation was
unavailable; this project uses unittest, which ran successfully. Independent
review found no blockers after expanded bounds/fidelity/cancellation coverage.
No real-case success or acceptance is implied by these synthetic checks.
