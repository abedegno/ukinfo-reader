# ukinfo-reader

Read historical UK-Info Disk databases with Python. Search names and postcodes, reconstruct
stored household addresses, and export records to JSON Lines or CSV without installing the
original Windows application.

**Supported releases:**

| Release | Distribution | Entries |
|---|---|---:|
| UK-Info Disk SE | PC Format 78, January 1998 | 27,983,545 |
| UK-Info Disk SE 4.0 | PC Format 94, April 1999 | 46,355,850 |
| UK-Info 2004 SE (residential) | PC Plus 218, July 2004 | 63,373,974 |
| UK-Info Disk V10.5 SE (residential + business) | PC Format 175, June 2005 | 51,350,897 |

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
people/business search across editions, partial postcodes and name prefixes, readable
record details, lossless page exports and a desktop import dialog are implemented. All four supported editions have been imported and
verified: **189,064,266 source occurrences in a 38.26 GiB PostgreSQL catalogue**.
See the [complete-edition measurements](docs/research/compact-catalogue.md#all-four-complete-editions).
[Setup, current limits and portable app instructions →](docs/search-app.md)

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

| Stored items | Count |
|---|---:|
| Directory entries, including businesses | 46,355,850 |
| Addresses | 24,279,520 |
| Postcodes | 1,462,313 |

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
