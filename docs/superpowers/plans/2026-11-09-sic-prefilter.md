# SIC tail optimization implementation plan

**Goal:** Test one SIC index-prefilter policy against the unchanged 127-case HTTP cohort.
**Evidence:** Captured C4 ranking:13/14 slow cases use SIC; three generic plans still use scalar expressions despite Top-K. Direct read-only inspections took1236/634/366ms.
**Approach:** Bound exact term for exact SIC, escaped regex prefix for trailing wildcard; retain scalar SIC/nonempty/kind/town checks and all ordering/cursors. An index rebuild or resource increase would confound the query change and is outside scope.

- [x] Observe failing SQL tests; add minimal predicate; run full Go suite.
- [x] Extend synthetic native/candidate HTTP paging fixture with SIC exact/prefix, leading zero, punctuation, Unicode, empty/absent, combinations and all orders; retain existing street tests.
- [x] Independent code review before exact-source NAS build and test.
- [x] Use production-matching Alpine source and separate ParadeDB fixtures, bounded tmpfs; require both integration suites before real load.
- [x] Run one unchanged635-request frozen cohort at existing2CPU/2GiB caps. No adaptive retries. Preserve original C4miss; C1<=500ms/C4<=1s remain fixed.
- [x] Replay saved three successful street-candidate page queries against new binary and compare content/next availability, keeping prior baseline timeout as failed evidence. Additional real SIC page equality checks against scalar query if bounded diagnostics support them; don't substitute synthetic proof for missing real comparisons.
- [x] Verify cleanup/public image/health/quota restoration; update PR48 scope and both repos. No deployment; other228scopes remain unqualified.

Real query strings, IDs, cursors, response hashes, raw plans and credentials remain NAS. No full-index mutation, local Docker image/DB copy, storage migration or Bitcoin pause. Build max2CPU/2GiB. Reuse reviewed harness gates with exact-source labels; one code policy, no quota/index/code variant selection after timing.

## Recorded decision

The single policy was correct (635/635 frozen responses; all street and SIC cursor
checks passed) but missed C4 p95 at 1485ms. Actual SQL capture confirmed the policy
was emitted. Paired generic diagnostics showed mixed benefit: prefix/name roughly
593/610ms scalar versus 484/463ms policy; exact SIC shared 819 buffer accesses in
both and showed small timing differences; prefix+town warm scalar 366ms versus
policy 475/461ms. These small samples/cache conditions do not establish a global
speedup or a causal regression. Original HTTP gate stays false; no replay.

Remove the additional SIC production predicate and its implementation-specific
SQL assertion test; retain canonical SIC fixture and independent native/candidate
full-pagination regressions. Server source returns to b485487 street-only state.
All experimental commits/images/reports remain reproducible. Engine 0.26.3 source
clarifies postgres_expression placeholders are solved before scanning, so their
presence is not proof of ineffective filtering. Next benchmark existing native
SIC routing with matching contents/ordering before proposing another code change.
No production deployment or resource/index/storage change.
