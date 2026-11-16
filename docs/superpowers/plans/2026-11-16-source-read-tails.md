# Source-read tail implementation plan

> **For agentic workers:** Execute inline with executing-plans; request one independent review before merge.

**Goal:** Remove the two remaining full-qualification timeouts without increasing deadlines or weakening acceptance.

**Architecture:** First bound canonical source group-member reads to 16 distinct groups per query, at most two concurrent reads, preserving scalar edition pruning and all complete counts. Then benchmark a narrow all-edition postcode-only route through the existing ParadeDB numeric cursor path against native execution. Adopt routing only on measured equivalence and benefit.

**Tech Stack:** Go/pgx, PostgreSQL 18.7, existing ParadeDB projection.

**Spec:** User-approved ordering in the conversation: grouped batching, postcode benchmark, unchanged qualification, then deployment and six-edition validation. Retained failure: homelab given-membership/evidence/failed-qualification-1.json.

## Global constraints

SQL source 2s / target 4s, pools 8/4, existing HTTP 2s/5s, two concurrent source group reads. No new database/index, local archive copies, global cache flush, query-specific acceptance warming, or changed fixed workloads. Inputs, SQL, cursors and full plans stay NAS-only. V14 remains hidden until all required proof passes.

## Review focus

- Duplicate group keys crossing batch boundaries must not multiply counts or edition membership.
- Canonical representative remains min(dataset,id), independent of completion order or edition-key ordering.
- Cancellation returns no partial result and releases pool connections.
- Empty input and missing groups retain current behavior; all valid groups beyond a batch boundary are included.
- Postcode routing must preserve partial/full precision, ordering and existing incoming cursors; unmeasured refinements remain unchanged.

## Approaches and decision

Small bounded source batches use the existing authoritative covered index and can keep individual HDD reads within the existing statement deadline. A new aggregate projection would reduce reads further but adds a data build/version/publication contract; reserve it for evidence that batching is insufficient. Longer deadlines do not meet the acceptance contract. For postcode candidates, compare the current native path with the existing derived index before choosing a route; original record decoding remains authoritative.

### Task 1: Canonical membership batching

- [x] Add an invented PostgreSQL test with 65 groups, 3 editions, duplicate input keys, literal complete count/representative/order checks, <=16 groups per query and <=2 overlapping queries. Old code must fail the work gate.
- [x] Deduplicate and deterministically order group keys, then partition each edition into bounded batches. Merge complete results with existing canonical/edition ordering. Add cancellation/missing/empty controls and verify pool cleanup.
- [x] Run RED/GREEN on disposable PostgreSQL, full catalog suite, Go tests/vet and a private ABBA full-response/page-chain diagnostic against unchanged source. Record cache limitations.

### Task 2: Postcode routing experiment

- [x] Build a throwaway overlay for all-edition person postcode-only default-order searches using existing ParadeDB results; do not commit routing from the spike.
- [x] Compare native vs overlay in ABBA order, with complete responses and a continuation chain, on the original failed shape and representative original postcode controls. Use unchanged pools/deadlines; keep all actual inputs on the NAS.
- [ ] If beneficial, add routing and native-cursor equivalence regressions first, watch RED, then implement narrow routing and run all suites. Otherwise retain native routing and investigate source record reads.

### Task 3: Review and acceptance

- [ ] Independent review, reviewed PRs and exact-main CI/advisory checks; pin a fresh immutable runtime/profile with fresh epochs.
- [ ] Original full five-edition acceptance, with no unchanged retry after failure. Deploy only if all gates pass; then resume the separate six-edition publication proof.

## Execution ledger

Pre-flight: task 2 uses the same API but compares against the completed task-1 source so changes are measured separately. Work is in the existing clean owned reader/homelab linked worktrees; original worktrees remain untouched. No production change or new data format yet.

Task 1: RED unbounded read105keys -> GREEN both plans. Full PostgreSQL suite47pass, target-dependent ParadeDB test skipped; Go tests/vet pass. Private ABBA16responses all200, exact full-body/cursor matches; warm member batching has small overhead and no cold claim. Independent review found no important issue.

Task 2: routing spike rejected: eight/eight postcode requests timed out vs sixteen/sixteen native controls completed. Actual estimated plans use Top-K; upstream v0.26.3 limit is five, not three. Ruling: keep native routing and inspect record-read ordering — evidence rejects direct derived routing; cost is a separate diagnostic before the next immutable qualification. No routing changes were copied into product source.
