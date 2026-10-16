# Changelog

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
