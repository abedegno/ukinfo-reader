# Optional ParadeDB search preview

The hosted Go service can use a **separate, derived ParadeDB database** to
accelerate address-filter searches, including names, postcodes and businesses
combined with those filters. Enable it explicitly with
`serve --search-backend paradedb` and a SELECT-only `PARADE_DATABASE_URL`.
The default remains `native`; desktop continues using its existing PostgreSQL
package. Original records, edition visibility and archival identities remain in
the source catalogue. The public NAS site enabled this address-only policy on 26 October 2026;
see the [control and rollout report](research/parade-rollout-2026-10-26.md).

The [full 2003 benchmark](research/real-search-spike-2026-10-26.md) explains why
this was worth evaluating. The
[five-edition serving trial](research/parade-serving-trial-2026-10-26.md) measures
this projection's compatibility, 48.41-GiB database, concurrency and offline
rollback. Warmed load improved, but broad postcode regressions prevent a blanket
switch. The [hybrid follow-up](research/parade-hybrid-trial-2026-10-26.md) validates
address-only routing on the full corpus and on measured, isolated cold file
caches. It reduces failures but still exposes slow cold paths. The serving
projection additionally preserves compact dictionary IDs for
cursor-compatible ordering. Do not point the API at the earlier benchmark's
`native`/`parade` tables or use its figures as measurements of this schema.

## Coverage and selection

The hybrid selector uses ParadeDB only when a street, locality, town or county
filter is present. Name-only, postcode-only, names combined only with postcodes,
and business-classification-only searches retain native execution. This avoids
the broad-postcode and simple-name regressions found in the full serving trial.
Selection happens before contacting the target; it is not a retry after failure.

Accelerated searches handle people and businesses, exact/trailing-prefix fields, given
names, postcode district/sector precision, primary/secondary street/locality,
town/county, SIC/industry and alphabetical sorting. House, building, flat and
middle-initial refinements use displayed original records, continuing through
complete candidate batches until the page plus lookahead is known. No sparse
candidate-count cap truncates results.

Grouped/deduplicated searches, nearby/nearest searches and edition ordering use
the native backend. They remain available; this version does not accelerate them.
Group membership, record detail, maps, exports and archival lookup retain their
existing permission and original-record checks. Exports use the same search
selector and pagination as the interactive API.

Every requested edition must have a **complete, current** derived generation.
Otherwise the entire request uses native search, including `dataset=all`—never
a partial union of indexed editions. Full editions hide their samples in the all
scope exactly as before; explicit sample searches remain possible. The source
serving role controls the edition inventory; target-only or obsolete editions
cannot appear in results.

Generation identity includes source dataset ID/key, manifest, exact entry count,
import timestamp and projection version. A reimport with the same manifest/count
still invalidates the old copy. Build publication requires the complete ID range,
index creation and final validation. The builder compares every source name,
first name, postcode and listing kind with the original record, and copies the
source dictionary IDs for order preservation.

An absent/incomplete/stale generation falls back **before** candidate execution.
An unavailable target or failure after selecting ParadeDB returns an ordinary
503; it does not run a second expensive search or return a partial result. The
existing request deadlines and quotas remain intact. Health continues reporting
the source service independently; target failures appear in search errors and
phase metrics.

