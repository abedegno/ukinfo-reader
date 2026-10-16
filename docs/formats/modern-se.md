# UK-Info SE: the 2004 and 2005 sources

## Status and scope

This is experimental format research. The independent Python implementation can decode
physical residential records, fixed dictionaries and the observed c-tree leaf indexes.
The inspector verifies exact source hashes and checks the entire residential streams.
It produces aggregate reports without displaying directory records.

**Neither edition is supported by `Database`, `ukinfo-read` or normal exports.** Physical
records include entries outside the surname indexes and, in 2005, records affected by
updates. A clean structural scan does not establish the original program's result set.
Address assembly, source flags, business records and update selection need further work.

| Source | Base residential file | Update residential file |
|---|---:|---:|
| PC Plus 218, UK-Info 2004 SE | 540,263,828 bytes | None in the extracted payload |
| PC Format 175, UK-Info Disk V10.5 SE | 347,726,510 bytes | 19,929,983 bytes |

The 2004 program identifies itself as UK-Info 2004 SE; its library is `icd_92se.dll`.
The 2005 program identifies itself as V10.5 SE; its library is `icd_10.dll`.
Magazine dates identify distributions, not the reference dates of the underlying people
or addresses. The program also contains business and telephone-directory components;
the files should not be described as a single dated electoral roll.

## Source extraction

