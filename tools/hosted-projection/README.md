# Hosted projection experiment

This is an operator-only benchmark, not a catalogue migration or supported serving format. It combines search keys, reverse address IDs, grouping keys and business classifications for **one complete edition**. Original compressed record blocks remain the record store.

Run on a private PostgreSQL 18 catalogue, as its owner:

```sh
psql "$DATABASE_URL" -X -v dataset=EDITION_KEY -f tools/hosted-projection/build.sql
```

The script refuses to overwrite `ukinfo_projection_spike`, checks complete index metadata, commits approximately one million rows at a time, verifies the final count and builds six indexes. It grants no permissions to the public serving role. Failed builds remain private for inspection. Row loading is not resumable; inspect before explicitly dropping the experiment schema to start again. If all row batches completed and only index construction failed, `finish.sql` can resume the private index stage. It checks row-count metadata and validates index definitions before marking the experiment complete.

Allow tens of gigabytes of extra disk space for a full edition, plus WAL and temporary files. Measure the actual table, indexes, WAL growth and build duration before extrapolating to all editions. Each statement has a 20-minute budget and temporary files are limited to 4 GiB per process. Automatic vacuum is disabled on the experiment table during loading/index construction, then restored after explicit vacuum and analysis. Index-stage DDL may wait up to two minutes for a lock on this private table; serving budgets are unchanged. PostgreSQL server CPU/memory limits still apply. Run after other import/backup work has completed, and keep the public API unchanged during comparisons.

A useful comparison must check exact IDs and ordering, seek pagination, address prefixes and classifications, and include original record retrieval. Repeated small workloads measure cache performance; they do not establish cold disk latency or internet capacity. Do not publish source records, query values, private reference hashes or connection credentials in reports.

## Compare queries and record reads

Build `server/cmd/ukinfo-benchmark-projection` for the database host. Give it a private JSON array of objects containing `query` strings in the existing search URL format. The edition is explicitly overridden by `--dataset`.

```sh
cd server
go build -o ukinfo-benchmark-projection ./cmd/ukinfo-benchmark-projection
./ukinfo-benchmark-projection --dataset=EDITION_KEY --cases=/private/cases.json \
  --output=/private/projection-results.json --rounds=2 --pages=3 --concurrency=1
```

Repeat with concurrency 4 and 8 using the same corpus. Each case is assigned once to a worker, with alternating baseline/projection execution order across rounds and pages. A 10-minute whole-run budget applies by default; individual SQL reads keep a 4-second deadline. Failures and incomplete runs produce nonzero exit codes. Reports contain timings and fixed outcomes, without query values, record IDs, raw records or hashes.

The measured SQL runs used catalogue-owner authentication with read-only sessions. They exclude the serving role’s row-security planning overhead; adoption must be retested with serving permissions.

This spike compares **candidate queries ordered by source ID**, with exact first-name or prefix filters, postcodes, address fields and business classifications. It validates up to three successive 50-candidate pages by default, including a lookahead row and grouping-key identity, then compares decoded source records using the production block reader. Grouped/lexically sorted output, nearby searches and final record refinements (including given-name boundaries) are rejected. It does not replace the independent HTTP benchmark or establish equivalence for every API feature.

The baseline reads the existing compact source, joining research keys only for classification filters and reverse-address blocks only for address filters. The candidate reads the combined table. Both use the same terms/address dictionaries and compressed record store. SQL latency and record retrieval latency are reported separately. Grouping keys for the bounded result/lookahead set are validated after the timed search/record path; their validation-only latency is reported separately. Ordinary candidate SQL selects IDs, allowing covering indexes without forcing grouping-key heap reads before LIMIT. This isolates the storage projection decision; it is not a claim that the baseline SQL reproduces every deployed API access path.

## Visibility and allocation diagnostics

`visibility.sql` samples only on-disk page headers for a complete immutable edition, returning aggregate flags. It requires the operator's server-file read permission and supports PostgreSQL 18 with 8-KiB pages, 1-GiB segments and little-endian layout version 4. Interpret flags only if `unsupported_header_layouts=0`.

```sh
psql "$DATABASE_URL" -X -v dataset=EDITION_KEY -f tools/hosted-projection/visibility.sql
```

Uninitialized allocation holes can lower the table's all-visible percentage even when sampled tuple pages are marked visible. Disk headers can lag buffers; this is not a full visibility-map audit or a substitute for actual query heap-fetch counts. It does not rewrite tables, flush caches or install extensions.

`tune-classifications.sql` reproduces the one-time covering-index refinement on the initial measured prototype. New builds already use those covering definitions in `finish.sql`. The refinement is private-table DDL, not a production migration.

## Remove the disposable projection

After saving aggregate evidence, use the guarded cleanup to avoid including the experiment in full database backups:

```sh
psql "$DATABASE_URL" -X -v dataset=EDITION_KEY -f tools/hosted-projection/cleanup.sql
```

Cleanup requires matching complete edition metadata and an operator-owned experiment. It uses `RESTRICT`, not cascading deletion: external dependencies or additional schema objects make the transaction fail. The source catalogue is retained. The measured 22 October experiment was removed after its reports were collected.
