# UK-Info SE: the 2004 and 2005 sources

## Supported scope

The exact PC Plus 218 (2004 SE) and PC Format 175 (V10.5 SE, 2005) residential
sources support `Database`, `ukinfo-read` searches, JSON Lines/CSV export and
`ukinfo-validate`. Electoral and residential telephone entries are separate source
occurrences. V10.5 SE also supports the separate business streams. The supplied 2004 SE
media omits `bus.dat`; its business word dictionary/index does not supply listings.

Address components and telephone numbers are joined from their source tables. The 2005
reader uses separate base/update dictionaries and applies telephone deletions. Known
source anomalies remain visible through raw references and `source_warning`.

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

Source: [Internet Archive item `pcf-175-dvd-1-06-05.7z`](https://archive.org/details/pcf-175-dvd-1-06-05.7z).
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
cp 'private/2005-disc/Data/Bonus/Bonus 01/InfoDisk/'{uk.dat,d5.dat,lobdesc.dat} private/2005-inspect/
cp private/2005-cab/Data_Files/{d1.dat,d2.dat,d3.dat,d4.dat,d6new.dat,d7.dat,d8.dat,ukspos.idx,bus.dat,dd.dat,dbsic.dat} private/2005-inspect/
cp private/2005-cab/Update_data/{ukupd.dat,d2upd.dat,d3upd.dat,d4upd.dat,d5upd.dat,d6newupd.dat,d8upd.dat,uksposupd.idx,delr.idx,busupd.dat,ddupd.dat,dbsicupd.dat,lobdescupd.dat,delb.idx} private/2005-inspect/
ukinfo-read --data private/2005-inspect --identify
ukinfo-read --data private/2005-inspect --surname Smith --limit 20
ukinfo-validate --data private/2005-inspect --output private/2005-report.json
```

### PC Plus 218 (2004)

Source: [Internet Archive item `pcp-218-dvd`](https://archive.org/details/pcp-218-dvd).
Wait for the browser download to finish and the `.crdownload` suffix to disappear.
The completed supplied ISO is named `PCP218_DVD.iso`. Its UK-Info payload is under
`Software/Full/UK-Info/`. The cabinet puts the tables in `Data_Files_Compress`;
the extracted files themselves are directly readable.

```sh
7zz x PCP218_DVD.iso -oprivate/2004-disc 'Software/Full/UK-Info/*'
unshield -d private/2004-cab x private/2004-disc/Software/Full/UK-Info/data1.cab
mkdir -p private/2004-inspect
cp private/2004-disc/Software/Full/UK-Info/uk.dat private/2004-inspect/
cp private/2004-cab/Data_Files_Compress/{d1.dat,d2.dat,d3.dat,d4.dat,d5.dat,d6new.dat,d7.dat,d8.dat,hfix.idx,ukspos.idx} private/2004-inspect/
ukinfo-read --data private/2004-inspect --identify
ukinfo-read --data private/2004-inspect --surname Smith --limit 20
ukinfo-validate --data private/2004-inspect --output private/2004-report.json
```

The `cp` examples use POSIX shells with brace expansion. On Windows, copy the listed
files into one directory and run the same reader/validator commands. They use only the
Python standard library and needs no original executable or DLL.

The exact query file fingerprints are in
[`modern_profiles.py`](../../src/ukinfo_reader/modern_profiles.py). `ukinfo-inspect` remains
available for a smaller structural-only profile; it does not replace full validation. Media fingerprints
are recorded in [modern source media](../validation/modern-source-media.json). The local 2004 ISO and both 2005 archives were independently matched by size and SHA-1
to the items' [2004 metadata](https://archive.org/metadata/pcp-218-dvd) and
[2005 metadata](https://archive.org/metadata/pcf-175-dvd-1-06-05.7z).

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
fields. The joined reader interprets `flag80` as the building string offset, `flag40` as
the subbuilding ID, `kind25` as the associated telephone name code, `kind16` as the
subscriber number, `pc20` as the area code and `pc80` as the postcode precision control.

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
The joined reader applies these keys only to the telephone component of base records.
It then reads the indexed update stream with the update dictionaries. The lower-level
inspector reports structure and boundary checks only.

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

## Entry selection and updates

| Physical kind | Electoral occurrences | Telephone occurrences |
|---|---|---|
| 0 | Each primary stored name | None |
| 3 | None | Each primary stored name, unless deleted |
| 4 | Each primary stored name | One contact using the first primary name, unless deleted |
| 5 | Each primary stored name | One contact using `optional_kind25`, unless deleted |

These are the kinds observed on the verified sources. Other kinds fail with `FormatError`.
An offset in `delr.idx` suppresses the telephone occurrence, **not** the electoral names in
that physical record. All 377,311 deletions target kinds 3, 4 or 5. Updates are read from
their own surname, name, house, postcode, building and subbuilding dictionaries; streets
and the place hierarchy are shared. No fuzzy matching or deduplication is used. An entry
ID includes its stream, byte offset and source occurrence ordinal.

The 2004 source also supplies `hfix.idx`. Its keys are big-endian physical offsets and
its values override building offsets. The reader applies an exact matching correction
when a nonzero building reference is present, following the original lookup. Keys that
are never reached in this source do not create records or cause guessed corrections.
The full scan applies 16,243 corrections that change the stored building reference.

Telephone encoding is edition-specific. In 2004, values strictly between 10,000,000
and 11,000,000 restore one leading zero. The 2005 encoding supports multiple leading
zeroes using the million bands above 10,000,000. Both implementations were compared
with their original formatters for 2,009 generated/boundary cases each, with zero
differences; see [the comparison evidence](../validation/modern-telephone-reference-comparison.json).
The reader retains the historical area/subscriber form. Electoral occurrences do not
inherit a telephone number from another contact stored in the same physical record.
Unknown telephone-name code `0xffffff` produces an empty name with a warning.

## Address model and PAF

Royal Mail's [PAF Programmer’s Guide, Section 1](https://www.poweredbypaf.com/wp-content/uploads/2025/01/Latest-Programmers_guide_Edition-7-Version-6-2.pdf)
provided the component model: premises, thoroughfare, locality, post town and postcode.
This disc uses its own compressed storage and locality IDs. It is not a raw PAF Mainfile;
PAF rules alone do not establish its binary links or update selection.

The postcode row has seven text bytes, a u16 LE place ID at byte 7 and a u24 LE street
string offset at byte 9. `d7.dat` contains the street strings. `d1.dat` has 56-byte place
rows: text in bytes 0–29, type in byte 30, county reference at byte 36 and town reference
at byte 40 (u32 LE). A village resolves its town first, then uses that town's county.
The original functions also contain a few named/ID-based overrides. Both editions' complete
27,958-row place tables were independently compared with their original decoder functions
in local x86 emulation: **zero differences with text bounded to the declared fields**.
See [the comparison evidence](../validation/modern-locality-reference-comparison.json).

The reader exports components separately; it does not claim certified PAF mailing-label
formatting. Historical county text is retained. Full-length place names are bounded at
30 bytes, preventing the original function's read into adjacent metadata; affected joins
are flagged `unterminated_place_text`.

The source precision byte is also retained. When neither zero nor 10 it shortens the displayed
postcode and clears the street; values below 10 additionally clear locality/town/county
text. `postcode_raw` preserves the dictionary value for research. Exact full-postcode
filters do not treat a partial code as a verified full code. The original SE application's
separate display cap on first names and postcodes is not reproduced: the reader exposes
stored text subject to the source precision rule.

## Source anomalies and validation

Source building references can point inside a NUL-delimited string or beyond its file.
An interior reference is ambiguous: it may be a suffix reference or inconsistent source
metadata. The reader preserves its substring as `building_raw`, its numeric reference
as `building_reference`, and sets `building` to null with a warning. Out-of-range
references retain the number and set both text fields to null. No replacement is guessed.

The 2004 scan finds 981,500 physical records with interior building references after
corrections, 138,198 with postcode precision controls and 17,465 unknown telephone names.
The 2005 base has 4,272 out-of-range building references and five live telephone entries
with unknown names. Its base dictionaries also contain six surname rows and 15 name
rows with bytes undefined in Windows-1252; replacement characters are flagged if emitted.
These source defects do not prevent other fields or unaffected records from being read.

Residential reports from version 0.4: [2004 residential](../validation/se2004-pcp218-2004-07-residential.json)
and [2005 residential](../validation/se2005-pcf175-2005-06-residential.json).
Earlier [2004](../validation/se2004-pcp218-2004-07.json) and
[2005](../validation/se2005-pcf175-2005-06.json) structural-only reports record the state
of the 0.3.0 investigation; their former query-support limitations are historical.

| Edition | Electoral entries | Telephone entries | Total exported occurrences |
|---|---:|---:|---:|
| 2004 SE | 53,936,980 | 9,436,994 | 63,373,974 |
| V10.5 SE (base plus updates) | 32,062,644 | 13,135,957 | 45,198,601 |

These are source occurrences, not counts of distinct people or households. The one
unindexed 2004 record and the one unindexed 2005 update record are excluded. Full
validation checks every physical record, surname range, deletion, name/house/subbuilding
reference and postcode/street/place join, verifies all required hashes, and reports the
source defects above. Business streams are additionally checked as described below.

Research used static inspection and local emulation of the original decoder routines.
The 2005 library's code was recovered from compressed sections for local analysis. No
installer or activation was run. No original binaries, recovered machine code,
disassemblies or directory records are distributed here. Tests use invented fixtures,
including combined electoral/telephone records, telephone-only deletions, update-specific
dictionaries, building corrections and ambiguous references, precision controls and export.

## V10.5 business layout and selection

The headerless `bus.dat` and `busupd.dat` streams use little-endian fields.
The business constructor and length callback in the original `icd_10.dll` establish
this separate layout; it must not be decoded as a residential record.

| Offset | Content |
|---|---|
| 0 | Optional-field mask `F` |
| 1 | Word count minus one in low four bits; source kind in bits 4–5; building high bit in bit 6; precision-present bit 7 |
| 2–4 | Postcode ID in low 21 bits; bit 7 of byte 4 supplies house ID bit 16 |
| 5–6 | Low 16 bits of house ID |
| 7 onward | `3 × word_count` bytes of one-based word references |

Words reference byte offsets **plus one** in `dd.dat`/`ddupd.dat`; their stored order
forms the organisation name. There are 1–16 words. Optional values follow the words:

| Mask | Bytes | Meaning |
|---|---:|---|
| `F & 1` | 3 | Activity text reference (low 24 bits) |
| `F & 2` | 2 | SIC dictionary ID |
| `F & 4` | 3 | Building offset; byte 1 bit 6 adds bit 24 |
| `F & 8` | 2 | Subbuilding ID |
| `F & 16` | 3 | Telephone subscriber |
| `F & 32` | 2 | Telephone area |
| `F & 64` | 3 | Fax area |
| `F & 128` | 3 | Fax subscriber |
| byte 1 bit 7 | 1 | Postcode precision |

Base and update streams use their own house, postcode, building, subbuilding, word,
activity and SIC dictionaries; street/place dictionaries are shared. `dbsic*.dat`
is a fixed table of five-byte SIC text. `lobdesc*.dat` contains NUL-terminated activity
text. Industry references need the bounded recovery/ambiguity rules documented in
[the output reference](../usage.md#v105-se-business-output): the base file exceeds 16 MiB.
No ambiguous candidate is emitted as a resolved industry.

`delb.idx` keys are four-byte little-endian **base business byte offsets**. They remove
the entire business occurrence. Unlike residential `delr.idx`, there is no electoral
component to retain. Both observed business source kinds (1 and 2) are emitted; they are
not interpreted as dates. Update entries follow surviving base entries, without fuzzy
deduplication. IDs include the business namespace to avoid residential offset collisions.
The original business-selection routine checks `delb` before producing a result.

`busw1*.idx` indexes name words, while the CAWS files are auxiliary word-search records
and postings: they do not supply another stream of joined business listings. The reader
scans business records, so these acceleration files are not query dependencies.

The additional required files are `bus.dat`, `busupd.dat`, `dd.dat`, `ddupd.dat`,
`dbsic.dat`, `dbsicupd.dat`, `lobdesc.dat`, `lobdescupd.dat` and `delb.idx`. Existing
0.4 residential extraction directories must add them before V10.5 identification succeeds.
Every source byte boundary, name reference, dictionary join and deletion target is checked
by `ukinfo-validate`, including joins on entries suppressed by the deletion index.
Reports contain counts and fingerprints only; original media/data are not distributed.

### Whole-source business results

| Stream | Physical records | Deleted | Exported entries |
|---|---:|---:|---:|
| Base | 5,310,345 | 188,131 | 5,122,214 |
| Update | 1,030,082 | 0 | 1,030,082 |
| Combined | 6,340,427 | 188,131 | **6,152,296** |

All word references resolve to string boundaries, all address/SIC references are within
their dictionaries, and all deletion keys reach physical business record boundaries.
Among all physical base records, 121,513 industry references have a unique upper-range
candidate and 9,101 have multiple candidates. The latter remain null and flagged.
Existing place-text warnings affect 10,277 base and 2,005 update business records.
These warning counts include deleted records; exported counts exclude them.

The business length callback was compared on 1,536 synthetic combinations of optional
flags, word counts and precision. The original business telephone formatter was compared
on 2,006 numeric cases, with zero differences. See
[the data-free reference comparison](../validation/modern-business-reference-comparison.json).
This checks those routines, not every original-program display decision. Activity wrap
recovery is an explicit reader inference. The full aggregate report is
[the complete 2005 validation](../validation/se2005-pcf175-2005-06-complete.json).
