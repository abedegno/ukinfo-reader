# Full-edition PostgreSQL / ParadeDB test — 25–26 October 2026

**ParadeDB is a credible hosted-search candidate, with fewer timeouts and much
smaller indexes in this test. Native PostgreSQL remains faster for many ordinary
lookups. Neither result justifies replacing the production search yet.**

This follows the [one-million-row synthetic experiment](postgres-search-spike-2026-10-25.md).
It uses the **complete 2003 SE edition: 59,886,841 entries**, including 2,471,038
businesses. Both candidates use identical flattened search fields and retrieve
original records from the existing catalogue. Production services were unchanged.

- [Aggregate results, preliminary failure and sanitized plans](real-search-spike-2026-10-26-results.json)
- [Reproducible configuration and operator instructions](../../tools/real-search-spike/README.md)
- [Builder](../../server/internal/catalog/search_engine_spike.go),
  [measurement client](../../server/internal/catalog/search_engine_spike_measure.go),
  [semantic checks](../../server/internal/catalog/search_engine_spike_test.go)

## Workload and environment

The private workload contains **87 searches**, combining 36 seed cases with
positive cases sampled across the edition and explicit negative cases. It covers
people and businesses, exact/prefix names, given-name boundaries, partial postcodes,
street/locality/town/county combinations, business classifications, alphabetical
order and source-ID order. Search values, source IDs, record hashes and cursors are
not published. This is a targeted regression workload, not a random sample of user
traffic. The business industry filter is exercised negatively; positive industry
coverage has not been established.

Each stage runs two rounds, at most three pages per search, 50 displayed records
plus one lookahead. Every engine therefore has the same **174 first-page
attempts per stage**. Later pages stop on exhaustion or failure, so their
populations differ. Requests retain a **4-second SQL timeout and 5-second total
search-plus-record budget**. Failed attempts remain in the percentiles.

The isolated NAS database runs PostgreSQL **18.6**, `pg_search` **0.26.3**, Linux
amd64, with **2 CPUs / 6 GiB RAM**, 1 GiB shared buffers, 32 MiB work memory,
parallel query/maintenance workers disabled and JIT off. It has no published port.
The client has 1 CPU / 512 MiB. Measurements use a dedicated SELECT-only target
role, with read-only transactions and no TEMP/schema creation privileges; source
access is also SELECT-only.

Both search candidates share this server and alternate execution order. Four
clients mean **mixed-engine concurrent contention**, not separate per-engine
capacity tests. Original records are loaded and decoded through the production
reader from the existing, separate 2-CPU / 6-GiB source database. Timings exclude
HTTP, Cloudflare and browser rendering. They also exclude verification hashing.

There is no cache flush. Preparation, source-reference capture and earlier runs
warm caches; other NAS activity is uncontrolled. Stages run in the order shown
below. Differences between `auto` and custom stages cannot be attributed solely
to planning policy. No source import, sizing scan or index build overlaps timing.

## Fixed first-page results

Times include target search, client/pool overhead and original-record retrieval.
`auto` permits ordinary prepared-statement plan selection; it does not mean that
every execution used a generic plan.

| Plan policy | Clients | Native p50 / p95 | ParadeDB p50 / p95 | Native / ParadeDB failures |
|---|---:|---:|---:|---:|
| auto | 1 | 1.9 / 309 ms | 9.0 / 798 ms | 3 / 0 |
| force custom | 1 | 3.0 / 271 ms | 6.0 / 136 ms | 2 / 0 |
| auto | 4 | 2.9 / 429 ms | 9.1 / 247 ms | 2 / 0 |
| force custom | 4 | 3.1 / 429 ms | 7.8 / 290 ms | 2 / 0 |

ParadeDB completed all **696 first-page attempts**. Native failed nine. Native
medians are consistently lower, while ParadeDB avoids the four-second tail in
these cases. The initially slower ParadeDB p95 also matters: this is not evidence
that every real search becomes faster or consistently sub-100 ms.

## Paging and correctness

Across all four corrected stages, including later pages:

- **ParadeDB: 1,136 attempts, no failures.**
- **Native: 1,117 attempts, 11 query cancellations at the four-second SQL limit**
  (SQLSTATE `57014`). Nine were first pages and two were later pages. All were
  address-filter cases; one first-page case failed in every stage. Forcing custom
  plans therefore does not solve every native failure.
- **No returned-content mismatches** between candidates or against available
  independent source references. This includes order, lookahead, page positions
  and original-record digests.

| Plan policy | Clients | Native later-page p95 | ParadeDB later-page p95 | Native / ParadeDB later failures |
|---|---:|---:|---:|---:|
| auto | 1 | 262 ms | 495 ms | 1 / 0 |
| force custom | 1 | 268 ms | 550 ms | 1 / 0 |
| auto | 4 | 470 ms | 1,062 ms | 0 / 0 |
| force custom | 4 | 475 ms | 1,030 ms | 0 / 0 |

ParadeDB's worst measured request was **2,004 ms**, on a later page with four
clients. Its later-page medians were 103–221 ms, compared with native's 7–12 ms.
This is a material trade-off, hidden by reporting first-page p95 alone.

