# UK-Info 4.0 binary format

This describes the two files on this particular disc. All fixed-width integers are little
endian. Strings are decoded as Windows-1252. The implementation is in `src/ukinfo_reader/reader.py` and `src/ukinfo_reader/codecs.py`.
The reference reader is `extracted/installer/Program_DLLs/ukiddll.dll`, with image base
`0x400000`. Addresses below are virtual addresses in that library.
Its SHA-256 is `a4141029865d6ad77fd73da2a210e4499b98b8c23d42be51b1df9701b185b51b`; this identifies the exact analysis reference.

## Index file

At offset 0, a schema lists fields. Each definition consists of:

- `uint16` field ID;
- zero-terminated ASCII field name;
- six `uint16` metadata words;
- two bytes (`01` on this disc);
- four more `uint16` metadata words.

A standalone `uint16 0xffff` ends the schema. The first metadata word is the field type,
the second the record level. Other metadata describes original search/output behavior;
it is preserved by `ukinfo-read --schema`, without assigning unverified meanings.

The 26 fields are segment, postcode, postcodesToLocalities, county, town, locality, street,
localitiesToAddresses, houseno, building, subbuilding, addressesToNames, phone, surname,
firstName, initial2, residency, gender, organisation, business, sic, sicHeading,
sicDescription, addressesToPostcodes, namesToPostcodes, localitiesToPostcodes.

A block directory starts at byte **923**: a `uint16` tag 0 and `uint32` section length
**1,989,526**, including its six-byte header. The payload is **248,690** pairs of
`uint32 data_offset, uint32 byte_length`, ordered segment first, then field ID.
There are **9,565** segments with 26 entries each. Offsets address `ukid.dat`.
Zero-length entries are possible.

The next headers are tag 6/value 9,566 and tag 7/value 0. Their values are scalar metadata,
not section lengths. The reference initialization subtracts one from tag 6. Tag 7 controls
postcode granularity; this disc groups records by postcode sector.

The remainder contains tag-1 field index sections: `uint16 tag, uint32 length,
uint16 field_id`, followed by nested sections. Section lengths include the six-byte header.
Nested sections include 8, 9 and 2 (original search indexes) and optional 12 (common strings).
Common strings start with a `uint32` count followed by zero-terminated strings. The surname,
firstName and business fields have 8,685, 9,150 and 2,390 common strings respectively.
The parser reads common strings and locates the other sections but does not reproduce the
original index-based query algorithm. Whole-disc queries scan decoded columns.

Initialization/layout reference: `0x403898`, especially `0x403fa4` and `0x40410c`.
Block lookup: `0x404988`.

## Variable-length integers

Seven-bit payload groups are accumulated most significant first. In the usual mode,
bit 7 marks the **last** byte, the opposite of many modern encodings. Examples:
`80` = 0; `81` = 1; `01 80` = 128.

The original implementation wraps the accumulator at 32 bits. Some actual house-number
deltas use much longer sequences than a minimal 32-bit encoding. Preserving this overflow
and arithmetic right shifts is essential for recovering the correct values.

For packed character counts, all count bytes have bit 7 set, and the following first
alphabet byte has bit 7 clear. This is the reference reader's mode 2.

Reference: `0x402a4c` (also duplicated at `0x4011fc`).

## Integer columns

A numeric array has `uint32 count`, `uint16 encoding`, `uint32 checksum`, then payload.
The checksum is the sum of the final decoded values modulo 2^32.

| Encoding | Values | Delta accumulation | Repeat compression |
|---|---|---|---|
| 0, 1, 2 | byte, uint16, varint | yes | no |
| 3, 4, 5 | byte, uint16, varint | yes | yes |
| 6, 7, 8 | byte, uint16, varint | no | yes |
| 9, 10, 11 | byte, uint16, varint | no | no |
| 12 | varint dictionary | no | no |

Byte repeat tokens >200 repeat the previous value `token-200` times. Word repeat tokens
>65000 repeat it `token-65000` times. Varint repeat tokens use the low bit: odd tokens repeat
the previous value `token>>1` times; even tokens are values shifted right one using the
original signed 32-bit arithmetic. Repetition precedes delta accumulation.

Encoding 12 begins with varint `dictionary_count_minus_one`, followed by that many plus one
varint dictionary values. Output tokens <128 reference that dictionary; other tokens encode
literal `token-128`.

Reference: `0x404ca8`; byte/word/varint decoders `0x404a6c`, `0x404ae4`, `0x404b68`;
dictionary encoding `0x404c00`. Composite numeric data starts with tag 10 and a section
length, then two numeric arrays (`0x404e94`); this must be distinguished from a plain array
whose count happens to equal ten.

## String columns

A string column starts with a varint dictionary count and one byte indicating kind 4 or 11.
Each string ends in zero. A leading byte 1–31 reuses that many bytes from the preceding
string, followed by the remaining suffix. Other strings are literal, including the empty string.

For columns with per-record references, a numeric array follows the local dictionary.
Kind 4 references the local strings. Kind 11 references the common strings for the field
first, followed by the local dictionary. The segment field can contain an unreferenced
string dictionary or a dictionary and references.

Locality and street contain two equally sized halves: component 1 for every locality row,
then component 2 for every locality row. They are not interleaved.

Reference: `0x404fe4`, with two-component output at `0x4057f1`.

## Character columns

Type-3 fields contain either one byte per record or a packed representation. A high first
bit selects packed mode: mode-2 varint count, four-byte alphabet, then two bits per record,
least significant pair first in each byte. The stored payload has `floor(count/4)+1` bytes,
including an extra byte when count is divisible by four.

Reference: `0x4052d4`.

## Postcodes

The segment key uses padded outward codes, e.g. `AL03 6`, `B 01 1`, `WC01A1`.
The displayed sectors are `AL3 6`, `B1 1`, `WC1A 1`.

The postcode column is a sparse bitmap over 400 possible letter pairs. The alphabet is
`ABDEFGHJLNPQRSTUWXYZ`, so bit index `i` represents letters `alphabet[i//20]` and
`alphabet[i%20]`. Existing postcodes occur in ascending bit order. Add those letters to
the displayed sector to reconstruct the full postcode.

Bitmap commands use ordinary varints. Opcode 0 ends the stream; >4 sets absolute bit
`opcode-5`. Opcodes 1–4 each consume a zero-terminated command sequence with a cursor
initially zero:

- 1: read length and start delta; advance cursor by delta and set that byte range to 0xff.
- 2: advance bit cursor by `value-1` and set that bit.
- 3: advance byte cursor by `value-1`; copy a zero-terminated literal byte string there.
- 4: advance byte cursor by `value-1`; copy one literal byte there.

For commands 1 and 3, the next delta is relative to the previous **start**, not its end.

Reference: bitmap `0x4031fc`; letter pairs `0x4026fc`; formatting `0x40264c`.

## Joins

Record levels: 0 = postcode, 1 = locality, 2 = address, 3 = name/directory entry.

`postcodesToLocalities`, `localitiesToAddresses` and `addressesToNames` are boundary arrays.
For parent row `i`, child ordinals lie in `[boundaries[i], boundaries[i+1])`. Boundary arrays
include a final sentinel. `addressesToPostcodes` and `namesToPostcodes` provide inverse
postcode ordinals, also with a sentinel; `localitiesToPostcodes` has one ordinal per locality.

The validator checks every decoded numeric checksum, all expected column lengths, every
parent boundary, inverse postcode agreement at each level, and contiguous coverage of the
entire data file by the block directory. It does not equate directory entries to unique people
or infer historical meanings for the residency codes.
