# Broad first-name membership implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use executing-plans inline, then request one whole-change review.

**Goal:** Prevent exact-surname/postcode searches from probing the entry index once per expanded first name.

**Architecture:** Keep the complete materialized vocabulary and use a Boolean membership fence for broad first-name expansions when an exact surname and postcode are present. PostgreSQL can hash the membership set while retaining the postcode/surname entry anchor. Other shapes keep their current SQL. No global planner settings or new indexes.

**Tech Stack:** Go, pgx, PostgreSQL 18.7.

**Spec:** Existing source deadlines, ordering and cursor contracts; diagnosis below.

## Global constraints

Source SQL 2s, target SQL 4s, pools 8/4, unchanged original acceptance. Archive inputs and full plans stay NAS-only. No release or v14 publication before full acceptance. Preserve wildcard, Unicode, middle-name refinement and pagination.

## Review focus

- Broad matching tails remain complete beyond 8,192 terms.
- NULL membership preserves WHERE semantics.
- No-postcode and unanchored searches retain their previous plan.
- Both custom and generic prepared plans stay bounded.
- Continuation ordering and grouped/refined paths remain compatible.

## Diagnosis and alternatives

The failed .15.4 case's custom plan estimated one matching term but read 9,701, then made 9,701 name-index probes before postcode filtering. A retained true-prepared-plan experiment changed only `first_id IN (...)` to `(first_id IN (...)) IS TRUE`: all ordered values matched, custom entry probes fell to one and buffer hits fell from 42,313 to 3,553. Caches were uncontrolled. Blanket generic planning could regress other shapes. A new covering index cannot address the observed zero-heap-fetch plan. Routing more shapes to ParadeDB requires a broader separate equivalence study. The small membership fence is the chosen implementation judgment under the user's autonomous-fix authorisation.

### Task 1: Membership regression and narrow fix

- [x] Add an invented broad valid-name tail and selective postcode regression with deliberately low-resolution vocabulary statistics; prove the old custom plan repeats entry probes.
- [x] Fence membership only for exact surname plus postcode; rerun custom/generic work and complete pagination/Unicode controls.
- [x] Run full Go tests/vet and the full disposable PostgreSQL catalog suite; request independent review.
- [x] Record numerical diagnostic evidence and qualification failure in project documentation; commit the reviewed change.

### Task 2: Immutable runtime qualification

- [ ] Create a new immutable runtime/profile after review; retain failed .15.4 receipts unchanged.
- [ ] Run the original full five-edition qualification without changing client, cache rules, deadlines or workload.
- [ ] Only if accepted, advance deployment and six-edition qualification; never claim v14 live beforehand.

## Execution ledger

2026-11-16: source regression RED (9,703 custom entry probes; generic control passes) -> GREEN using named PREPARE/EXECUTE. Full disposable PostgreSQL suite: 46 pass, one target-dependent skip. Go tests/vet/diff checks pass. Review identified direct EXPLAIN did not test the prepared generic plan; corrected the test and repeated RED->GREEN plus the full suite. No production correctness findings. No live or selected-edition changes.
