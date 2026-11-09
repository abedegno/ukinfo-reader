# SIC tail optimization implementation plan

**Goal:** Test one SIC index-prefilter policy against the unchanged 127-case HTTP cohort.
**Evidence:** Captured C4 ranking:13/14 slow cases use SIC; three generic plans still use scalar expressions despite Top-K. Direct read-only inspections took1236/634/366ms.
**Approach:** Bound exact term for exact SIC, escaped regex prefix for trailing wildcard; retain scalar SIC/nonempty/kind/town checks and all ordering/cursors. An index rebuild or resource increase would confound the query change and is outside scope.

- [ ] Observe failing SQL tests; add minimal predicate; run full Go suite.
- [ ] Extend synthetic native/candidate HTTP paging fixture with SIC exact/prefix, leading zero, punctuation, Unicode, empty/absent, combinations and all orders; retain existing street tests.
- [ ] Independent code review before exact-source NAS build and test.
- [ ] Use production-matching Alpine source and separate ParadeDB fixtures, bounded tmpfs; require both integration suites before real load.
- [ ] Run one unchanged635-request frozen cohort at existing2CPU/2GiB caps. No adaptive retries. Preserve original C4miss; C1<=500ms/C4<=1s remain fixed.
- [ ] Replay saved three successful street-candidate page queries against new binary and compare content/next availability, keeping prior baseline timeout as failed evidence. Additional real SIC page equality checks against scalar query if bounded diagnostics support them; don't substitute synthetic proof for missing real comparisons.
- [ ] Verify cleanup/public image/health/quota restoration; update PR48 scope and both repos. No deployment; other228scopes remain unqualified.

Real query strings, IDs, cursors, response hashes, raw plans and credentials remain NAS. No full-index mutation, local Docker image/DB copy, storage migration or Bitcoin pause. Build max2CPU/2GiB. Reuse reviewed harness gates with exact-source labels; one code policy, no quota/index/code variant selection after timing.
