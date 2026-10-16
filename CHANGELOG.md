# Changelog

## 0.4.0

- Support residential search, JSON Lines/CSV export and full validation for the exact
  2004 SE / PC Plus 218 and V10.5 SE / PC Format 175 sources.
- Resolve postcode, street, locality, town, county, house, building and subbuilding fields.
- Compare all 27,958 locality rows in each edition with its original decoder.
- Preserve separate electoral and telephone occurrences; apply 2005 deletions only to
  telephone entries, retaining electoral names in combined records.
- Use separate update dictionaries, restore telephone leading zeroes, apply the 2004
  building-fix index and preserve source postcode precision.
- Validate 63,373,974 residential occurrences for 2004 and 45,198,601 for 2005.
- Flag ambiguous/out-of-range building references and undefined dictionary text.
- Keep business data outside the residential API and retain the structural inspector.

## 0.3.0

- Add experimental structural decoders for the exact 2004 SE / PC Plus 218 and
  V10.5 SE / PC Format 175 sources, outside the normal query edition registry.
- Recover physical residential records, c-tree prefix/fill compression and fixed-table resources.
- Add `ukinfo-inspect` for hash verification and aggregate full-stream structural checks.
- Document source differences and unindexed prefixes; keep unverified joins and update
  semantics explicit. No normal search/export support is claimed for these editions.
- Add invented fixtures covering every residential field condition and corrupt inputs.

## 0.2.0

- Add a separate decoder for UK-Info Disk SE from PC Format 78, January 1998.
- Recover its B-tree indexes, fixed dictionaries, variable records and address/telephone joins.
- Support exact name/postcode/sector searches and JSON Lines/CSV exports for both formats.
- Validate all early-disc physical records and usable index entries, yielding 27,983,545 entries.
- Document source anomalies, preserve unresolved values and exclude unusable index pointers.
- Add invented fixtures for all eight flag combinations, wrapped lengths, prefix pages and joins.
- Keep edition-specific APIs explicit; retain SE 4.0 support and regression tests.

## 0.1.0

- Support UK-Info Disk SE 4.0 from PC Format issue 94, April 1999.
- Decode names, address components, telephone strings, business fields and postcodes.
- Provide exact name/postcode searches, JSON Lines and CSV exports, and a Python API.
- Identify the exact supported source and verify its SHA-256 fingerprints.
- Validate the full disc's blocks, numeric checksums, boundaries and inverse relationships.
- Document the binary format, source provenance and process for adding other editions.
- Include synthetic tests and CI without distributing source databases or original software.
