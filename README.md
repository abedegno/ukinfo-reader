# ukinfo-reader

Read historical UK-Info Disk databases with Python. Search names and postcodes, reconstruct
stored household addresses, and export records to JSON Lines or CSV without installing the
original Windows application.

## Choose how to use it

- **Desktop app:** the [desktop guide](docs/desktop-releases.md) explains first-launch
  import, platform packages and release status. [Release downloads](https://github.com/abedegno/ukinfo-reader/releases)
  only show published builds; current preview builds remain drafts. The app includes
  its runtimes and an invented demo, with no directory data bundled.
- **Web search / self-hosting:** follow [server setup](docs/search-app.md) and the
  [hosting guide](docs/hosting.md). There is no project-operated public data service.
- **Python / preservation:** use the [CLI quick start](#quick-start) below for direct
  reading, validation and streaming exports from supported media.

| Component     | Platforms checked                                                | Scope                                                                |
| ------------- | ---------------------------------------------------------------- | -------------------------------------------------------------------- |
| Desktop       | macOS Apple Silicon, macOS Intel, Linux x86-64 (Ubuntu 22.04 CI) | Native packages; Windows desktop and Linux ARM are not yet supported |
| Web/API       | Go + PostgreSQL 18                                               | Browser UI shared with desktop; self-hosted deployment               |
| Python reader | macOS, Linux, Windows CI; Python 3.10+                           | CLI reader; optional dependencies for media preparation              |

“Portable” means no separately installed Python/Go/PostgreSQL is needed. User data
normally lives in the OS application-data folder. `UKINFO_DATA_DIR` selects a private
archive folder; moving the executable does not move that archive. Cold backup/restore
requires matching OS, CPU and PostgreSQL major version; this is not a cross-platform
single-file database. See [backup and archive switching](docs/catalogue-management.md).

**Supported releases:**

| Release                                        | Distribution               |    Entries | Internet Archive source                                                  |
| ---------------------------------------------- | -------------------------- | ---------: | ------------------------------------------------------------------------ |
| UK-Info Disk SE                                | PC Format 78, January 1998 | 27,983,545 | [PC Format 78](https://archive.org/details/pc-format-78)                 |
| UK-Info Disk SE 4.0                            | PC Format 94, April 1999   | 46,355,850 | [PC Format 94b](https://archive.org/details/pcf-94b-apr-1999-uk-info)    |
| UK-Info 2004 SE (residential)                  | PC Plus 218, July 2004     | 63,373,974 | [PC Plus 218 DVD](https://archive.org/details/pcp-218-dvd)               |
| UK-Info Disk V10.5 SE (residential + business) | PC Format 175, June 2005   | 51,350,897 | [PC Format 175 DVDs](https://archive.org/details/pcf-175-dvd-1-06-05.7z) |

### Find the source media

Use the Internet Archive item links above and choose the matching source file:

- **PC Format 78:** `PC_FORMAT_78.iso`.
- **PC Format 94b:** `PCF 94b apr 1999 UK-INFO.iso`, rather than the other ISO in the item.
- **PC Plus 218:** `PCP218_DVD.iso` for UK-Info 2004 SE.
- **PC Format 175:** `PCF175DVD2_06_05.7z` for UK-Info 2005 SE. The archive item
  contains both DVDs; UK-Info is on **DVD 2**, despite `dvd-1` in the item's URL.

The desktop app can import supported ISO/7z files directly. For the Python reader,
follow the extraction examples below. Source images and directory data are not bundled
with this repository or the app.

All supported scopes have been validated across their complete source streams. Known
[early-disc defects](docs/formats/pcf78.md#source-defects-and-validation-limits) and
[2004/2005 anomalies](docs/formats/modern-se.md#source-anomalies-and-validation) are reported
explicitly. Counts are source entries, not distinct people. The two newer readers include
electoral and residential telephone entries. V10.5 SE also includes business records and
business updates/deletions; the supplied 2004 SE disc omits the business record file.
Other editions, Standard and Pro releases require separate identification and validation.

This repository contains original reader code, format documentation and synthetic tests.
The disc image, original software, database and exported directory records are excluded.

## Web and desktop search (development preview)

A Go + PostgreSQL search service, shared web UI and Electron desktop shell are now
available. The Python reader provides verified streaming ingestion. Edition selection,
people/business search across editions, partial postcodes and name prefixes, optional
ONSPD nearby search, readable
record details, lossless page exports and a desktop import dialog are implemented. All four supported editions have been imported and
verified: **189,064,266 source occurrences in a 38.26 GiB PostgreSQL catalogue**.
See the [complete-edition measurements](docs/research/compact-catalogue.md#all-four-complete-editions).
[Setup, current limits and portable app instructions →](docs/search-app.md)

Street, locality, town/city and county search is available through an optional
[additive address index](docs/address-search.md), including on existing catalogues.
[Search form design and controls](docs/search-interface.md) explains the Who/Where layout.
[Refine names and premises](docs/search-refinements.md) with house/building/flat filters,
given-name modes, middle initials and removable search filters.
[Research lists and saved searches](docs/research-lists.md) retain notes/citations,
recoverable drafts and optional private search history locally;
[optional maps](docs/maps.md) show approximate postcode positions.
[Desktop setup, build and release instructions](docs/desktop-releases.md) cover the
first-launch wizard and native preview packages.
[Explore exact addresses and save record links](docs/address-exploration.md) to compare
co-listed people/businesses and available edition snapshots.

![Search application showing invented demo records](docs/screenshots/search-preview.png)

## Quick start

Requires **Python 3.10+** on macOS, Linux or Windows. The reader has no third-party runtime
dependencies. Run these commands from a checkout:

```sh
python -m venv .venv
# macOS/Linux:
source .venv/bin/activate
# Windows PowerShell instead: .venv\Scripts\Activate.ps1
python -m pip install .
```

Obtain the [matching disc from Internet Archive](https://archive.org/details/pcf-94b-apr-1999-uk-info).
That item contains two ISOs: select **`PCF 94b apr 1999 UK-INFO.iso`**.
Extract just `ukid.dat` and `ukid.idx` into a local `extracted/` directory. For example,
with [7-Zip](https://www.7-zip.org/):

```sh
7z x 'PCF 94b apr 1999 UK-INFO.iso' -oextracted ukid.dat ukid.idx
ukinfo-read --identify
ukinfo-read --sector 'AB10 1' --surname Smith --limit 20
```

Some systems name the extractor `7zz`; use that in place of `7z`. Alternatively, copy the
two files from a mounted disc image. The original installer and DLLs are not needed.
`--identify` checks both database files against the verified release's SHA-256 fingerprints.
[Source details and checksums →](docs/editions.md)

### Earlier PC Format 78 disc

Obtain [PC FORMAT Issue 78 Cover Disk](https://archive.org/details/pc-format-78), file
`PC_FORMAT_78.iso`. Extract these nine files into a separate directory:

```sh
7z x PC_FORMAT_78.iso -oextracted78 Uk1.idx Ukdemo.dat d1.dat d2.dat d3.dat d4.dat d5.idx d6.dat d7.dat
ukinfo-read --data extracted78 --identify
ukinfo-read --data extracted78 --surname Smith --first-name John --limit 20
ukinfo-validate --data extracted78 --output validation.json
```

The reader selects the decoder from the required filenames and sizes. PC Format 78 supports
name/postcode/sector filters, JSON Lines and CSV export. Its row format differs from SE 4.0;
`--segment` and `--field` apply only to SE 4.0. See [the early format](docs/formats/pcf78.md)
for recovered fields, index membership and preserved source anomalies.

### 2004 and 2005 sources

Obtain the matching [2004 disc](https://archive.org/details/pcp-218-dvd) or
[2005 archives](https://archive.org/details/pcf-175-dvd-1-06-05.7z), then follow
[the extraction instructions](docs/formats/modern-se.md#source-extraction).
The address tables are required in addition to the smaller structural-inspection profile.
Then use the normal reader:

```sh
ukinfo-read --data private/2005-inspect --identify
ukinfo-read --data private/2005-inspect --surname Smith --limit 20
ukinfo-read --data private/2005-inspect --surname Smith --format csv > results.csv
ukinfo-validate --data private/2005-inspect --output private/2005-report.json
```

Replace the directory for 2004. Output distinguishes electoral and telephone occurrences,
applies the 2005 residential and business updates, and flags source anomalies.
For business searches use `--organisation "EXACT STORED NAME"` or `--entry-type business`.
`--all` and postcode searches include businesses by default. See
[the output field reference](docs/usage.md#20042005-residential-output-fields).

## Search and export

```sh
# Whole-disc exact name search; case is ignored.
ukinfo-read --surname Smith --first-name John --limit 20

# Full postcode lookup; spaces and case are ignored.
ukinfo-read --postcode 'AB10 1AL'

# Export a postcode sector as CSV.
ukinfo-read --sector 'AB10 1' --format csv > results.csv

# Inspect supported editions and the stored field definitions.
ukinfo-read --editions
ukinfo-read --schema

# Validate the entire source disc, without printing directory records.
ukinfo-validate --output validation.json
```

Pass `--data /path/to/files` to either command if your files are elsewhere. You can also run
`python -m ukinfo_reader` in place of `ukinfo-read`. Searches stream their results. SE 4.0 name
searches scan compressed columns; PC Format 78 surname searches use its original B-tree index.

[CLI reference and export examples →](docs/usage.md) · [Python API →](docs/api.md)

## What has been recovered?

The SE 4.0 disc passed a full validation of **248,690 blocks** across **9,565 postcode
sectors**:

| Stored items                            |      Count |
| --------------------------------------- | ---------: |
| Directory entries, including businesses | 46,355,850 |
| Addresses                               | 24,279,520 |
| Postcodes                               |  1,462,313 |

The SE 4.0 reader recovers full forenames, surnames, second initials, address components,
postcodes, telephone numbers and the original business fields. `household_id` groups entries
at the same stored address. It is a database address identifier; it does not prove kinship
or distinguish separate households that share an address.

For genealogy, treat this as a historical directory snapshot. The SE 4.0 files are dated
18 December 1998, but the underlying records' exact reference dates are unverified.
The data includes residential and business entries; the entry count is not a count of
unique people. No birth dates or family relationships are established by this reader.
Field meanings that remain uncertain are documented rather than inferred.

The SE 4.0 [full validation report](docs/validation/se4-pcf94-1999-04.json) records hashes and checks.
Validation establishes internal consistency, including numeric checksums, record boundaries
and agreement between postcode relationships. Output has not been compared against a
running original Windows application.

The earlier disc has **19,018,491 usable indexed records** containing **27,983,545 entries**.
It recovers full forenames, surnames, house/building names, street, locality, town, county,
postcodes and stored telephone components. Its [validation report](docs/validation/se-pcf78-1998-01.json)
records one excluded mismatched index pointer, 113 unindexed physical records, three unresolved
surname references, 13 indexed records with nonstandard postcode values and one inconsistent
place-navigation range. These source
issues are preserved or excluded as documented; their historical cause is unknown.

The 2004 and 2005 residential readers reconstruct the address hierarchy and separate
electoral names from telephone contacts. The 2005 deletion index removes telephone
occurrences while retaining electoral names in combined records. Full reports for
[2004](docs/validation/se2004-pcp218-2004-07-residential.json) and
[2005](docs/validation/se2005-pcf175-2005-06-complete.json) record every stream's
counts, fingerprints and source anomalies. All place joins were compared independently
with the original decoder routines.

## Documentation and development

- [Supported editions and provenance](docs/editions.md)
- [PC Format 78 binary format and decoding evidence](docs/formats/pcf78.md)
- [SE 4.0 binary format and decoding evidence](docs/formats/se4.md)
- [2004/2005 residential format, address joins and update rules](docs/formats/modern-se.md)
- [Adding support for later editions](docs/adding-editions.md)
- [Contributing and running tests](CONTRIBUTING.md)
- [Changes](CHANGELOG.md)

CI uses generated, fictional data and downloads no directory databases. Tests cover all 13
numeric encodings, signed overflow, shared strings, postcode bitmaps, early-disc records and
B-tree pages, joins, searches and exports.

## License

The new code and documentation are available under the [MIT license](LICENSE).
That license grants no rights to the original UK-Info software or directory data.
UK-Info and PC Format names identify the historical product and magazine; this project is
independent of their publishers.

### Guided desktop setup

The desktop app can prepare a supported ISO or 7z directly, verify its edition and
import a sample or complete catalogue. **Postcode setup** adds the optional official
ONSPD ZIP for nearby search and maps. See [desktop setup](docs/desktop-setup.md) for
supported discs, temporary space requirements and cancellation.

**Manage archive** shows storage/completeness, upgrades samples, restarts interrupted
setup and safely removes imports while preserving research notes. See
[catalogue management](docs/catalogue-management.md) and [macOS release signing](docs/macos-signing.md).

Desktop problems: use **Help → Export diagnostics…** and attach the reviewed JSON to
a [desktop bug report](https://github.com/abedegno/ukinfo-reader/issues/new?template=desktop.yml).
See [diagnostics contents and exclusions](docs/diagnostics.md).
