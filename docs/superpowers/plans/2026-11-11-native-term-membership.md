# Native exact-name/town timeout

Goal: remove the reproduced native name/town timeout while preserving results, deadlines and release gates.

## Evidence and design

The baseline private request returned HTTP 503 in 2.275 seconds, with a 2.169-second source probe. Actual plan analysis subsequently showed 370,555 matching vocabulary terms across workers. Diagnostic EXPLAIN uses 32 MiB work memory; serving remains at its existing settings. EXPLAIN timing is not serving latency evidence.

Use relational first-name term membership when an independent exact surname anchors the search. Keep other broad surname/postcode/unanchored paths unchanged. The synthetic old probe failed its 500 ms budget; the replacement completed in 61–84 ms under auto, forced-custom and forced-generic plans.

The source improvement exposed a second bottleneck. The forward town side expanded 706,369 occurrences and made 20,941 posting lookups, with 4,581 shared blocks read. A private JIT-off diagnostic still took 4.13 seconds and did not resolve the request. No JIT setting change is proposed.

For exact person surnames with town filters in catalogue order, use the existing complete reverse address projection after both bounded probes overflow. Read at most 65,536 ordered source IDs plus one sentinel, then close the source rows before querying address metadata. Filter in batches of 1,024. A complete page plus lookahead proves the ordered page; source exhaustion proves the last page. An overflowing prefix without enough matches uses the complete original forward path. The prefix is a work budget, never a result cap.

Source IDs occupy about 512 KiB per request. Closing rows before metadata lookups avoids nested pool borrows, including a single-connection pool and separate APIs sharing a small pool. Missing optional projection coverage retains the forward route; missing required blocks and cancellation fail explicitly. Street, locality, county, grouped, sorted and private experimental routes remain unchanged.

## Verification

- [x] Observe the old broad vocabulary probe fail its budget and the replacement pass.
- [x] Observe the old native town path fail the bounded reverse-work regression.
- [x] Validate all pages, multiple editions, dataset ties and given-name refinement.
- [x] Validate missing optional coverage, corrupt blocks, cancellation and no connection leaks.
- [x] Observe the held-cursor design fail with one connection; validate the replacement with one connection and two APIs sharing two connections under four concurrent requests.
- [x] Validate matches beyond source position 65,536 use complete fallback and paginate without truncation.
- [x] Run the isolated PostgreSQL catalog suite: passed in 39.138 seconds. The separate pg_search integration requires its dedicated CI service and was skipped in this source-only fixture.
- [x] Run local Go tests and vet; independent review found no remaining blockers.
- [x] Replay the saved failing request once with the final candidate: HTTP 200 in 895.169 ms, with normal serving JIT and unchanged deadlines. This is a warm diagnostic, not a cold or capacity claim.
- [ ] Pass application CI, merge and qualify the exact immutable runtime through the unchanged private protocol.
- [ ] Promote through the guarded release controller; retain failures and verify public catalogue and telemetry.

SQL, query values, case IDs, hashes and raw plans remain NAS-only. Live diagnostics are read-only; synthetic PostgreSQL is networkless and tmpfs-only. No public saved-search replay, database copies, cache eviction, relaxed gates or unrelated worktree changes.
