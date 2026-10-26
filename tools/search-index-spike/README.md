# Small PostgreSQL search-index benchmark

This is an operator experiment on **synthetic data only**. It does not migrate
production, import source discs, or call the public website.

Compare four rebuildable, denormalised search representations:

- `btree`: targeted composite B-tree indexes, plus native street full-text GIN.
- `gin`: native multicolumn GIN combining `btree_gin` scalar fields and street
  `tsvector` search. The primary key remains available for ordering.
- `parade`: ParadeDB `pg_search` 0.26.3, literal field indexes, street tokens,
  scalar filter pushdown and ID ordering.
- `text`: Timescale `pg_textsearch` 1.5.1 Boolean/prefix index on field-qualified
  tokens, with ID sorting. This tests filtering, not BM25 relevance ranking.

All use PostgreSQL 18, identical physical row columns and primary keys, the same
server settings, and 50-result keyset pages with one lookahead. `pg_search` and
`pg_textsearch` register the same legacy `bm25` access-method name, so they use
separate databases in the same resource-limited server. No candidate is installed
in an existing local or production database.

## Run

Requires Docker with **native ARM64** Linux containers, Python 3 and the
repository's Go toolchain. The measured package lock deliberately refuses other
architectures rather than silently comparing an emulated extension. Portability
beyond this environment remains a separate acceptance gate.

```sh
python3 tools/search-index-spike/run.py \
  --rows 1000000 --rounds 4 \
  --output /private/tmp/ukinfo-search-index-spike-results.json \
  --custom-output /private/tmp/ukinfo-search-index-spike-custom.json
```

The runner downloads official pinned release assets, verifies their SHA-256
checksums, builds a pinned-base Docker image and compiles the client from
`server/cmd/search-index-spike`. It refuses an existing experiment container or
output report. It requires 8 GiB free local disk; also check Docker's VM capacity.
Rows are bounded to 1,000–2,000,000, rounds to 1–10 and client runtime to 15 minutes.

The database has 2 CPUs, 2 GiB memory, 512 MiB shared buffers, 32 MiB work memory,
no parallel query workers and JIT disabled. Queries have a 10-second statement
budget. Builds have a separate 180-second per-statement budget. The benchmark
container has no network and publishes no ports. Trust authentication exists
only inside this isolated disposable synthetic database.

The runner removes only its own container and anonymous database volume in a
`finally` block. The small custom image and Docker build cache remain for reuse;
remove the named image explicitly if no longer needed. No global Docker prune.

## Workload and correctness

One million deterministic MD5-derived listings contain skewed names/towns,
997 synthetic surname variants, 127 given-name variants, 997 town variants,
23 counties, dummy `zz...` postcodes, two streets, business classifications and
five edition labels. Approximately 30% share a surname and 20% share a town.
This is intentionally not a population model or a real postcode validation test.

A separate Go generator computes expected IDs directly from the deterministic
fields. It does not treat another SQL engine as the oracle. Cases include common
names with town/county, prefix postcodes/names, street words, classifications,
edition filtering, empty results and a guaranteed-positive rare lookup.

Before timing, every engine must return exact expected IDs and lookahead on
three pages per case, or through exhaustion for the rare, empty and prefix-name
cases. Timed requests repeat the oracle comparison. Timed stages use one and
four client workers, rotate engine order by case/page/round, and follow up to
three pages with each engine's own last returned ID. No result cap truncates the
eligible source universe.

All clients share one server's CPU/memory budget. The four-client stage is an
interleaved mixed-engine workload, **not independent per-engine throughput**.
Latency includes pool acquisition, planning, SQL and ID retrieval over a local
Unix socket; it excludes HTTP, displayed record retrieval and UI rendering.
All engines are warmed before measurement. Normal prepared-statement reuse is
retained. Explicit custom/generic `PREPARE`/`EXPLAIN EXECUTE` diagnostics follow
measurement and are not counted in latency samples.

The report records versions, settings, table/index sizes, copy/index/vacuum build
times, per-request latencies/errors, correctness coverage and representative
plans. Use the **fixed first-page cohort** as the primary comparison; later pages
have different selectivity and exhaustion. Percentiles use nearest rank; with
48 first-page samples per engine/client level, p99 is simply the maximum sample.
Do not interpret this as a production p99 estimate.

With `--custom-output`, a second stage reuses the same fixtures with
`plan_cache_mode=force_custom_plan` for all candidates, repeating oracle checks.
It does not repeat builds, so its build metadata comes from the first run. It is
later and potentially warmer: paired custom/generic plan diagnostics support
causal interpretation better than simply subtracting the two latency tables.
Any timed query error still makes the runner exit nonzero, even when the custom
follow-up passes. Reports are retained before cleanup; a failed first run is not
silently reclassified as successful.

Summarize one or both reports:

```sh
python3 tools/search-index-spike/summarize.py \
  /private/tmp/ukinfo-search-index-spike-results.json \
  /private/tmp/ukinfo-search-index-spike-custom.json \
  --output /private/tmp/ukinfo-search-index-spike-summary.json
```

A successful small run demonstrates query semantics and exposes plan problems.
It does not establish 249-million-row capacity, cold-HDD latency, full record I/O,
lexical ordering, punctuation/Unicode handling, real cross-edition deduplication,
nearby searches, permissions isolation or desktop distribution support.
