# Indexed scalar postcode search implementation plan

> **For agentic workers:** Execute inline with executing-plans, TDD and one whole-change review. User autonomy authorization persists.

**Goal:** Adopt the verified scalar postcode route with complete index readiness and unchanged paging semantics, then qualify v14.
**Architecture:** A precise eligibility helper selects an explicit scalar mode in the existing ParadeDB SQL/cursor builder. A strict index contract guards search and projection creation/migration. A serial NAS owner worker provisions all six complete indexes under the reviewed exclusion/cleanup protocol.
**Tech Stack:** Go1.26.11/pgx5, PostgreSQL18.7/pg_search0.26.3, Python NAS controller.
**Spec:** ../specs/2026-11-17-postcode-scalar-search.md

## Global constraints

No paradeEdition serialized-field or paradeVersion changes; no source-generation/table rewrite. No handler DDL, source retry, population cap, cache manipulation, deadline/resource relaxation, local Docker/data copies or visibility change. Preserve original cohorts and all release gates. Existing index compatibility name is intentional to avoid a duplicate3.3GiB index/rename. Private values/plans/generation identifiers stay NAS-only.

### Task1: Scalar route and strict index lifecycle

- [ ] RED eligibility/SQL/seek-binding/index-contract/generation-invariance tests.
- [ ] Implement isolated index contract/helper; add explicit scalar mode to SQL builder without changing cursor construction or legacy helpers; gate new route on exact ready index.
- [ ] Add covering index before new ready publication and concurrently for existing ready projections under existing builder locks; reuse only verified exact definitions.
- [ ] Run focused/full Go tests, race/vet and source serialization checks; independent review and exact CI.

### Task2: Synthetic API and rollout fixtures

- [ ] Prove complete multi-page person/business/Unicode/partial/tie/empty/all-edition equivalence and cursor binding; prove missing/malformed index fails closed and other routes stay unchanged.
- [ ] Build and pin candidate runtime artifacts through existing NAS build workflow after exact source/main CI, retaining original private clients/reference bundle.

### Task3: Provision all six full-edition indexes

- [ ] Extend reviewed owner policy/controller with strict frozen six-edition inventory and generation binding, existing-index reuse, serial missing-index creation and aggregate progress; RED/GREEN ownership/resource/cleanup tests.
- [ ] Rehearse invented data; review/full policy CI; then start one fresh real operation under fixed maintenance/resources/lease guards.
- [ ] Independently validate all definitions/generations, cleanup/backends/public six identities and all77 public references; preserve failures.

### Task4: Qualification and guarded publication

- [ ] Run unchanged five/six correctness/field/preparation/ordered/cursor/load/cold/reference gates against the new pinned runtime. Diagnose remaining grouped/legacy/live failures from retained evidence without narrowing cases or budgets.
- [ ] Publish only after every gate passes, verify public six scope, rollback and post-publication checks; update reader/homelab docs. Mark goal complete only after actual verified live/performance requirements are met.
