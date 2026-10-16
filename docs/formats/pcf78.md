# UK-Info Disk SE: PC Format 78 format

This decoder applies to **PC Format issue 78, January 1998**, edition ID
`se-pcf78-1998-01`. The image was mastered in November 1997; database file timestamps
are September 1997. Neither timestamp establishes the records' reference date.
This is a separate format from [SE 4.0](se4.md).

## Files and joins

| File | Role | Record width | Source ID base |
|---|---|---:|---:|
| `Uk1.idx` | Surname/place keys and absolute main-file offsets | B-tree | — |
| `Ukdemo.dat` | Variable records, shared surname/address and one or more forenames | 11–42 bytes | Absolute byte offset |
| `d1.dat` | Places, town/county relationships and source navigation offsets | 44 | 1 |
| `d2.dat` | Surnames | 19 | 0 |
| `d3.dat` | Full forenames | 16 | 0 |
| `d4.dat` | House identifiers, including text | 10 | 1 |
| `d5.idx` | Building names in index order | B-tree | 1-based ordinal |
| `d6.dat` | Postcode and street reference | 12 | 0 |
| `d7.dat` | Street text | 32 | 0 |

The other supplied auxiliary indexes are not required for these joins. `d8.dat` contains
telephone exchange lookup information used by the original search interface; telephone
numbers in main records already contain their numeric area code and subscriber number.
Map rendering and the original Windows search UI are outside this reader's scope.

## Fixed dictionaries

All integer header fields here are little endian. Record width is a `u16` at byte 44;
record count is a `u32` at byte 20. Read the reserved-header boundary `h` from `u16` at
byte 12. The first record starts at `ceil((h + 32) / width) * width`.
The `u32` at byte 8 equals the last byte of the final live fixed record. File padding
continues after that byte. Source IDs must account for each dictionary's base above.

Strings are NUL-terminated Windows-1252, bounded by their fixed field. In `d1`, two `u16`
references precede a 30-byte place name. Type at bytes 34–35 is `C`, `TC`, `TO`, `VC` or
`VO`. `T` denotes town, `V` a subordinate locality; `C` in the second position indicates
a stored county relationship. For a village, the second `u16` points to its town; the town's
first `u16` supplies its county when applicable. The final `u32` is a main-file navigation
offset. County rows also mark ranges and are not themselves directory entry locations.
Other place metadata is retained in the source and is not interpreted as demographic data.

In `d6`, bytes 0–2 encode a zero-based street ID. Bytes 3–9 contain the postcode text;
bytes 10–11 are auxiliary codes and **must not be appended to the postcode**. Their exact
semantics are unverified. Normal postcode output inserts a space before the final three
characters; `postcode_raw` preserves the original text. Nonstandard source values are
preserved without postcode formatting and flagged.

## Prefix-compressed c-tree indexes

The files use 1,024-byte pages. Header fields:

| Offset | Type | Meaning |
|---:|---|---|
| 24 | `u32le` | Total keys |
| 42 | `u16le` | Page size, 1,024 |
| 66 | `u16le` | Full key width: 9 for `Uk1`, 34 for `d5` |
| 72 | `u32le` | First leaf page offset |

Each leaf starts with next/previous page offsets (`u32le` each), key count (`u16le` at 8),
and payload byte count (`u16le` at 10). Byte 17 is 1. Payload starts at byte 18.
Each entry has a one-byte shared-prefix length and the remaining literal key bytes.
The previous key starts as all zero bytes at each page boundary. Reconstruct the **full**
key before splitting its final four bytes into a **big-endian** record pointer.

`Uk1` has five logical key bytes: surname code (3), then place code (2). Its pointer is an
absolute `Ukdemo.dat` offset. Equal logical keys are allowed; pointers must be unique.
`d5` has 30 bytes of building-name text followed by a pointer. Main records refer to the
**ordinal in the sorted leaf sequence**, not that pointer. Those values sometimes differ.

The reader follows leaves directly and does not implement internal-node mutation or the
c-tree runtime. A small page directory enables surname searches without scanning all keys.
All leaf links, counts, prefixes, payload lengths and logical key order are validated.

## Main records

`Ukdemo.dat` begins with the 12-byte signature `*** R.A. ***`. The next byte starts the
first physical record; there are no delimiters between subsequent records.

| Bytes | Meaning |
|---|---|
| 0–1 | Place code |
| 2–4 | Surname code |
| 5–7 | Little-endian primary forename ID in the low 19 bits; record length in the high 5 |
| 8–10 | Big-endian postcode ID in the low 21 bits; optional-field flags in the high 3 |

