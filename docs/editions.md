# Supported editions

| Edition ID | Product | Distribution | Status |
|---|---|---|---|
| `se4-pcf94-1999-04` | UK-Info Disk SE 4.0 | PC Format issue 94, April 1999 | Supported; entire database validated |

**No later editions, or Standard/Pro releases, are currently supported.** Matching product
names or familiar filenames do not establish format compatibility. To propose another
release, follow [Adding editions](adding-editions.md).

## Investigated editions

| Distribution | Status | Evidence |
|---|---|---|
| PC Format 78, January 1998, UK-Info Disk SE | Unsupported; different layout | [Initial investigation](pcf78-investigation.md) |

## Identify your copy

```sh
ukinfo-read --editions
ukinfo-read --data extracted --identify
```

Ordinary queries select the edition profile by the exact sizes of both database files.
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
