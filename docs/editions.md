# Supported editions

| Edition ID | Product | Distribution | Status |
|---|---|---|---|
| `se-pcf78-1998-01` | UK-Info Disk SE | PC Format issue 78, January 1998 | Supported; entire database validated with documented source defects |
| `se4-pcf94-1999-04` | UK-Info Disk SE 4.0 | PC Format issue 94, April 1999 | Supported; entire database validated |
| `se2003-dpcs0104` | UK-Info 2003 SE | Supplied DPCS0104DVD image | Supported residential and business searches/exports; source anomalies documented |
| `se2004-pcp218-2004-07` | UK-Info 2004 SE | PC Plus issue 218, July 2004 | Supported residential searches/exports; source anomalies documented |
| `se2005-pcf175-2005-06` | UK-Info Disk V10.5 SE | PC Format issue 175, June 2005 | Supported residential and business searches/exports, including updates/deletions |

**Other editions and Standard/Pro releases are not supported.** Matching product
names or familiar filenames do not establish format compatibility. To propose another
release, follow [Adding editions](adding-editions.md).

## 2003 SE — DPCS0104DVD

The supplied image's installer identifies this product as **UK-Info 2003 SE** and
`CDID.TXT` contains `DPCS0104DVD`. Its distinct layout is dispatched explicitly;
it is not treated as a renamed 2004/2005 database. See the [format and extraction
guide](formats/2003-se.md), [whole-source report](validation/se2003-dpcs0104.json)
and [original-decoder comparisons](validation/se2003-dpcs0104-reference-comparison.json).
The reader validates **59,886,841 source entries**, including **2,471,038 businesses**.
Seventeen unresolved name references are flagged with raw codes retained. The source
omits its industry-description dictionary: business names, addresses, telephone/fax
and SIC codes are available, while missing descriptions are explicitly flagged.
No public archive source has been verified for this particular supplied image.

## 2004 SE and V10.5 SE (2005)

These exact sources are in `SUPPORTED_EDITIONS`. `Database`, `ukinfo-read`
and `ukinfo-validate` support them. Extraction requires additional address files beyond
the smaller profiles used by the earlier structural inspector. See
[extraction, source identities, entry selection and anomalies](formats/modern-se.md).
No other 2004/2005 distribution is implicitly supported; matching product names alone
do not establish compatible source identity. V10.5 SE business records are included. The supplied 2004 SE media contains a business
word index/dictionary but no `bus.dat`, so it supplies no separate business listings.
The 2005 query profile now requires the nine additional business files listed in the format guide.

## Identify your copy

```sh
ukinfo-read --editions
ukinfo-read --data extracted --identify
```

Ordinary queries select the edition profile by the exact sizes of all required database files.
`--identify` and `ukinfo-validate` additionally verify their SHA-256 fingerprints. Use these
commands to establish byte-for-byte identity. Unknown sizes are rejected with an unsupported
edition error. A checksum mismatch may indicate an altered, damaged or different release;
it is not silently added as a supported edition.

The registry is `src/ukinfo_reader/editions.py`. A registry entry identifies a verified release;
adding hashes alone does not establish compatibility with its decoder.

## PC Format 94, April 1999

Source: [Internet Archive item `pcf-94b-apr-1999-uk-info`](https://archive.org/details/pcf-94b-apr-1999-uk-info).
The item contains both the main PC Format disc and the UK-Info disc. Choose the exact filename
below. Internet Archive availability does not change the license of the original contents.

| File | Bytes | Fingerprint |
|---|---:|---|
| `PCF 94b apr 1999 UK-INFO.iso` | 677,095,424 | SHA-1 `8d75485218fcdfce5cae2b04274fce173d9cd951` |
| `ukid.idx` | 82,609,022 | SHA-256 `9029772fd7d47e9e55a3c30908c464146c69c1dd6d2f13ec0d5fda2040080195` |
| `ukid.dat` | 496,659,177 | SHA-256 `9e1eb85ecac003f3bc4cf74f050f984860dcd502c5be3f847eca8bb95faf0ce6` |

The ISO's size and SHA-1 were compared with the item's [file metadata](https://archive.org/metadata/pcf-94b-apr-1999-uk-info).
Disc metadata records mastering on 1–2 March 1999. The two database files carry a timestamp
of 18 December 1998; that timestamp is not proof of the records' collection or electoral-roll date.

The reader was reconstructed by static inspection of the schema, data blocks and database
library extracted from the disc's InstallShield cabinet using [unshield](https://github.com/twogood/unshield).
No unshield code, original binaries, disassemblies or database contents are distributed here.
The public implementation uses Python's standard library.

See the [format evidence](formats/se4.md) and [validation report](validation/se4-pcf94-1999-04.json).

## PC Format 78, January 1998

Source: [Internet Archive item `pc-format-78`](https://archive.org/details/pc-format-78), file
`PC_FORMAT_78.iso`. Its size is 518,256,640 bytes and SHA-1 is
`9d41302896f6e73ead112a4206080aaaa62c00f0`. These match the item's
[file metadata](https://archive.org/metadata/pc-format-78). A downloaded copy with a browser-added
`(1)` filename suffix has the same identity. ISO mastering dates are November 1997;
`Uk1.idx` and `Ukdemo.dat` carry September 1997 timestamps. The underlying records' reference
date is unverified. The program is named `UK-Info Disk SE.exe`; a numbered SE release
has not been established from this source.

Nine files are required: `Uk1.idx`, `Ukdemo.dat`, `d1.dat`, `d2.dat`, `d3.dat`, `d4.dat`,
`d5.idx`, `d6.dat`, `d7.dat`. Exact byte sizes and SHA-256 values appear in the
[edition registry](../src/ukinfo_reader/editions.py) and [full report](validation/se-pcf78-1998-01.json).
No original program, runtime DLL or installer is required to use the Python reader.

The decoder was reconstructed by static inspection of the executable and `ctreestd.dll`,
then checked against the entire database. See [the format and source defects](formats/pcf78.md).