Keep each edition in its own directory. Source media and extracted files belong outside
Git, for example under the ignored `private/` directory. Do not run the original installer.
The following commands use [7-Zip](https://www.7-zip.org/) and
[unshield](https://github.com/twogood/unshield) for InstallShield cabinets.

### PC Format 175 (2005)

The supplied `PCF175DVD1_06_05.7z` and `PCF175DVD2_06_05.7z` archives contain separate
DVD images. The UK-Info installer and data are on **DVD 2**, under
`Data/Bonus/Bonus 01/InfoDisk/`.

```sh
7zz x PCF175DVD2_06_05.7z -oprivate/2005-media
7zz x private/2005-media/PCF175DVD2_06_05.ISO -oprivate/2005-disc \
  'Data/Bonus/Bonus 01/InfoDisk/*'
unshield -d private/2005-cab x \
  'private/2005-disc/Data/Bonus/Bonus 01/InfoDisk/data1.cab'
mkdir -p private/2005-inspect
cp 'private/2005-disc/Data/Bonus/Bonus 01/InfoDisk/uk.dat' private/2005-inspect/
cp private/2005-cab/Data_Files/{d2.dat,d3.dat,d4.dat,d6new.dat,ukspos.idx} private/2005-inspect/
cp private/2005-cab/Update_data/{ukupd.dat,d2upd.dat,d3upd.dat,d4upd.dat,d6newupd.dat,uksposupd.idx,delr.idx} private/2005-inspect/
ukinfo-inspect --data private/2005-inspect --output private/2005-report.json
```

### PC Plus 218 (2004)

Wait for the browser download to finish and the `.crdownload` suffix to disappear.
The completed supplied ISO is named `PCP218_DVD.iso`. Its UK-Info payload is under
`Software/Full/UK-Info/`. The cabinet puts the tables in `Data_Files_Compress`;
the extracted files themselves are directly readable.

```sh
7zz x PCP218_DVD.iso -oprivate/2004-disc 'Software/Full/UK-Info/*'
unshield -d private/2004-cab x private/2004-disc/Software/Full/UK-Info/data1.cab
mkdir -p private/2004-inspect
cp private/2004-disc/Software/Full/UK-Info/uk.dat private/2004-inspect/
cp private/2004-cab/Data_Files_Compress/{d2.dat,d3.dat,d4.dat,d6new.dat,ukspos.idx} private/2004-inspect/
ukinfo-inspect --data private/2004-inspect --output private/2004-report.json
```

The `cp` examples use POSIX shells with brace expansion. On Windows, copy the listed
files into one directory and run the same inspector command. Inspection uses only the
Python standard library and needs no original executable or DLL.

The exact inspection file fingerprints are in
[`modern_sources.py`](../../src/ukinfo_reader/modern_sources.py). These experimental
profiles are deliberately separate from the supported query registry. Media fingerprints
are recorded in [modern source media](../validation/modern-source-media.json). A matching
Internet Archive item has not yet been independently established for these supplied files;
do not substitute an unverified download with a similar product name.

## Residential physical layout

There is no file header. All multibyte numbers in the physical records below are little
endian. Offsets are relative to the start of a record. `N = byte[0] & 7`, with observed
counts 1 through 5. `K = (byte[0] >> 3) & 7` is a source kind, not a verified year/type label.

| Offset | Bytes | Content |
|---|---:|---|
| 0 | 1 | `N`, `K`, and optional-field bits `0x40`, `0x80` |
| 1 | 3 | First stored name code, including source flag bits |
| 4 | 3 | Postcode ID in low 21 bits; upper three bits control other fields |
| 7 | 2 | House-number dictionary ID; zero denotes absence |
| 9 | `3 × (N−1)` | Further stored name codes |

The variable cursor then consumes these fields **in this order**:

| Condition | Bytes | Experimental API field |
|---|---:|---|
| `byte[0] & 0x80` | 3 | `optional_flag80`; `byte[6] & 0x40` additionally supplies bit 24 |
| `byte[0] & 0x40` | 2 | `optional_flag40` |
| `K` equals 2 or 5 | 3 | `optional_kind25` |
| `1 <= K <= 6` | 3 | `optional_kind16` |
| `byte[6] & 0x20` | 2 | `optional_pc20` |
| `byte[6] & 0x80` | 1 | `optional_pc80` |

Thus the physical length is `6 + 3*N` plus the sizes of present optional fields.
`decode_residence()` retains all codes and optional values, using `None` for absent
fields. It does not assign unverified human meanings to optional values. Source inspection
suggests building/subbuilding, telephone and postcode-display uses, but these joins are
not yet fully validated.

The name dictionary mask differs between the libraries: `0x3fffff` for 2004 and
`0x1fffff` for 2005. Keep the original 24-bit name codes so flag information is not lost.
Zero name and postcode references occur in the supplied streams; they are absent values,
not valid one-based dictionary rows. The inspector reports their counts.

Surnames are **not stored inside these physical records**. The `ukspos.idx` logical key is
a four-byte **big-endian** residential byte offset; its pointer is a one-based surname
dictionary ID. Consecutive keys delimit surname groups. The 2004 stream has an unindexed
initial record, and the 2005 update stream also has an unindexed initial record. Do not
assign either initial record the first indexed surname.

## Modern c-tree leaves

This layout differs from the PC Format 78 index decoder. Its separate record pointer is
not part of the logical key, and a second compression mode elides trailing fill bytes.

| File header offset | Encoding | Meaning |
|---|---|---|
| 24 | u32 LE | Total leaf key count |
| 42 | u16 LE | Page size (observed 4,096 or 16,384) |
| 57 | byte | Compression flags: `4` for prefix; `12` for prefix and fill run |
| 60 | byte | Fill character for trailing run (space on these sources) |
| 64 | byte | Observed pointer layout `1` |
| 66 | u16 LE | Logical key width, excluding its pointer |
| 72 | u32 LE | First leaf file offset |

Each leaf begins with next/previous leaf offsets (u32 LE), key count and payload size
(u16 LE at offsets 8 and 10). Byte 17 is `1`. Payload starts at byte 18.
Each entry has a u32 LE pointer, one shared-prefix byte, an optional trailing-fill-count
byte when compression bit 8 is set, then `key_width − prefix − fill_count` literal bytes.
Reconstruct the key from the previous logical key's prefix, the literals and the fill run.
The previous-key buffer resets at each page. The implementation verifies leaf links,
page bounds, payload consumption, key order and total count.

Key interpretation is index-specific. In particular, `delr.idx` keys are **little-endian
physical byte offsets**, although their stored byte strings are lexically ordered. Static
inspection traces lookup using the residential source offset. Independently checking all
377,311 supplied deletion keys places every one on a base residential record boundary.
The inspector checks deletion index structure and base record boundary references;
applying deletions and
selecting between base/update results remains outside its claimed behavior.

## Fixed tables

The u16 LE record width is at file offset 44. The u32 LE live record count is at 24.
Do not substitute the counter at 20: the 2005 files may include an extra metadata record
there. An embedded `FirstRecPos\0` resource is followed by a u32 LE data start offset.
The inclusive last-used byte at offset 8 must equal `start + width × count − 1`.
IDs are one based; ID zero has no physical row.

| Table | 2004 width | 2005 base width | 2005 update width |
|---|---:|---:|---:|
| `d2` surname | 24 | 34 | 54 |
| `d3` name | 21 | 32 | 44 |
| `d4` house number | 10 | 12 | 20 |
| `d6new` postcode | 22 | 22 | 22 |

The update and base dictionaries therefore cannot be interchanged. In the base surname
table, a legacy position column is not an adequate substitute for `ukspos.idx`; the 2005
source includes zero/sentinel/past-end values there. The update surname position column
is zero throughout. Use the index, whose references are checked against the physical scan.

## Reproducibility and remaining work

The published aggregate reports are
[2004 SE](../validation/se2004-pcp218-2004-07.json) and
[V10.5 SE](../validation/se2005-pcf175-2005-06.json).
They verify fingerprints, every physical record length, bounds for stored name/postcode/
house references, and every indexed surname range. They do not verify dictionary text
encoding, address assembly, every optional reference or a merged result set.

| Stream | Physical records | Stored name codes | Surname groups | Unindexed prefix bytes |
|---|---:|---:|---:|---:|
| 2004 `uk.dat` | 46,227,729 | 53,936,981 | 858,089 | 9 |
| 2005 `uk.dat` | 25,760,148 | 37,201,126 | 813,668 | 0 |
| 2005 `ukupd.dat` | 1,371,048 | 1,371,048 | 186,846 | 14 |

These are physical storage counts, including absent name codes. They are not counts
of distinct people, households or live search results.

Research used static inspection of the original libraries and whole-file consistency
checks. The 2005 library's original code was recovered from compressed sections for local
analysis. No program was installed or activated. No original binaries, disassemblies,
database records or recovered code bytes are included in the repository. Tests construct
invented bytes independently and exercise both compression modes, page resets, all
residential field conditions and malformed inputs.

Before promoting either source into `SUPPORTED_EDITIONS`:

1. Verify name flags, optional fields and complete address/telephone joins.
2. Validate business records separately; establish which record kinds each API returns.
3. Implement and validate the 2005 deletion/update selection against the original call paths.
4. Preserve unindexed records explicitly and establish totals for the claimed result set.
5. Add invented search/join fixtures, CLI integration and full validation of those semantics.

The experimental Python entry points are `ModernLeafIndex`, `ModernFixedTable`,
`residential_length` and `decode_residence` in `ukinfo_reader.modern`. They are low-level
research APIs, may change as the remaining semantics are established, and should not be
used as evidence that a person appeared in a particular electoral roll.