The existing production search independently returned **130 successful reference
pages**; four reference attempts failed and were recorded as unavailable, never
as empty results. References were privately cached against the exact workload
and source generation, then reused. Unreferenced results still receive paired
candidate comparisons, but agreement between two candidates is weaker evidence
than an independent oracle. This is not a complete result-set proof for every
broad search.

Preparation read every original compressed block, checked envelopes, counts and
ID continuity, and compared **58,484 sampled rows** with the existing separately
stored name/postcode, address and research indexes. Both finished tables have the
complete count and ID range. Invented-record tests passed locally with Go's race
detector and on the actual NAS ParadeDB version, including punctuation,
Ann/Anna boundaries, multiword names, postcode precision, secondary address
components, hidden-field suppression and lexical continuation.

## An important failure before the corrected run

The preliminary adapter used a PostgreSQL row comparison for alphabetical
pagination, including an unnecessary initial-page comparison against empty
strings. On ParadeDB this prevented Top-K pushdown: plans used **index scan plus
sort**, and **all 90 alphabetical first-page attempts timed out**. Native also
had 11 timeouts in that preliminary run. No content mismatch was returned.

The corrected initial page uses only the positive-ID bound. Subsequent native
pages retain row comparison; subsequent ParadeDB pages use the equivalent scalar
expression:

```sql
name > :name
OR (name = :name AND first_full > :first)
OR (name = :name AND first_full = :first AND id > :id)
```

All sort keys are non-null and use C collation. Duplicate-key continuation is
covered by tests. Sampled corrected first-page plans use `TopKScanExecState`
under both explicit custom and generic planning. Actual later-page timings and
content comparisons then passed. This is an access-path compatibility finding,
not evidence that row comparisons are generally slow in PostgreSQL.

The failed preliminary report and nonexecuting plans are retained in the public
aggregate artifact. Their zero buffer/work counters mean **not measured**. They
must not be compared with the later `ANALYZE, BUFFERS` counters as if both were
execution measurements. Native also benefits from the corrected initial bound;
the preliminary run is not a controlled estimate of any single change's speedup.

## Plans and storage

Post-timing `PREPARE` / `EXPLAIN (ANALYZE, BUFFERS) EXECUTE` captures custom and
generic execution for ten fixed cases per candidate. All twenty captured
ParadeDB plans use Top-K. Native uses targeted index scans, index-only scans and
bounded sorts. For one captured case, custom planning used the covering name
index in about **0.55 ms / 10 buffers**; generic planning used the town index
with a residual filter in about **980 ms / 34,806 buffers**. These diagnostic
execution times exclude original-record loading and are not request percentiles.
The fixed diagnostic sample does not include every timed-out native case.

| Complete 2003 search copy | Table storage | Index storage, including primary key | Combined |
|---|---:|---:|---:|
| Targeted native indexes | 6.05 GiB | 22.78 GiB | 28.83 GiB |
| ParadeDB literal-field index | 6.05 GiB | 4.24 GiB | 10.29 GiB |

Native has fourteen B-trees, including separate nonempty secondary
street/locality indexes. ParadeDB has its primary key and one wide search index.
The measured ParadeDB indexes are about **81% smaller** than this native index
set. This does not establish that the native set is minimal.

These are additional search-copy sizes, excluding the existing original-record
store, other editions, WAL, backups, temporary build space, grouping and geo
indexes. This is literal exact/range field retrieval, not a BM25 relevance test
or a benchmark of JSON/NoSQL storage.

Total preparation wall time was **2 h 18 min**, including operator iterations,
interruptions and recovery; it is not a clean build-speed comparison. Recorded
native index phases total about 43 minutes, ParadeDB copy/primary-key creation
6.5 minutes and its search-index phase 25 minutes. Source decode/copy, final
vacuum/count validation and interruption overhead are outside those comparable
DDL phase totals. Do not extrapolate these sizes or durations to all editions by
simple multiplication.

An interactive diagnostic process caused a test-only restart during preparation;
committed checkpoints recovered, and all final checks completed before timing.
Production containers remained healthy with zero restarts. The final prototype
configuration uses an init process and noninteractive diagnostics. The test
server is now stopped; its expensive derived volume is retained privately for
follow-up. The NAS still has about **22.5 TiB free**. No public route, production
image, source data or timeout was changed.

## Decision and next gate

My inference is to **keep PostgreSQL and advance ParadeDB as a hosted search
index candidate**, preserving the existing original-record store. It handled this
varied workload without timeouts and used substantially less index space. A
universal speed claim or an immediate production replacement would overstate the
evidence: native wins common lookup latency and later-page speed, and the
ParadeDB adapter needs extension-aware SQL.

Before a rollout, test all editions and cross-edition deduplication/visibility,
nearby and premises filters, deeper pages, and each engine under separate
concurrent load. Establish backup/restore and upgrade recovery, check the actual
distribution/licensing requirements, and assess desktop extension packaging.
Retain a reversible serving switch. Four mixed clients and three pages do not
establish public Internet capacity or desktop portability.