Partial-postcode searches additionally use a literal-token prefix prefilter via
[ParadeDB's regex query API](https://www.paradedb.com/blog/v2api). The prefix is
escaped and bound as a parameter; user regex syntax is never accepted. The
original scalar range and district/sector precision predicates remain in place,
so the prefilter changes candidate work without changing search semantics.

## Build privately

The preview requires PostgreSQL 18 with **pg_search 0.26.3**. Its explicit builder
refuses another version and refuses a target resolving to the source database.
Use separate instances for resource isolation. No extension, schema change or
write is required on the source catalogue. Supply its existing SELECT-only DSN.

Build both Go commands, or use [the application image](../deploy/Dockerfile):

```sh
cd server
go build -o ../build/ukinfo-server ./cmd/ukinfo-server
go build -o ../build/ukinfo-parade-index ./cmd/ukinfo-parade-index
```

Supply `SOURCE_DATABASE_URL` (source SELECT-only) and `TARGET_DATABASE_URL`
(derived owner) privately to the index job. Provision an independent derived
reader first; [the policy](../deploy/production/parade-reader.sql) takes its
password from environment, rejects ownership/memberships and inherited write
privileges, and grants CONNECT without TEMP/CREATE. Use random hex passwords of
at least 32 characters. The builder grants schema USAGE and SELECT on published
search tables and metadata; no owner password reaches the API.

```text
ukinfo-parade-index --dataset all --reader-role ukinfo_parade_search --work-dir /work
```

`all` means the source role's visible editions using the API's sample rule. Use
an explicit edition key to prepare a pilot or an otherwise hidden sample.
Committed 32,768-row checkpoints resume by rerunning the same command. It takes
the shared source maintenance lock and a separate target build lock. Corrupt
blocks, sequence gaps, field differences and changed generations stop publication.
A failed new generation remains unavailable to serving requests.

The job defaults to a 50-GiB free-space floor before each batch/build phase.
`--work-dir` must reside on the **actual derived-volume filesystem**; also monitor
Docker's filesystem and allow for WAL, temporary files and backups. The optional
floor override accepts 8–1024 GiB; lowering it does not make a large edition fit.
The job has a 24-hour total budget and one-hour SQL statement bounds. Resource
limits and checkpointing bound the job, but these are not build-time guarantees.

Completed generations are retained. The tool does not automatically drop old
tables, delete source data or prune volumes. Plan generation cleanup separately
after validating readers and backups.

## Configuration as code

The [optional Compose overlay](../deploy/compose.parade-preview.yaml) adds a pinned,
private 2-CPU/6-GiB derived database and an explicit index job, with no published
database port. Loading it **does not switch the API**. Keep actual NAS stack
configuration in `homelab`; these files are reviewable upstream templates.

Add independent `UKINFO_PARADE_OWNER_PASSWORD`, `UKINFO_PARADE_SEARCH_PASSWORD` and
an absolute `UKINFO_PARADE_WORK_DIR` to a private environment file. The work
location must be on the derived-volume filesystem and readable by the configured
indexer UID/GID. Existing production settings still apply. From the repository:

```sh
preview() {
  docker compose --env-file private/production.env -p ukinfo-production \
    -f deploy/compose.production.yaml -f deploy/compose.parade-preview.yaml \
    --profile parade-preview "$@"
}
preview config --quiet
preview build api
preview up -d parade-db
preview exec -T parade-db sh -c 'PGPASSWORD="$POSTGRES_PASSWORD" exec psql -X -v ON_ERROR_STOP=1 -U postgres -d ukinfo_search -f /checks/parade-reader.sql'
preview run --rm parade-indexer
```

The optional database intentionally skips the vendor's optional pg_cron bootstrap;
the explicit builder creates pg_search and its dependencies. Do not use a public
PostgreSQL port or an interactive TTY for automated diagnostics. The reader policy
is also explicit and applies to existing volumes. Reapplying it revokes old direct
table grants; rerun the builder to restore SELECT grants before enabling readers.

For a **private trial**, add `PARADE_DATABASE_URL` using `ukinfo_parade_search` to
the API's private environment and append `--search-backend paradedb` to its existing
serve arguments, preserving origin, quotas and trusted-proxy settings. Keep the
owner DSN solely in the index job. Test privately before applying a public stack
change; this overlay does not perform that change automatically.

Rollback uses `--search-backend native` or omission of the flag, followed by an API
restart. Existing cursors retain their ordering and can cross backends for an
unchanged source catalogue/query. Keep native indexes throughout the trial. The
preview database can then be stopped independently; retaining/removing its volume
is an explicit operator decision. Do not use global Docker pruning.

## Verification and operating limits

[CI](../.github/workflows/search-app.yml) has a separate pinned-extension job;
normal native/desktop testing remains independent. To run the extension checks
locally, set `UKINFO_TEST_DATABASE_URL` to a disposable source and
`UKINFO_PARADE_TEST_DATABASE_URL` to a separate disposable target with no existing
`ukinfo_search` schema, then run:

```sh
cd server
go test -race -count=1 -run '^TestParadeIntegration$' ./internal/catalog
```

Invented fixtures cover five edition labels plus a hidden sample, a 260-row
sparse-refinement edition and postcode district/sector boundaries. Tests walk
complete result sets, compare original records and order with native, cross
backends mid-pagination, check primary and
continuation Top-K plans, interrupt/resume publication, invalidate import
metadata, reject partial all-edition readiness, exercise grouped/native fallback,
and verify permission faults fail closed. Write denial is checked with read-only
defaults disabled. These are **synthetic all-edition compatibility checks**, not a
new benchmark of the complete real archive or historical format decoders.

Fixed observability labels `paradedb`/`paradedb_fallback` identify selection;
`parade_inventory`/`parade_candidates` measure target phases. Existing original
record-load signals remain. Query text, names and postcode values are not labels.

The real five-edition trials now cover first/deeper pages, native fallbacks,
field/order checks, fixed-cohort per-engine warm load and rollback with the target
offline. The hybrid policy has also been tested over the complete corpus and
measured cold file caches. The [equal-budget control and rollout](research/parade-rollout-2026-10-26.md)
confirm the selected policy with unchanged production deadlines. Remaining native
grouped/ordered/postcode tails are documented there. Backup/restore and extension-
upgrade recovery exercises were explicitly skipped at the operator's request;
the pinned derived database is rebuildable from original media. Existing source
backups remain authoritative; the observed full build took 5 hours 17 minutes.
Keep native indexes and use the current patched image for native rollback. Desktop packaging and
extension upgrades require separate validation.

The [serving-schema trial tools](../tools/parade-preview/README.md) provide real
coverage checks, value-free metadata/plans, private cursor exchange and matched
fixed-cohort load runs. These differ from the earlier simplified index spike.
