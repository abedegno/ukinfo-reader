# ukinfo-reader

Read historical UK-Info Disk databases with Python. Search names and postcodes, reconstruct
stored household addresses, and export records to JSON Lines or CSV without installing the
original Windows application.

**Currently supported: UK-Info Disk SE 4.0, supplied with PC Format issue 94, April 1999.**
Other editions, years, Standard and Pro releases are not yet supported. Later-edition support
is welcome; each release needs its own identification and validation evidence.

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
`python -m ukinfo_reader` in place of `ukinfo-read`. Searches stream their results. Name
searches scan compressed columns; building a modern search index is future work.

[CLI reference and export examples →](docs/usage.md) · [Python API →](docs/api.md)

## What has been recovered?

The supported disc passed a full validation of **248,690 blocks** across **9,565 postcode
sectors**:

| Stored items | Count |
|---|---:|
| Directory entries, including businesses | 46,355,850 |
| Addresses | 24,279,520 |
| Postcodes | 1,462,313 |

The reader recovers full forenames, surnames, second initials, address components,
postcodes, telephone numbers and the original business fields. `household_id` groups entries
at the same stored address. It is a database address identifier; it does not prove kinship
or distinguish separate households that share an address.

For genealogy, treat this as a historical directory snapshot. The files are dated
18 December 1998, but the underlying records' exact reference dates are unverified.
The data includes residential and business entries; the entry count is not a count of
unique people. No birth dates or family relationships are established by this reader.
Field meanings that remain uncertain are documented rather than inferred.

The [full validation report](docs/validation/se4-pcf94-1999-04.json) records hashes and checks.
Validation establishes internal consistency, including numeric checksums, record boundaries
and agreement between postcode relationships. Output has not been compared against a
running original Windows application.

## Documentation and development

- [Supported editions and provenance](docs/editions.md)
- [SE 4.0 binary format and decoding evidence](docs/formats/se4.md)
- [Adding support for later editions](docs/adding-editions.md)
- [Contributing and running tests](CONTRIBUTING.md)
- [Changes](CHANGELOG.md)

CI uses generated, fictional data and downloads no directory databases. Tests cover all 13
numeric encodings, signed overflow, shared strings, postcode bitmaps, joins, searches and exports.

## License

The new code and documentation are available under the [MIT license](LICENSE).
That license grants no rights to the original UK-Info software or directory data.
UK-Info and PC Format names identify the historical product and magazine; this project is
independent of their publishers.
