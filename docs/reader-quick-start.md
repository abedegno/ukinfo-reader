# Python reader quick start

Requires **Python 3.10+** on macOS, Linux or Windows. The reader has no third-party runtime
dependencies. Run these commands from a checkout:

```sh
git clone https://github.com/abedegno/ukinfo-reader.git
cd ukinfo-reader
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
[Source details and checksums →](editions.md)

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
`--segment` and `--field` apply only to SE 4.0. See [the early format](formats/pcf78.md)
for recovered fields, index membership and preserved source anomalies.

### 2004 and 2005 sources

Obtain the matching [2004 disc](https://archive.org/details/pcp-218-dvd) or
[2005 archives](https://archive.org/details/pcf-175-dvd-1-06-05.7z), then follow
[the extraction instructions](formats/modern-se.md#source-extraction).
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
[the output field reference](usage.md#20042005-residential-output-fields).

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

[CLI reference and export examples →](usage.md) · [Python API →](api.md)


[All documentation](README.md) · [Supported editions](editions.md)
