# PC Format 78: initial investigation

**Status: identified source; decoding unsupported.** This is not a supported-edition profile.

The earlier UK-Info Disk SE cover disc has a different database layout from the supported
PC Format 94 release. The current reader must not be used to interpret it as SE 4.0.

Source: [Internet Archive, PC FORMAT Issue 78 Cover Disk](https://archive.org/details/pc-format-78).
The local image's size and SHA-1 match `PC_FORMAT_78.iso` in the item's
[file metadata](https://archive.org/metadata/pc-format-78). A local filename with a browser-added
`(1)` suffix does not change that identity.

| File | Bytes | Hash |
|---|---:|---|
| `PC_FORMAT_78.iso` | 518,256,640 | SHA-1 `9d41302896f6e73ead112a4206080aaaa62c00f0` |
| `Uk1.idx` | 109,507,314 | SHA-256 `16beac35d7aa3dfea8a44c848b5f9148e8e11b8ab433325a1f71a94b33095cc7` |
| `Ukdemo.dat` | 329,429,733 | SHA-256 `7016525fe30110ed9dd15d5b0d5b17e29667496b766cd5fc90a16780d0985bcd` |

## Observations

- ISO metadata records mastering in November 1997. This disc is associated with PC Format
  issue 78, January 1998. Its program is named `UK-Info Disk SE.exe`.
- `Uk1.idx` and `Ukdemo.dat` carry September 1997 timestamps. These do not establish the
  directory records' reference dates.
- The index does not begin with the SE 4.0 schema. The data does not begin with its segment
  column encoding either.
- The executable imports functions by ordinal from `ctreestd.dll`. That library exports
  named record/index operations including `ADDREC`, `ADDKEY`, `CREIDX` and `CLSFIL`.
- Auxiliary `d1`–`d8` files are present in various `.dat`/`.idx` combinations. Their relationships
  to name, address and map data have not been established.

These observations establish a format difference. They do not establish the row layout,
record count, dictionary interpretation or correct joins.

## Work required

Trace the executable's database-open, lookup and record-decoding paths; establish how the
auxiliary files are used; then document and implement a separate decoder. Add synthetic
fixtures and full local validation before declaring support. Follow [Adding editions](adding-editions.md).

No original software, data records or database files accompany these notes.
