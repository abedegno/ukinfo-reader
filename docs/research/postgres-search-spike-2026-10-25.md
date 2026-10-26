# Small PostgreSQL search comparison — 25 October 2026

**Decision: continue evaluating denormalised PostgreSQL. Shortlist targeted
native indexes and ParadeDB; there is no evidence here requiring a move to a
JSON/NoSQL database. Do not deploy an extension or globally force custom plans
based on this small experiment.**

[Aggregate results and plans](postgres-search-spike-2026-10-25-results.json).
[Reproducible runner and methodology](../../tools/search-index-spike/README.md).
[Benchmark client](../../server/cmd/search-index-spike/main.go).

## Scope

One million **entirely synthetic** listings, generated independently in SQL and
Go, with common names, skewed town sizes, counties, dummy postcodes, street words,
business classifications and five edition labels. All candidates use flattened
fields. This is not a replay of production queries or source data, nor a benchmark
against the current compact production schema.

A local native ARM64 Docker server runs PostgreSQL **18.6**, `pg_search` **0.26.3**,
`pg_textsearch` **1.5.1**, `btree_gin` **1.3** and prerequisite `vector` **0.8.7**.
Limits: 2 CPUs / 2 GiB, 512 MiB shared buffers, 32 MiB work memory, query parallelism
off, JIT off, 10-second query statement timeout. PostgreSQL's I/O method is
`worker`, with effective I/O concurrency 16. No network/ports are exposed.

Both search extensions register the legacy access-method name `bm25`; creating
both in one database fails with SQLSTATE 42710. They were consequently installed
in separate databases on the same resource-limited server. This is a concrete
packaging finding, not evidence that their runtimes cannot coexist in a cluster.

Candidates:

- **Targeted B-tree**: common name/town composites, name/ID, town, county,
  postcode, business classification, and native GIN street-word search.
- **Multicolumn GIN**: `btree_gin` structured fields and a street `tsvector` in
  one index, with an ordinary primary key available for ordering.
- **ParadeDB**: one search index over literal fields, integer filters, street
  tokens and ID order. An explicit `pdb.all()` condition enables its custom scan.
- **pg_textsearch**: Boolean/prefix search over field-qualified tokens in one
  text column, followed by stable ID ordering. It is not tested as a ranked BM25
  document search engine.

Every table has identical row columns and a primary key. Auxiliary text/vector
representations are present in all tables, so this compares the selected indexes
and query strategies rather than minimising each engine's row storage. The
field-qualified tokens use restricted synthetic ASCII values; this is not a
production solution for Unicode, punctuation, compound names or field escaping.

## Fixed first-page results

Twelve cases × four rounds = **48 first-page attempts per engine per client
level**, always retrieving at most 50 IDs plus one lookahead. Failed attempts stay
in the percentiles. Reported times include client/pool, planning and query/ID
retrieval over a Unix socket, not HTTP or decoding full archived records.

The initial run retained ordinary prepared-statement reuse (`plan_cache_mode=auto`).

| Candidate | 1-client p50 / p95 | 4-client p50 / p95 | Errors, 1 / 4 clients |
|---|---:|---:|---:|
| Targeted B-tree | 4 / 235 ms | 4 / 124 ms | 0 / 0 |
| Multicolumn GIN | 18 / 301 ms | 5 / 726 ms | 0 / 0 |
| ParadeDB | 12 / 26 ms | 16 / 130 ms | 0 / 0 |
| pg_textsearch | 59 / 5,970 ms | 180 / 10,000 ms | 0 / 3 |

A follow-up reused the fixtures and set `force_custom_plan` for **all** candidates.
It repeated the independent correctness checks and the same timed workload.

| Candidate | 1-client p50 / p95 | 4-client p50 / p95 | Errors, 1 / 4 clients |
|---|---:|---:|---:|
| Targeted B-tree | 1 / 27 ms | 2 / 37 ms | 0 / 0 |
| Multicolumn GIN | 2 / 94 ms | 2 / 131 ms | 0 / 0 |
| ParadeDB | 5 / 14 ms | 6 / 54 ms | 0 / 0 |
| pg_textsearch | 13 / 38 ms | 17 / 93 ms | 0 / 0 |

