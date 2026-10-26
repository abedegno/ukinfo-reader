# Full-edition native PostgreSQL / ParadeDB experiment

Private operator tooling. This is **not a production migration or serving switch**.
It has no public endpoint and never requires replaying saved searches to an
Internet URL. Keep original records, sampled fields, queries, IDs, hashes, cursors,
credentials and reference files inside the private NAS directory.

The experiment is restricted to the complete `se2003-dpcs0104` edition with
59,886,841 entries. It compares two rebuilt search tables on one isolated
PostgreSQL 18 / ParadeDB 0.26.3 server. Both use identical denormalised rows,
source-ID and lexical pagination, and the original production record store.

## Environment

[Compose configuration](compose.yaml) pins the official amd64 image manifest.
The database has the production database's 2 CPU / 6 GiB limits, one-GiB shared
buffers, 32-MiB work memory, no parallel query/maintenance workers and JIT off.
The client has 1 CPU / 512 MiB and an eight-connection pool. Preparation alone
allows at most 16 GiB of temporary sort files per backend, with a 50-GiB free-space
floor; measured queries retain the server's 4-GiB cap and original deadlines. Both connect only to
the existing internal `ukinfo_database` network; neither publishes ports.

Create an owner-only NAS directory (`SPIKE_ROOT`, mode 0700, uid/gid matching the
client) on the same data filesystem as the named experiment volume. The builder
checks this directory for at least 50 GiB headroom before each batch/build phase;
also monitor the actual Docker volume's filesystem. Recheck capacity before
starting. Never bind the production database directory into the experiment.

Supply random `SPIKE_PASSWORD`, the SELECT-only `SOURCE_DATABASE_URL`, and the
experiment owner's `TARGET_DATABASE_URL` through private local configuration.
Do not put these in Git, terminal transcripts or the public report. The source
connection enforces read-only transactions. Its shared advisory lock prevents a
concurrent source import/index build while a full projection is prepared.

The official image's optional init scripts are hidden with an empty tmpfs because
its normal bootstrap also requires pg_cron. The builder explicitly creates
pg_search and required dependencies. `init: true`, noninteractive diagnostics,
`psql -w -P pager=off` and socket authentication avoid interactive pager/password
processes becoming orphaned under a PostgreSQL PID 1. Do not use interactive
TTY exec sessions for automated diagnostics.

## Prepare and resume

Build the client using `server` as the Docker context and this directory's
Dockerfile, or compile `server/cmd/ukinfo-search-engine-spike` for Linux/amd64.

Run `--stage prepare` against the separate owner database. It:

1. Reads original compressed blocks sequentially, verifies envelopes/counts and
   uses the catalogue's production normalisation and displayed-address projection.
2. Materialises names, full first names, given names, displayed postcodes and
   precision, both street/locality components, town/county and SIC/industry.
   Original records remain in the source store; hidden address/postcode fields
   do not restore suppressed data.
3. Commits batches of 32,768 rows and the last-ID checkpoint atomically. One row
   in every 1,024 is independently compared with the existing integer/research/
   address indexes. Count/sequence/generation errors stop the build.
4. Builds targeted B-trees, a second identical heap and the ParadeDB literal-field
   covering index. Each DDL phase has a transactional completion marker.
5. Vacuums/analyzes both tables, checks exact counts/min/max IDs and publishes
   `finished_at`. Incomplete builds cannot be measured.

Resume the same `prepare` command after an interruption. It checks the complete
edition count and source-manifest fingerprint before using a committed checkpoint.
Never overwrite a build from a changed source generation. The builder's source
connection has no write privileges. Its checkpointed DDL and copying occur only
inside the isolated target database.

Before measurement, set a **separate random password** on `spike_search`, revoke
PUBLIC database TEMP/schema CREATE, set that role's default transaction read-only,
and verify it has only SELECT on `native`, `parade` and build metadata. Use that
role's DSN for every measured target connection. Do not benchmark as the owner.

## Cases and measurement

Stage private seed cases as a JSON array of `{"query":"..."}` values. They use
the existing API's query grammar. Groups, nearby, house/building/flat/middle-initial
refinements and non-name custom ordering are deliberately rejected. First-name
`given`, exact/prefix fields, postcode precision, both street/locality fields,
business classifications and lexical sorting are supported.

```text
--stage cases --cases /work/seeds.json --output /work/cases.json
```

This adds guaranteed-positive cases sampled at spread source IDs, including the
business tail of the edition. Negative cases and seeds remain private. No-address
cases receive explicit lexical sorting so their source-reference order matches
rather than silently comparing source dictionary-ID order with row-ID order.

Measure the same fixed workload at 1 and 4 clients under `auto` and
`force_custom_plan`, in separate processes. Retain four reports. Initial capture:

```text
--stage measure --cases /work/cases.json --output /work/auto-c1.json
--clients 1 --rounds 2 --pages 3 --plan-mode auto --reference
```

The client first obtains independent reference pages from the existing source
search implementation. A source failure is recorded separately and never treated
as proof of an empty result. Source references cache only IDs and digests, bound
to the exact workload and source generation, at `UKINFO_SPIKE_REFERENCE_FILE`.
Subsequent stages reuse that private file with `--reference` omitted.

Each measured request has a 4-second target SQL timeout and a 5-second total
search-plus-record deadline. It retrieves up to 50 rows plus a lookahead, then
loads and decodes original records through the production catalogue reader.
Candidates alternate execution order, use independent keyset positions and check
exact rows, lookahead and original-record digests. Initial and later pages are
reported separately. Neither side truncates a broad candidate set to obtain a
successful response. Measurement errors produce a nonzero exit **after** saving
the report. The whole measurement process has a 30-minute budget.

Both engines share a server in this paired experiment: concurrent results are
mixed-engine contention, not independent engine throughput. No cache flushing is
performed. Build/capture/measurement stages must not overlap; no sizing scans or
other operator maintenance should run during timing.

```text
--stage plans --cases /work/cases.json --output /work/plans.json
```

Plans use explicit custom/generic PREPARE/EXPLAIN EXECUTE after measurement.
`--stage plans` executes the queries with ANALYZE/BUFFERS. `--stage explain`
only inspects plans; its zero work counters mean **not measured**, not zero I/O. Only
allowlisted node names, buffer/temp/heap counters and timing leave the function;
SQL, predicate values, Tantivy queries and cursors are excluded from reports.

Alphabetical keyset pagination needs special care: native B-trees use row
comparison, while the search adapter expands continuation into equivalent scalar
OR clauses. An initial page uses only the positive ID bound. The original row
comparison caused ParadeDB to scan and sort instead of using Top-K, timing out
every alphabetical case in the preliminary full-data run. Preserve that failed
report separately when repeating the corrected workload. Synthetic fixtures
exercise both adapters, duplicate names and subsequent lexical pages.

`python3 tools/real-search-spike/summarize.py PRIVATE_DIRECTORY --output REPORT.json`
requires all four completed, two-round, matching-cohort reports, build/environment
metadata, execution plans, and the retained preliminary report/diagnostics. It
publishes aggregate timings and fixed case numbers without search values.

## Cleanup

Stop test clients and the test database when timing ends. Keep or remove the
labelled derived volume explicitly according to the report's retention decision;
never use a global Docker prune or delete the source database, rollback edition,
original media or ordinary NAS backups. Keep private reference files mode 0600.
The experiment grants no public serving-role privileges and changes no route,
production image, timeout or quota.
