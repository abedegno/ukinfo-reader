# Search application (development preview)

A shared Go service serves the web UI and searches PostgreSQL 18. Electron launches
that same service with an app-managed local PostgreSQL process. The original Python
reader remains the verified decoder and importer: searches do not run Python.

This is the first working vertical slice, not a production hosting capacity claim.
Exact surname, first name, business name and postcode searches, keyset pagination,
source details and lossless **current-page** JSONL export are implemented. Fuzzy search,
sector search, cross-edition matching, background bulk exports, GUI import management,
resumable ingestion, automatic database upgrades and signed installers are not yet implemented.

## Requirements and edition scope

- Go 1.25+ (tested with 1.26.4), PostgreSQL **18**, Python 3.10+ for importing discs.
- Node 22.12+ and npm for desktop development/browser tests, not hosted serving.
- All four [verified release profiles](editions.md) can be imported. Each import is
  searched separately. Modern entries retain their electoral/telephone/business types.
- Business searching covers the validated **2005 V10.5 SE** business stream, including
  updates/deletions already applied by the reader. The supplied **2004 SE** disc has no
  business listings. Early-edition business-related source fields remain visible as
  original fields; they have not been reclassified as modern business listings.
- Source warnings, nulls, incomplete postcode precision, raw references and source IDs
  survive ingestion. Raw postcodes are never substituted for the displayed source postcode.
- `--limit N` imports the **first N** decoded entries into a separately named sample.
  It does not select a representative random sample. In 2005, residential records occur
  before businesses; a small first-N sample will usually contain no businesses.

## Run with Docker Compose

From the repository root (Docker is needed for this example):

```sh
python -m pip install -e .
# Use URL-safe random characters; this password is interpolated into a PostgreSQL URL.
export UKINFO_DB_PASSWORD="$(python -c 'import secrets; print(secrets.token_hex(32))')"
docker compose -f deploy/compose.yaml up -d db --build
docker compose -f deploy/compose.yaml run --rm api migrate

# Optional invented records to try the UI without obtaining any original media:
set -o pipefail
python tools/search_demo.py | docker compose -f deploy/compose.yaml run --rm -T api import

docker compose -f deploy/compose.yaml up -d api
# Open http://localhost:8080
```

To import an extracted, supported disc, leave Python on the host:

```sh
set -o pipefail
python -m ukinfo_reader.search_export --data private/pcf175/reader --limit 10000 \
  | docker compose -f deploy/compose.yaml run --rm -T api import
```

The UI labels this as a partial sample. Omit `--limit` for a complete edition only after
allowing enough disk space for expanded JSON records, indexes, WAL and a transaction.
The source files' compressed size is not a useful database capacity estimate. This schema
retains every original field in JSONB alongside indexed search keys; its full-edition
storage footprint has **not** been measured. The earlier spike used a narrower projection.

The importer hashes all required source files before emitting a manifest. Go streams
batches into one transaction and checks the terminal count and EOF before commit. A
failed/truncated producer, duplicate source identity, duplicate dataset key or count
mismatch rolls back the whole dataset. A retry starts again; there is no resume or replace.
Existing imported datasets remain available during another import. Run one ingestion
job at a time. `ANALYZE entries` updates planner statistics after ingestion.

The interchange manifest is produced by our trusted local decoder; accepting a manifest
from an arbitrary third party does not independently prove its asserted source hashes.
Do not import untrusted SQL dumps. Source bytes must stay unchanged while decoding.

## Run without Docker

```sh
cd server
go build -o ../build/desktop/ukinfo-server ./cmd/ukinfo-server
cd ..
export DATABASE_URL='postgres://USER:PASSWORD@127.0.0.1:5432/ukinfo?sslmode=disable'
build/desktop/ukinfo-server migrate
python tools/search_demo.py | build/desktop/ukinfo-server import
build/desktop/ukinfo-server serve --listen 127.0.0.1:8080
```

