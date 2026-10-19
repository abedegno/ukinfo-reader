# Search application (development preview)

A shared Go service serves the web UI and searches PostgreSQL 18. Electron launches
that same service with an app-managed local PostgreSQL process. The original Python
reader remains the verified decoder and importer: searches do not run Python.

Development preview. The [public archive](https://ukinfo.6153112.xyz) serves the four
complete editions. Hosted capacity depends on storage and workload; broad national
grouping/sorting and cold HDD reads can exceed query deadlines.
Exact and prefix name searches, postcode district/sector searches, optional nearby
and [street/locality/town/county searches](address-search.md), all-edition searches,
keyset pagination, readable record details, page-local identical-listing grouping,
source comparison, copyable citations and current-page JSONL/CSV exports are implemented.
[Direct record links and exact-address exploration](address-exploration.md) add co-listed
people/businesses, edition-scope comparison, saved-link opening and page exports.
[Search refinements](search-refinements.md) add house/building/flat fields, independent
given-name matching, middle initials, removable filters and zero-result actions.
[Local research lists](research-lists.md), snapshot comparison and [optional postcode
maps](maps.md) are available in the web interface and desktop app. The desktop opens a
guided importer for an empty catalogue; [native package/release CI](desktop-releases.md)
tests the wizard, imports and list persistence across restarts.
The optional [research index](research-index.md) adds search-wide identical-listing
grouping and name/publication/distance ordering. This groups listings, not identities.
Contains/fuzzy matching, background bulk exports, resumable ingestion and automatic
database upgrades are not implemented. Signed/notarized macOS candidates are prepared
through the [release workflow](desktop-releases.md#signing-and-platform-limits);
all desktop releases remain drafts.

## Requirements and edition scope

- Go 1.25+ (tested with 1.26.9), PostgreSQL **18**, Python 3.10+ for importing discs.
- Node 22.12+ and npm for desktop development/browser tests, not hosted serving.
- All four [verified release profiles](editions.md) can be imported. Each import is
  searchable individually or together. All **189,064,266** occurrences in the complete supported sources
  have passed PostgreSQL record/projection verification. Modern entries retain their
  electoral/telephone/business types.
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

The UI labels this as a partial sample. Omit `--limit` for a complete edition.
[Schema 2](formats/catalogue.md) stores integer search keys and lossless Zstandard blocks of 128 source records.
The complete 2005 edition occupies **11.79 GiB** in PostgreSQL (51,350,897 entries;
41.36 GB of original record JSON before compression). Allow additional room for WAL,
index construction, temporary identity checks and backups. Source media size does not
predict database size. See [the storage measurements](research/compact-catalogue.md).

For a native server, the guarded supervisor avoids a temporary JSONL export and cancels
its children if local free space falls below **5 GiB**:

```sh
python tools/ingest_catalogue.py --data private/pcf175/reader \
  --server build/desktop/ukinfo-server
```

It inherits `DATABASE_URL`. `--watch-filesystem PATH` selects the local filesystem to
monitor; it cannot monitor a remote database volume or Docker VM's internal free space.
Compose pipelines above do not include this guard. Monitor the database host separately.

The decoder hashes required source files before and after decoding. Protocol 2 also
checks a terminal SHA-256 of ordered source IDs and exact record JSON bytes. Go loads
unattached partitions in one transaction, validates every source identity, builds indexes,
then publishes the edition. A failed/truncated producer, duplicate identity or dataset,
checksum/count mismatch or canceled transaction rolls back the dataset. A retry starts
again; there is no resume or replace. Existing datasets remain readable during ingestion.
Only one ingestion job runs per database. Cancellation racing a successful commit can
leave a completed edition; refresh the catalogue to check the actual result.

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
process a separate role with SELECT only on the catalogue and optional geography tables:

```sql
CREATE ROLE ukinfo_search LOGIN; -- set its password separately, outside shell history
GRANT CONNECT ON DATABASE ukinfo TO ukinfo_search;
GRANT USAGE ON SCHEMA public TO ukinfo_search;
GRANT SELECT ON datasets, entries, terms, record_blocks, catalog_schema, geography_releases, geography_active, postcode_locations, address_locations, address_members, address_indexes, research_keys, research_indexes TO ukinfo_search;
ALTER ROLE ukinfo_search SET default_transaction_read_only = on;
```

Compose is a local development baseline and uses the owner connection. For public hosting,
use a read-only connection for serving, a TLS reverse proxy preserving `Host`, edge rate
limits, backups and monitoring. Set `--public-origin https://your-domain` (or
`UKINFO_PUBLIC_ORIGIN` in Compose). The app rejects unexpected Host/Origin headers.
There is no built-in user authentication or per-IP quota. The HTTP service admits up to
32 concurrent API requests, uses at most eight database connections, times queries out
after two seconds for ordinary searches and five seconds for nearby requests, and returns
at most 100 entries. At most two uncached spatial lookups run per process; their SQL
deadline is four seconds. Main entry SQL retains its two-second server deadline.
These bounds protect one process;
they are not a substitute for admission control at your public edge. Additional API
replicas each add their own pool: size PostgreSQL and any connection proxy accordingly.
No search terms or directory records are written to application request logs.

See the [dated hosting shortlist and sizing gate](hosting.md) before selecting a server.

## Desktop and portable data

Desktop local mode requires native PostgreSQL **18** binaries: `bin/initdb`, `bin/postgres`,
`lib`, `share` and the distribution's required license notices. The app never downloads
executables at runtime. Use a trusted current patch release and preserve all notices.
The Go supervisor initializes with SCRAM authentication, a random persistent password,
loopback-only networking and private directories. A file lock prevents concurrent owners.
It shuts down only its own child; it never deletes a cluster or kills a PID from a file.

```sh
python -m pip install -e ".[desktop]"
python tools/stage_desktop.py --postgres /path/to/native/postgresql18
cd desktop
npm ci
npm start
npm run pack
```

If your Python distribution installs its license outside the interpreter prefix, pass
`--python-license /path/to/full/Python-license` when staging. Available Go/CPython/
PyInstaller notices are collected in `runtime/notices`; retain PostgreSQL and shared
library notices from the native distribution too.

`npm run pack` produces an unsigned native development directory under `dist/desktop`.
It includes Electron, the Go binary, a standalone Python decoder and the staged
PostgreSQL runtime; **no catalogue**. End users do not need Python installed.
Only native macOS/Linux bundles are supported at present. Windows local process
supervision remains unsupported. macOS signing/notarization has a separate
[candidate workflow](macos-signing.md); ordinary local builds remain unsigned/ad-hoc.
Native packaged archives are checked through [desktop release CI](desktop-releases.md). PostgreSQL/runtime
redistribution notices must accompany any bundle you distribute; the project's MIT license
does not relicense those dependencies or the original data.

By default data lives in Electron's OS user-data directory. To make an unpacked build
portable, choose an explicit directory on the same machine/OS:

```sh
export UKINFO_DATA_DIR=/absolute/path/to/portable-archive
```

In the app, choose **Add edition**, select an ISO/7z or extracted source folder, and
choose a complete edition or a first-10,000 sample. Required files are fingerprinted
before import. **Postcode setup** adds the pinned ONSPD ZIP for maps and nearby search.
See [desktop setup](desktop-setup.md) for temporary storage and supported containers.
The progress display supports cancellation, and the import retains a 5 GiB local disk
reserve. The invented demo can be imported from the same dialog without source media.
The private import capability stays in Electron main; public web hosting has no import UI.

For command-line provisioning, close the app first:

```sh
unset DATABASE_URL
set -o pipefail
python tools/search_demo.py | build/desktop/ukinfo-server import \
  --data-dir "$UKINFO_DATA_DIR/catalogue" --pg-bin /path/to/native/postgresql18/bin
cd desktop && npm start
```

Use `python -m ukinfo_reader.search_export --data ...` in place of the demo for real data.
Imports and searches work offline once the bundle and source data are available.
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
- Names match exactly after Unicode lowercase conversion and whitespace collapse by
  default. `match=prefix` applies starts-with matching to supplied name fields; a single
  trailing `*` enables it for one field (`surname=Smi*`). Prefixes require two Unicode
  characters for surnames/business names; first-name prefixes can contain one. Exact
  names such as Li and Ng have no minimum length. `?`, `%`, embedded/multiple `*`,
  contains matching and fuzzy matching are not supported. Literal apostrophes are supported.
- Postcode examples below illustrate formats, not installed reference locations.
  Postcode keys remove whitespace and uppercase. Districts (`AA1`), sectors (`AA1 1`)
  and full postcodes (`AA1 1AA`) are supported. A trailing `*` is optional (`AA1*`). **Keep the sector space where ambiguous:**
  `W11` is the district, `W1 1` is a sector. Matching checks the length of the full code,
  so `AA1` does not include `AA10`. Displayed partial source values can match a district
  when equal to that district; raw full postcodes never replace partial displayed codes.
- Prefix searches have no vocabulary-count rejection. Small expansions use bound integer
  arrays; broad expansions stay inside PostgreSQL in a shared materialized CTE. All matching
  terms participate before indexed per-edition candidates and the next result page are
  selected. The 8,192-term threshold chooses an execution path, not a result limit. Existing
  query timeouts, admission limits and 100-result maximum page size still apply. The existing
  C-collated vocabulary B-tree and integer entry indexes are used; no rebuild is required.
- `dataset=all` searches imported editions together. Samples are excluded when a complete
  import with the same manifest edition exists; samples remain individually selectable.
  Results identify their originating `dataset`. This is occurrence search, not person identity matching.
- People need a surname or a location constraint; businesses can also anchor a search
  with organisation, industry or source SIC code. Address-only searches require the
  [address index](address-search.md); classifications require the
  [research index](research-index.md). `near=AA11AA&radius_m=8047` selects an approximate
  five-mile circle. `radius_m` accepts integer metres from 50 to 50,000; defaults to 8,047.
  Nearby responses include `geography` provenance and per-item `distance_m`,
  `location_quality` and optional `location_terminated`. `/api/geography` reports availability.
  Distances use a spherical Earth model; see [accuracy and exclusions](geography.md).
- `limit=1..100` (default 50). Reply: `{items:[{dataset,id,source_id,record}],next_cursor:"..."}`.
  Repeat the original filters with `cursor=...`. Cursors are opaque and bound to normalized
  filters, postcode precision and the edition selection; no exact-total query or offset scan.
- Default catalogue ordering uses internal name/first-name codes and import ordinal, with dataset IDs
  breaking ties so editions interleave rather than exhausting one edition first. Each edition
  contributes at most `limit+1` indexed candidates before the global page is selected.
  Postcode-prefix and nearby searches order by postcode code before name codes to use the existing
  postcode index efficiently. This order is stable, **not alphabetical or chronological**.
  With the research index, `sort=name`, `sort=edition` and `sort=distance` provide
  explicit ordering; distance requires a nearby centre. See [ordering rules](research-index.md#ordering-and-classifications).
  Old-version cursors may be invalid after upgrading or a catalogue change. Edition labels use cover-disc dates,
  not verified electoral-roll dates.
- `GET /api/export` takes the same filters, limit and cursor and returns the matching
  bounded page as NDJSON. Original JSONL page/record downloads retain exact source numeric
  bytes and nulls. Readable CSV is generated in the UI with selected human-facing fields,
  edition, source identity and citation; cells are quoted and formula prefixes neutralized.
  CSV is a convenience view, not a lossless substitute for original JSONL.
- Import ordinals are stable only within a dataset; source identity is edition plus
  source ID (SE4 additionally qualifies entry IDs with the source segment).

## Postcode locations and nearby search

The optional [ONSPD reference](geography.md) adds radius searches using current and
retired Great Britain postcode points. Import it once with an owner connection; it is
not bundled with desktop binaries. No re-import of the source editions is needed.
Nearby search then runs locally/offline. The pinned August 2026 reference uses about
840 MiB in PostgreSQL. Northern Ireland has separate licensing and is excluded.

## Using the interface

The edition selector defaults to **All editions**. Enter a surname/business name,
postcode or address in the **Who?** and **Where?** groups. A name is optional for a
location search. **Name search options** contains independent surname/first-name
matching; new first-name searches include following given names by default. Use a
trailing `*` for a prefix. Detailed house/building/flat/locality/county controls live in
**More address filters**, while street and town remain visible.

Enter a postcode once, then choose **In this postcode** or **Nearby**. Nearby reveals
a 1/2/5/10/25-mile distance and accepts a full postcode, district or sector. Results show
approximate postcode-point distances. Catalogue order is the default; with the research
index, **Nearest to search centre** selects distance order. Nearby searches exclude
incomplete/unlocated and non-geographic postcodes. See the
[search form design](search-interface.md) for controls, saved-link compatibility and
research references.

Page sizes are 25/50/100 with controls above and below results. Search URLs preserve filters,
page size and the current cursor; browser Back/Forward restores searches. A directly opened
page URL cannot reconstruct the app's earlier page history; use browser history or start
that search again. Search terms are visible in bookmarked URLs and browser history.

**View record** separates person/business details, formatted addresses, sources and
plain-language data-quality notes. Known place codes are translated (for example `T` is
Town); undecoded fields/codes remain available in **Show original source fields**.
Offsets and raw references are kept there rather than presented as the record's identity.

![Readable record view using invented demo data](screenshots/record-preview.png)

Copy addresses/citations, export the selected original record, or export the current page
in original JSONL or readable CSV. Source warnings never disappear from original exports.

**Group identical listings on this page** is optional and reversible. It groups matching
normalized names, listing types, complete addresses and telephone numbers among the
returned occurrences. It requires a recorded street and house/building, and leaves
incomplete names/postcodes and warned records separate. Sources remain individually
inspectable and a comparison table shows their human-facing fields. This does not establish
that listings describe the same person. Grouping does not combine entries on different
pages, alter the source pagination or remove occurrences from page exports.

**Group across search** uses the optional [research index](research-index.md) and
group-aware cursors to group across pages. Page-local grouping remains a separate mode;
it does not change the underlying occurrence pagination.

See [the interface polish measurements](research/search-polish.md) for bounded local
checks of the new query paths. They do not establish hosted capacity.

## Verification

```sh
python -m unittest discover -s tests -v
cd server
go test -race ./...
UKINFO_TEST_DATABASE_URL='postgres://...' go test -race -count=1 ./internal/catalog -v
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
The [compact catalogue report](research/compact-catalogue.md) measures full-field storage
and records the latency/throughput tradeoff.

## Local verification, 17 October 2026

- The Python regression suite, including local source regression tests, passed.
- Go race-enabled PostgreSQL integration tests and `go vet` passed.
- Exclusive ownership, restart persistence and same-platform data-directory relocation
  passed with PostgreSQL 18.6 on native macOS ARM64; Linux lifecycle tests passed in
  the PostgreSQL container (relocation coverage is also in CI).
- Real Go/PostgreSQL browser tests passed for pagination, source details, page export,
  business search and a 390px mobile viewport.
- The native macOS Electron development app **and packaged bundle** passed fresh-cluster
  import, search and owned PostgreSQL shutdown checks using invented entries.
- Hash-verified first-N source imports passed: 10,000 entries from 2005, and 1,000 each
  from 1998, 1999 and 2004. All four complete editions were then imported and every
  record/index projection checked: 27,983,545 (1998), 46,355,850 (1999), 63,373,974
  (2004) and 51,350,897 (2005). The combined catalogue occupies 38.26 GiB.
- Native Electron page downloads preserved the invented source records and nulls; the
  trusted project link opens externally while untrusted popup targets remain blocked.
- The new desktop wizard passed bundled-decoder demo ingestion and cancellation of a
  real 2005 import; canceled data was not published.
- Full-corpus warm HTTP checks at 100/s and 300/s completed 24,000 scheduled requests
  with zero errors/drops under a shared 2-CPU/1-GiB PostgreSQL/API cap. These are local,
  short repeated-key tests on a Mac host; see the [report](research/compact-catalogue.md).
- An additional mixed-edition 100/s check with all complete sources present passed
  6,000 scheduled requests: p95 8.86 ms, zero errors/drops, under the same container cap.
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

## Schema 1 preview migration

Schema 2 is incompatible with the first preview. Startup refuses a schema-1 catalogue
without modifying it. Create a **separate** destination and keep the source until verified:

```sh
export SOURCE_DATABASE_URL='postgres://.../old_catalogue?sslmode=disable'
export DATABASE_URL='postgres://.../new_catalogue?sslmode=disable'
build/desktop/ukinfo-server migrate
build/desktop/ukinfo-server repack-v1
build/desktop/ukinfo-server verify --dataset DATASET_KEY
build/desktop/ukinfo-server stats
```

For an app-managed source, use `repack-v1 --source-data-dir /old/catalogue --pg-bin
/path/to/pg18/bin`. The destination can use `--data-dir /new/catalogue` with
`DATABASE_URL` unset. Close the desktop app first. Repacking reads the original JSONB
fields and IDs through a read-only connection; it does not require another source export.
Each edition commits separately. If interrupted, completed editions remain in the target;
there is no skip/resume flag. Plan space for both copies or re-import into a fresh target.
Do not delete the old catalogue until its replacement has been checked and backed up.

`verify` decompresses **every** block, checks its checksum and framing, recomputes the
whole stored record digest, validates counts, and compares every search projection with
the original record. Its output contains aggregate counts only. Protocol-2 source imports
have an independently computed decoder digest; legacy repacks only have the imported
stream digest. Verification establishes preservation, not that historical source values
are factually correct. `stats` reports relation sizes and import timings without records.

## Address searches

See [address search](address-search.md) for street, village/locality, town/city and
county matching, optional index provisioning, coverage and storage requirements.

## Research index and ordered results

The optional [research index](research-index.md) enables grouping across the complete
search, original-source reference lookup, and resolved industry/SIC business filters.
Name A–Z, nearest and publication order use server-side result ordering and stable
cursors. The desktop's archive manager builds indexes for existing imports; an index
build does not reread the ISO or change the original record blocks. Its storage is
additional to the earlier 38.26-GiB compact-catalogue measurement.
