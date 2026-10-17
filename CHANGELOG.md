# Changelog

## Unreleased

- Replace the preview JSONB catalogue with schema 2: integer search projections and
  checksummed Zstandard record blocks. Add full-catalogue verification, size statistics
  and a read-only schema-1 repacker into a separate destination.
- Add protocol-2 source-stream checksums and source fingerprint checks after decoding.
- Add desktop folder/demo ingestion with a bundled decoder, progress, cancellation and
  a 5 GiB disk reserve. Add bounded raw NDJSON downloads preserving large integers.
- Import the complete 51,350,897-entry 2005 edition and measure full-field pilot storage
  and HTTP throughput; document the compact layout's saturated-throughput tradeoff.

- Add a Go/PostgreSQL search catalogue, verified Python streaming ingestion, edition-aware
  web UI and an Electron desktop development shell with a local PostgreSQL supervisor.
- Preserve original source fields/warnings, reject incomplete imports, paginate duplicate
  names safely, and bound API queries and page exports.
- Add Compose deployment, synthetic demo data, integration/browser tests and application docs.
- Record the data-free storage spike and its limitations.
- Add backward keyset page navigation and a dated free/low-cost hosting shortlist.


## 0.5.0

- Include V10.5 SE businesses in normal search and JSON Lines/CSV export.
- Add exact organisation filters, entry-type filters and a business Python API.
- Join ordered business-name words, addresses, activity/SIC descriptions, telephones and fax numbers.
- Apply business deletions and separate update dictionaries; retain stable source occurrence IDs.
- Flag ambiguous activity references and identify unique 24-bit wrapped dictionary candidates.
- Expand source fingerprints and whole-source validation to include nine business files.
- Document that the supplied 2004 SE media omits the separate business record payload.

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