Use TLS for non-loopback database connections. Do not expose the PostgreSQL port to the
internet. Migrations/imports need an owner connection; hosting should give the serving
process a separate role with SELECT only on `datasets`, `entries`, and `catalog_schema`:

```sql
CREATE ROLE ukinfo_search LOGIN; -- set its password separately, outside shell history
GRANT CONNECT ON DATABASE ukinfo TO ukinfo_search;
GRANT USAGE ON SCHEMA public TO ukinfo_search;
GRANT SELECT ON datasets, entries, catalog_schema TO ukinfo_search;
ALTER ROLE ukinfo_search SET default_transaction_read_only = on;
```

Compose is a local development baseline and uses the owner connection. For public hosting,
use a read-only connection for serving, a TLS reverse proxy preserving `Host`, edge rate
limits, backups and monitoring. Set `--public-origin https://your-domain` (or
`UKINFO_PUBLIC_ORIGIN` in Compose). The app rejects unexpected Host/Origin headers.
There is no built-in user authentication or per-IP quota. The HTTP service admits up to
32 concurrent API requests, uses at most eight database connections, times queries out
after two seconds and returns at most 100 entries. These bounds protect one process;
they are not a substitute for admission control at your public edge. Additional API
replicas each add their own pool: size PostgreSQL and any connection proxy accordingly.
No search terms or directory records are written to application request logs.

## Desktop and portable data

Desktop local mode requires native PostgreSQL **18** binaries: `bin/initdb`, `bin/postgres`,
`lib`, `share` and the distribution's required license notices. The app never downloads
executables at runtime. Use a trusted current patch release and preserve all notices.
The Go supervisor initializes with SCRAM authentication, a random persistent password,
loopback-only networking and private directories. A file lock prevents concurrent owners.
It shuts down only its own child; it never deletes a cluster or kills a PID from a file.

```sh
python tools/stage_desktop.py --postgres /path/to/native/postgresql18
cd desktop
npm ci
npm start
npm run pack
```

`npm run pack` produces an unsigned native development directory under `dist/desktop`.
It includes Electron, the Go binary and the staged PostgreSQL runtime; **no catalogue**.
Only native macOS/Linux bundles are supported at present. Windows local process supervision,
signing/notarization and release installer testing remain future work. PostgreSQL/runtime
redistribution notices must accompany any bundle you distribute; the project's MIT license
does not relicense those dependencies or the original data.

By default data lives in Electron's OS user-data directory. To make an unpacked build
portable, choose an explicit directory on the same machine/OS:

```sh
export UKINFO_DATA_DIR=/absolute/path/to/portable-archive
```

Close the app before importing into its local catalogue:

```sh
unset DATABASE_URL
set -o pipefail
python tools/search_demo.py | build/desktop/ukinfo-server import \
  --data-dir "$UKINFO_DATA_DIR/catalogue" --pg-bin /path/to/native/postgresql18/bin
cd desktop && npm start
```

Use `python -m ukinfo_reader.search_export --data ...` in place of the demo for real data.
This first UI has no import wizard: provisioning is via these commands. Imports/searches
work offline after the Python package, Go binary and PostgreSQL runtime are installed.
Developers can set `UKINFO_SERVER`, `UKINFO_PG_BIN` or `UKINFO_DESKTOP_DATABASE_URL` to test
alternate local runtimes. The last option uses an existing PostgreSQL catalogue; it does
not turn the app into a client for an arbitrary remote web page.

The local `pg18` directory is a PostgreSQL physical cluster, not a universal data pack.
For another OS/architecture or a major PostgreSQL upgrade, re-import source data or use a
trusted `pg_dump`/`pg_restore` logical transfer. Patch upgrades stay within major 18.
A crash may leave a postmaster process or `postmaster.pid`: startup fails safely rather
than deleting it. Inspect `postgres.log`, verify the cluster's actual process is stopped,
and follow PostgreSQL recovery procedures. Preserve the password file with its cluster.

