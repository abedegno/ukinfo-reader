# Postcode-partitioned index pilot — 8 November 2026

**Result: promising private pilot; no production change.** On a one-million-row
copy of the 2003 edition, postcode partitioning roughly halved measured first-query
latency and private-container block reads. All 64 queries succeeded and all 25
pagination comparisons matched. This is not full-sized release qualification.

## Why this layout

Inspection of all six installed pg_search 0.26.3 indexes found four immutable
segments each, no deleted/invisible segments and all 19 fields indexed/fast.
Their index options are default. Fragmentation or disabled fast fields do not
explain the reproduced read amplification.

The installed release supports `partition_by='postcode'` with literal text fields.
Its physical `sort_by` supports only one field, which is not a direct match for
our name/first-name/ID total ordering. Postcode partitioning changes physical
layout while retaining the existing query and ordering.

See the [versioned implementation](https://github.com/paradedb/paradedb/blob/v0.26.3/pg_search/src/postgres/options.rs)
and [partitioning documentation](https://github.com/paradedb/paradedb/blob/v0.26.3/docs/reference/indexing/partition-by.mdx).
The feature is beta and the result does not justify an immediate full rebuild.

## Controlled comparison

The sealed SELECT-only reader exported exactly one million projected 2003 rows,
using 20 disjoint 50,000-row windows across the edition. The 144,199,899-byte binary
export stayed on NAS. Both imported copies matched the complete ordered source
export digest. No hashes, identities, original rows or query values leave NAS.

Both tables have the same schema, columns, collation, query and four-segment
target. The only index-option difference is postcode partitioning. Visible segment
bytes were 52,382,807 for baseline and 53,742,650 for partitioned (about 3% larger).
Both use the exact deployed PostgreSQL/extension image in a separate private server.
The pilot has no published ports, its own internal network/password, and total
private quotas of 2 CPU/2 GiB across database and helper. Bitcoin remained running.

Eight distinct nonempty districts were selected at fixed spread positions in the
copied rows. ABBA/BAAB order alternates by case. Before each first query, only the
pilot was cleanly stopped, its private data files received `POSIX_FADV_DONTNEED`,
and that pilot was restarted. The advice helper receives only the private mount,
checks its run marker/version/stopped state and refuses symlinks. No live database
restart, global cache drop or SSD pool was involved. Immediate repeat queries
provide warm observations. Hardware/storage-controller caches remain uncontrolled.

## Measurements

Each row below represents 16 observations (eight cases, two repetitions).

| Layout | State | Diagnostic median | Sample p95/max | Private-container reads |
| --- | --- | ---: | ---: | ---: |
| Existing | First query | 2,019 ms | 3,440 ms | 700.5 MiB |
| Postcode partitioned | First query | 961 ms | 1,472 ms | 351.2 MiB |
| Existing | Immediate repeat | 19.6 ms | 64.0 ms | 0 MiB |
| Postcode partitioned | Immediate repeat | 12.8 ms | 13.5 ms | 0 MiB |

Observed first-query median improved 52%, sample p95/max 57%, and block reads 50%.
Every case's paired first-query median improved. PostgreSQL execution-only median
was 1,540 versus 480 ms for first queries, and 10.9 versus 5.1 ms for repeats.
All plans were actual prepared generic Top-K plans; SQL budgets stayed four seconds.

Interpretation limits:

- Diagnostic wall time includes PREPARE, EXPLAIN ANALYZE, plan-counter inspection
  and DEALLOCATE. It is not endpoint/page-delivery latency. PostgreSQL execution
  timing is reported separately above.
- With 16 observations, nearest-rank p95 is the largest observation. These are
  eight focused cases, not a national workload or a capacity estimate.
- Read bytes bracket the helper task at the private-container level and can
  include its connection/catalog/background work; they are not query-exclusive.
- Cache advice is a hint. All first queries had nonzero block reads and all
  immediate repeats had zero. This demonstrates a material cache-state difference,
  not cold platters or controlled hardware caches.
- Reported `Segment Count` remained four for both layouts. We do not infer that
  exactly three segments were pruned from that field. The measured gain is lower
  I/O and latency.
- The small subset, single client and focused postcode workload do not establish
  behaviour for full editions, other search shapes or concurrent requests.

The fixed gate required matching full-copy digests and pagination, zero errors,
generic Top-K, 25% lower first-query median and p95, 25% fewer block reads, and
nonzero-read evidence for both layouts. All passed. An independent read-only code
review found no important validity/isolation/correctness blocker and confirmed
the reporting qualifications above; it did not inspect NAS evidence directly.

## Operations and next step

Homelab preserves code, tests, fixed protocol, numeric analysis and recovery recipes
under `docker/ukinfo/index-layout/`. Eight helper tests passed with the real Linux
client in the image; 11 pure helper/analysis tests pass locally. Earlier pre-data
bootstrap/dependency failures and helper revisions remain retained. No timed
workload was retried until green and the source subset was exported only once.

All private services and their lease are stopped. Reports and copies remain at
`/home/jonw/ukinfo-backups/index-layout-2026-11-08`. The live site is unchanged at
five editions / 248,951,107 occurrences; v14 remains hidden. No SSD pool is assumed.

The next useful step is one **full-sized private edition** with this layout,
followed by real-role correctness/cursor checks and mixed concurrent workloads.
Keep the existing index and deployment until that validation passes. Do not treat
this pilot as permission to rebuild or replace all six live indexes.
