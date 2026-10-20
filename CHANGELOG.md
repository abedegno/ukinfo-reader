# Changelog

## Unreleased

- Expand required CI with skewed invented search-plan and paginated API matrices;
  retain aggregate timing/work artifacts and add a private read-only plan benchmark.
- Add pool-wait and structured API failure counters, request p95/error/wait charts
  and missing-telemetry detection; export latency histograms with delta temporality.

- Add optional online hosted covering indexes for name/given-name searches with
  postcode filters, avoiding national heap reads on HDD storage; verify index
  creation, compatibility guards and unchanged results in required CI.

- Add opt-in upstream OpenTelemetry request, database pool/query, record-loading
  and address-selection traces; private Collector configuration for Splunk OTLP,
  versioned Terraform dashboard/detector, and invented-data privacy checks in CI.

- Extend adaptive address searches to postcode/nearby/classification anchors, compact
  address candidate sets and source-block budgets rather than a 128-entry cutoff.
- Reuse decoded source blocks and candidate plans within a bounded HTTP request;
  filter small given-name vocabularies before loading records and apply selective
  premises refinements before SQL ordering/grouping.

- Avoid whole-town address expansion for selective name/business searches by checking
  the complete small candidate set against preserved address fields. Retain full
  indexed searches above the threshold, with unchanged grouping and pagination.

- Add bounded public API quotas, explicit Cloudflare tunnel-peer trust and additional
  browser security headers for anonymous hosted deployments.
- Reorganise the README and documentation around web, desktop, Python and preservation
  workflows; update edition scope, feature/release status and public-service guidance.
  Add web-search and feature issue forms.

- Align optional macOS signing with Muesli's certificate/API-key secret names,
  replacing Apple-ID/password authentication and discovering the certificate identity by team.

- Add native archive inventory, edition/storage/completeness details, safe confirmed
  removal, sample-to-full actions and recovery of interrupted address indexing.
- Persist original setup sources for verified restart after interruption and add
  backup guidance with a native data-folder action.
- Prepare an opt-in Developer ID/notarization workflow with temporary credential
  handling, signed embedded Python libraries and gated candidate artifacts.
- Desktop preview version 0.3.0.

- Add desktop postcode-reference setup with a native ZIP chooser, pinned ONSPD
  verification, progress/cancellation and immediate nearby/map availability.
- Add direct ISO/7z preparation, including modern InstallShield tables, fixed output
  paths, fingerprint checks, temporary-file cleanup and a 5 GiB free-space reserve.
- Bundle portable media readers and a source-built cabinet helper with notices;
  desktop preview version 0.2.0.

- Keep research JSON backups within the import size limit, including near-limit collections.

- Add named local research lists, notes/citations, saved-snapshot comparison, safe
  JSON backup/import and CSV export, with native persistence across server-port changes.
- Add optional postcode maps for current results, records and saved entries, labelled
  nearby radius/centres, local ONSPD lookup and opt-in attributed street-map backgrounds.
- Add guided first-launch import, a native app icon, official-source PostgreSQL runtime
  builds and three-platform package CI with checksums, manifests and draft prereleases.

- Add house/building/flat refinements, independent first-name modes, middle initials,
  active-filter removal and zero-result actions. Preserve complete sparse-match paging
  and use postcode-leading ordering for exact-postcode searches.

- Add guarded direct record links, saved-link reopening, exact-premises address pages,
  people/business co-listings, edition-scope comparison, pagination and raw page exports.

- Add optional street, locality, town/city and county indexes, address-only and combined
  searches, edition coverage, cursor paging and a guarded rollback-only index pilot.

- Move the search button below all location controls and support nearby district/sector
  centres, with explicit centre provenance and current/retired postcode selection.

- Add a hash-pinned ONSPD importer, versioned Great Britain postcode reference, spatial
  index and offline nearby search with distance/quality/source provenance.
- Add nearby-area form controls, enriched page/record exports, bounded shared postcode
  caching and postcode-batched radius-only paging without rebuilding source occurrences.

- Remove the broad-prefix vocabulary rejection: keep large expansions inside PostgreSQL
  and page through all matching occurrences with the existing cursors.

- Add all-edition searches, bounded trailing name wildcards, Starts with matching and
  postcode district/sector searches with distinct district/sector boundaries.
- Add edition badges, readable record sections and place types, copyable addresses and
  citations, source comparison, original-record export and readable CSV page exports.
- Add page sizes, navigation above/below results, bookmarkable searches and optional
  conservative grouping of identical listings on the current page.

- Replace the preview JSONB catalogue with schema 2: integer search projections and
  checksummed Zstandard record blocks. Add full-catalogue verification, size statistics
  and a read-only schema-1 repacker into a separate destination.
- Add protocol-2 source-stream checksums and source fingerprint checks after decoding.
- Add desktop folder/demo ingestion with a bundled decoder, progress, cancellation and
  a 5 GiB disk reserve. Add bounded raw NDJSON downloads preserving large integers.
- Import and verify all four complete supported sources (189,064,266 occurrences in
  38.26 GiB); measure mixed-edition HTTP searches and full-field pilot storage
  and HTTP throughput; document the compact layout's saturated-throughput tradeoff.

- Add a Go/PostgreSQL search catalogue, verified Python streaming ingestion, edition-aware
  web UI and an Electron desktop development shell with a local PostgreSQL supervisor.
- Preserve original source fields/warnings, reject incomplete imports, paginate duplicate
  names safely, and bound API queries and page exports.
- Add Compose deployment, synthetic demo data, integration/browser tests and application docs.
- Record the data-free storage spike and its limitations.
- Add backward keyset page navigation and a dated free/low-cost hosting shortlist.
- Verify native page downloads and open the trusted project link in the system browser.
- Reuse vocabulary identities after cache eviction without consuming additional sequence
  values, preserving capacity across repeated imports and later editions.


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