Length is `byte7 >> 3`. If it is 1–10, add 32. Zero is invalid. After the 11-byte header,
optional fields occur in this order:

1. Flag 4: area code (`u16le`) and subscriber number (`u24le`), five bytes total.
2. Flag 2: house code, two bytes.
3. Flag 1: building ordinal (`u24le`), three bytes.
4. All remaining bytes: additional forename IDs (`u24le` each).

Every forename produces an entry with the same stored surname, address and telephone.
`record_offset` identifies this shared record; `entry_id` is `offset:forename_ordinal`.
This grouping does not establish kinship or a unique household. No cross-edition identity
is implied, and the early reader does not synthesize an SE 4.0 `household_id`.

For a shifted base-128 code, fold bytes as `value = value * 128 + byte - 48`.
Surname and street IDs use three bytes with no increment. A house ID uses two bytes,
clamps negative values to zero and then adds one. Place codes instead use base **155**:
clamp negatives to zero; otherwise increment values below **14,550**, leaving larger
values unchanged. This unusual boundary is present in the original decoder.

Telephone output follows the stored integers: `0{area} {subscriber}` when area is nonzero.
It preserves historical numbering, without guessing missing subscriber padding or applying
modern renumbering. Both numeric components are also exposed separately.

## Source defects and validation limits

The [complete report](../validation/se-pcf78-1998-01.json) distinguishes physical records,
index keys and exported entries. These are not counts of unique people.

- One key, at zero-based index ordinal 3,402,050, points to offset 150,994,689 but its key
  disagrees with that record's header. It is excluded, never redirected to a guessed record.
- 113 physical records are not reached by usable main-index keys. Their historical status
  is unknown; default exports exclude them. `decode_record(offset)` permits explicit local
  inspection of a known physical record, without claiming index membership.
- Three indexed records carry surname bytes `00 2c 22`, which decode outside the surname
  dictionary. Their surname is `null`, with `surname_code` and `source_warning` retained.
- 37 postcode dictionary values are nonstandard; 13 indexed records (16 entries) use such
  values. They remain visible and carry `nonstandard_postcode`.
- Place-navigation row 19,899 points to offset 259,849,361, where records instead decode to
  place ID 19,898. Their decoded place references are retained and carry
  `place_navigation_mismatch`; no inferred correction is applied. This affects 275 indexed
  records, containing 433 entries.

Validation confirms every physical record boundary, optional-field length and dictionary
reference, with the explicitly reported surname exception. Every usable index pointer is
unique, agrees with the pointed-to header and lands on a physical boundary. All dictionary
text, postcode/street references, building ordinals and place relationships are checked.
The nonzero navigation offsets in non-county place rows provide an independent cross-check
of place-code conversion, with the single reported mismatch.
There are no SE 4.0 numeric checksums in these records. Output has not been compared with
a running original Windows application, and structural validation cannot establish the
accuracy of the historical information.

## Reconstruction evidence

The inspected binaries are identified by SHA-256:

| Binary | Bytes | SHA-256 |
|---|---:|---|
| `UK-Info Disk SE.exe` | 222,720 | `0044eb57084c9ba8a774110014982a352545b5001a41ef7426557bb2e24e5b35` |
| `ctreestd.dll` | 249,856 | `16a414839034a2de83109fad9fd999bbc7e9b050eacbf47984504ffb0693a87c` |

Static inspection of `UK-Info Disk SE.exe` (image base `0x400000`) established the record
routine at `0x408b60`, fixed-record retrieval at `0x408ea0`, place conversion at `0x40ad20`,
house conversion at `0x40ad50`, three-byte conversion at `0x40ad70` and display joins around
`0x40b2f0`. In particular, its building lookup passes the stored ordinal minus one to
`ORDKEY`. `ctreestd.dll` has image base `0x10000000`; its `ORDKEY` entry is `0x1000cb2e`.
The [FairCom function reference](https://faircom.helpjuice.com/en_US/db-function-descriptions/db-function-descriptions-g)
provides context for ordinal-key lookup; the Python decoder implements the observed bytes
independently and does not distribute library code or disassemblies.

Synthetic fixtures independently encode all eight flag combinations, multiple forenames,
wrapped lengths, prefix-compressed pages, ordinal/pointer differences, hierarchy joins,
source anomalies and malformed inputs. Full-disc validation additionally checks the exact
source fingerprints recorded in [Supported editions](../editions.md).
