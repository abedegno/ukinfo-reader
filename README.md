# ukinfo-reader

Read historical UK-Info Disk databases with Python. Search names and postcodes, reconstruct
stored household addresses, and export records to JSON Lines or CSV without installing the
original Windows application.

**Supported releases:**

| Release | Distribution | Indexed entries |
|---|---|---:|
| UK-Info Disk SE | PC Format 78, January 1998 | 27,983,545 |
| UK-Info Disk SE 4.0 | PC Format 94, April 1999 | 46,355,850 |

Both complete source databases have been validated, with [known early-disc source defects](docs/formats/pcf78.md#source-defects-and-validation-limits)
reported explicitly. Other editions, years, Standard and Pro releases are not yet supported. Later-edition support
is welcome; each release needs its own identification and validation evidence.

**Experimental archaeology:** the PC Plus 218 UK-Info 2004 SE and PC Format 175
UK-Info V10.5 SE (2005) sources have separate [structural decoders](docs/formats/modern-se.md)
and `ukinfo-inspect` reports. These editions are **not yet supported by normal searches
or exports**. Their physical record counts must not be treated as live directory totals.

This repository contains original reader code, format documentation and synthetic tests.
The disc image, original software, database and exported directory records are excluded.

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

## Documentation and development

- [Supported editions and provenance](docs/editions.md)
- [PC Format 78 binary format and decoding evidence](docs/formats/pcf78.md)
- [SE 4.0 binary format and decoding evidence](docs/formats/se4.md)
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