## API contract

- `GET /api/health`: database readiness.
- `GET /api/datasets`: imported keys, source manifests, row counts, timestamps.
- `GET /api/search?dataset=KEY&kind=person&surname=Smith&first_name=John&postcode=AB101AA`.
- Business: `kind=business&organisation=Exact%20Name` or postcode.
- Names match exactly after Unicode lowercase conversion and whitespace collapse;
  postcode keys remove whitespace and uppercase. Display/source fields remain untouched.
- People require surname or postcode; businesses require organisation or postcode.
  First name is an additional exact filter. Names such as Li and Ng need no minimum length.
- `limit=1..100` (default 50). Reply: `{items:[{id,source_id,record}],next_cursor:"..."}`.
  Repeat the original filters with `cursor=...`. Cursors are opaque and bound to the
  normalized filters and dataset; there is no exact-total query or offset scan.
- Ordering is surname/first-name/import ordinal for people, organisation/import ordinal
  for businesses. Entries are never merged into inferred people or households.
- Import ordinals are stable only within a dataset; source identity is edition plus
  source ID (SE4 additionally qualifies entry IDs with the source segment).

## Verification

```sh
python -m unittest discover -s tests -v
cd server
go test -race ./...
UKINFO_TEST_DATABASE_URL='postgres://...' go test -race ./internal/catalog -v
UKINFO_TEST_PG_BIN=/path/to/native/pg18/bin go test -race ./internal/desktop -v
cd ../desktop
npm ci
npm test
npx playwright install chromium
# Requires the running Go service and imported tools/search_demo.py fixture:
UKINFO_TEST_URL=http://127.0.0.1:8080 npm run test:ui
```

Integration tests create and remove a uniquely named schema in the configured test database.
Desktop lifecycle tests use temporary clusters. Browser tests use invented records and
write ignored screenshots. [Storage spike report](research/search-storage-spike-2026-10-17.html)
records the earlier 2M-row experiment, including its limits and raw aggregate measurements.
It does not benchmark this wider production-preservation schema.

## Local verification, 17 October 2026

- All 41 Python tests, including local source regression tests, passed.
- Go race-enabled PostgreSQL integration tests and `go vet` passed.
- Exclusive ownership, restart persistence and same-platform data-directory relocation
  passed with PostgreSQL 18.6 on native macOS ARM64; Linux lifecycle tests passed in
  the PostgreSQL container (relocation coverage is also in CI).
- Real Go/PostgreSQL browser tests passed for pagination, source details, page export,
  business search and a 390px mobile viewport.
- The native macOS Electron development app **and packaged bundle** passed fresh-cluster
  import, search and owned PostgreSQL shutdown checks using invented entries.
- Hash-verified first-N source imports passed: 10,000 entries from 2005, and 1,000 each
  from 1998, 1999 and 2004. These checks do not establish whole-edition ingestion capacity.
- Runtime and bundle binaries remain local/ignored; there is no signed release download.

For the native end-to-end smoke test (fresh temporary cluster and synthetic data):

```sh
cd desktop
UKINFO_PG_BIN=/path/to/native/pg18/bin npm run test:electron
```

`UKINFO_TEST_APP` can point to a packaged Electron executable to test bundled runtime
resolution without `UKINFO_SERVER`/`UKINFO_PG_BIN` overrides inside the app. The test runner
still uses its supplied PostgreSQL binaries to prepare the temporary catalogue.

## Design references

The shared hosted/local model follows [Muesli](https://github.com/abedegno/muesli), with
independently written implementation code. Runtime behavior follows PostgreSQL's
[initdb](https://www.postgresql.org/docs/18/app-initdb.html) and
[logical backup](https://www.postgresql.org/docs/18/backup-dump.html) documentation.
Electron's [security recommendations](https://www.electronjs.org/docs/latest/tutorial/security)
inform the sandboxed renderer, disabled Node integration and restricted navigation.