**These are separate, increasingly warm runs, not a controlled estimate of the
speedup caused by custom plans.** Both rotate engine execution order. The explicit
prepared-plan diagnostics below independently establish concrete plan changes.
The four-client stages interleave engines on one server and must not be read as
independent per-engine throughput/capacity tests. With 48 observations, p99 is the
maximum; it is retained in the artifact but is not a production p99 estimate.

## What the plans show

`PREPARE` plus `EXPLAIN (ANALYZE, BUFFERS) EXECUTE` was run under both forced
custom and generic planning after the timed workload. A fresh `EXPLAIN` alone
would not reproduce the generic plan used by repeatedly prepared queries.

- **Exact name + town:** the targeted native index returns the page through an
  index-only scan, about **0.15 ms and 4 buffers** in the custom-plan diagnostic.
  The native multicolumn GIN generic plan instead builds/sorts a bitmap result,
  about **569 ms and 9,806 buffers**. Custom planning chooses its primary-key
  early-stop path, about **2.5 ms and 139 buffers**. Native GIN is not automatically
  the best index for ordered pages.
- **Empty name + town:** pg_textsearch's custom plan uses its search index,
  about **0.23 ms**. The generic plan walks the primary key and evaluates text
  conditions, about **6,547 ms and 30,024 buffers**. All three measured timeouts
  were this empty-result case at four clients. The custom-plan follow-up had no
  timeouts; this supports a plan-selection diagnosis for this fixture, not a
  claim that the extension inherently takes seconds to find empty results.
- **ParadeDB:** all captured cases use `TopKScanExecState` custom scans under both
  plan modes, including structured filters, prefix ranges and empty results.
  Its stable plan shape is encouraging for flexible combinations.

Targeted B-trees are particularly good when the composite index matches the
requested fields. ParadeDB's benefit is handling more combinations within one
index; it does not win every individual case or concurrency setting.

## Pagination, correctness and storage

Before timing, **144 pages per run** matched the independent Go oracle, with
**16 complete result-set walks**. Timed requests checked exact IDs and lookahead
again. Each run made **1,024 timed requests**: the first had three timeouts and no
returned-content mismatches; the follow-up had no errors or mismatches.

Up to three pages are timed for each case. Later pages remain important:
pg_textsearch's third business-classification page reached **1,461 ms** even with
custom planning. Its all-page p95 was 49 ms / 110 ms at one/four clients, but
all-page p99 reached 1,250 ms / 1,426 ms. First-page results alone would hide this.

| Candidate | Index storage, including primary key | Copy + index + vacuum time |
|---|---:|---:|
| Targeted B-tree | 221.6 MiB | 8.9 s |
| Multicolumn GIN | 108.3 MiB | 6.8 s |
| ParadeDB | 80.7 MiB | 9.9 s |
| pg_textsearch | 120.0 MiB | 12.9 s |

Each common table occupies about 205.8 MiB separately from its indexes. Build
times include copying already generated seed rows and vacuum/analyze; seed
creation, package installation and total WAL/peak storage are not included.
These sizes must not be multiplied by 249 to promise full-archive requirements:
real value distributions and edition partitioning can change compression/indexes.

## Follow-up

The [full 2003-edition NAS comparison](real-search-spike-2026-10-26.md) now
tests the two finalists against real data. The original next-test scope was:
**targeted native PostgreSQL indexes and ParadeDB**. Include flexible combinations,
empty results, later pages, actual record retrieval, lexical sorting and the
serving role. Keep timeouts, permission isolation and result completeness intact.

Before considering deployment, ParadeDB also needs a verified backup/restore and
upgrade procedure, edition visibility tests, and desktop packaging assessment.
Its Community/Enterprise replication and licensing differences are described in
[the vendor's edition comparison](https://www.paradedb.com/docs/operate/deploy/enterprise).
This experiment does not make a licensing or distribution decision.

No public/NAS service or production data was changed. Synthetic database volumes
were removed. The pinned local test image/build cache is retained for reuse.
The documented runner also passed a separate 10,000-row end-to-end smoke run,
including prepared-plan diagnostics and automatic container/volume cleanup.
