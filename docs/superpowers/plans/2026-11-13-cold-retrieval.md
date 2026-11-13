# Cold retrieval diagnosis

Goal: reproduce the two retained private qualification failures and isolate retrieval cost before changing serving behavior.

Use the existing isolated source clone, frozen input and 4 CPU / 8 GiB fixture supervisor. Never start it outside the atomic release namespace and fixture flock. Preserve the failed 11 November acceptance. No public replay, serving restart, new database copy, global cache flush, deadline increase or v14 publication.

The first diagnostic extends only the operator profiling command: opt-in hybrid reader, at most two repetitions, optional suppression of EXPLAIN, fixed-phase span durations, record-query overlap, per-request pool acquisition deltas, process CPU and existing decode/cache counters. It does not modify catalogue SQL or request handling. CPU/phase residuals are not exact decoder timings; add finer instrumentation only if those costs dominate. SQL/args/records, case indices, raw diagnostic files and fingerprints stay on the NAS.

- [x] Add failing tests for overlap accounting and supported stage privacy, then implement bounded operator profiling options.
- [x] Run Go tests/vet; build one code-only Linux diagnostic binary with existing caches and no local Docker images.
- [x] Run one declared cold/warm diagnostic per retained failure under the contained supervisor, with stopped-source private-file eviction and original API limits. Export numeric results only. Capture stats/wait categories where supported without changing serving settings.
- [x] Form a specific hypothesis from evidence. Add a regression before any evidenced serving fix; otherwise document the unresolved difference without speculative changes.
- [ ] Review, CI and fresh exact-runtime qualification before guarded promotion. Retain every failure and keep production unchanged until acceptance passes.

## Evidence and correction

The contained cold household request reproduced HTTP 503 at 5,000.6 ms. Go CPU was 46.3 ms, successful source acquisition duration 0.027 ms (no empty acquires), and record-query interval union 3,706.1 ms. The immediate repeat still took 3,199.3 ms and succeeded. Read counters/timing are database-wide and asynchronously published, not exact per-request physical-device attribution. The postcode page-8 failure did not reproduce: first traversal page 8 was 581.5 ms, repeat 19.5 ms; record retrieval dominated the first traversal. Preserve the original failed acceptance.

A separate bounded projection probe found 857 postcode candidates across 284 raw source blocks and five editions. All 857 shared the projected street/locality/town fields. Reusing that projection cannot filter this case. No serving SQL was changed by either diagnostic.

Ruling: add an optional full-coverage premises-key projection, following the existing additive research-index pattern. SHA-256 of the existing canonical premises identity selects candidate IDs, with original raw-record identity validation retained. Unsupported/warned source addresses get NULL keys. Index every source ID, publish a count/version registry atomically, and require complete matching coverage/SELECT permissions before using it; otherwise keep the full postcode scan. Source data/record IDs and cursor fingerprints do not change. A key collision can cause extra validation work, never an incorrect join. Only matching records plus lookahead hydrate.

- [x] Test original fallback equivalence and demonstrate excessive decode work before enabling a complete compact projection.
- [x] Implement optional schema, bounded full builder and exact same-address lookup; preserve ordering, cursor behavior, raw validation and errors.
- [x] Test full/missing coverage, ambiguous addresses, businesses, multiple editions, pagination, cancellation/rollback and corrupted anchor keys.
- [ ] Prepare a separately reviewed private-clone build using an owner-only setup/build process, with SELECT-only serving measurement, resource/disk guards and no public schema changes.
- [ ] Re-run the declared cold/warm probe against the corrected lookup and unchanged frozen qualification before any public schema/application promotion.


Verification ledger: the first synthetic attempt failed its invented manifest validation and is not a valid old-path reproduction. After correcting the fixture, the old-scan control failed at 12 decoded blocks; the indexed candidate passed the bounded-read assertion and exact full-page/cursor comparison. Initial full source-only suite passed in 38.970 seconds; final strengthened suite passed in 36.199 seconds. Separate pg_search integration needs its dedicated CI service. Local Go tests/vet/public-tree/diff checks passed. Independent profiler and premises reviews found no remaining blockers after legacy removal, populated cancellation and collision tests. No real-clone index is published yet; production remains the old five-edition release.
