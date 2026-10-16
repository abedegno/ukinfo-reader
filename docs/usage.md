# Command-line reference

These commands support the exact four releases in the [edition registry](editions.md),
including residential entries from PC Plus 218 (2004 SE) and PC Format 175 (V10.5 SE).
Run `ukinfo-read --help` or `ukinfo-validate --help` for built-in help.

## Search

| Option | Meaning |
|---|---|
| `--data DIRECTORY` | Location of all required edition files; defaults to `extracted` |
| `--surname NAME` | Exact surname, ignoring case |
| `--first-name NAME` | Exact full first name, ignoring case |
| `--sector 'AB10 1'` | Restrict to a postcode sector, ignoring case |
| `--postcode 'AB10 1AL'` | Exact full postcode, ignoring spaces and case |
| `--segment INTEGER` | SE 4.0 only: restrict to a zero-based source segment ID |
| `--limit INTEGER` | Stop after this many matching rows; must be positive |
| `--format jsonl\|csv` | Output format; defaults to JSON Lines |
| `--all` | Explicitly export every entry |
| `--field FIELD` | SE 4.0 only: decode one stored column; requires `--segment` |
| `--schema` | Show edition ID and field definitions |
| `--editions` | List supported releases; needs no database files |
| `--identify` | Confirm the candidate release using SHA-256 fingerprints |
| `--version` | Show the reader version |

Name, postcode, sector and segment filters combine with AND. SE 4.0 scans name columns;
PC Format 78 and the 2004/2005 residential readers use surname indexes when a surname is given. Queries are exact matches,
not substring or fuzzy searches.

Successful searches with no matches produce no records and exit 0. Invalid options exit 2;
source-file and format errors exit 1. Diagnostics go to stderr. Records go to stdout.
CSV writes a header when the first row is found; no matches produce an empty file.

## Export

```sh
ukinfo-read --sector 'AB10 1' --format csv > results.csv
ukinfo-read --surname Smith > results.jsonl
ukinfo-read --all --format csv > directory.csv
```

A full export contains about 28 million rows (PC Format 78) or 46 million rows (SE 4.0)
and about 63 million or 45 million residential occurrences for the supported 2004/2005 sources.
Exports require substantial time and disk space.
`--limit` is useful for inspecting output. The command streams rows and does not assemble
the full export in memory. Output is UTF-8; original spelling/case is preserved.

## SE 4.0 output fields

| Field | Interpretation |
|---|---|
| `segment_id`, `entry_id` | Source row identity, meaningful within this exact edition |
| `household_id` | `segment:address_ordinal`; groups entries at a stored address |
| `surname`, `firstName`, `initial2` | Original name components |
| `phone` | Original telephone string; historical numbering is preserved |
| `houseno` | Numeric house number; 0 is the original display's blank value |
| `building`, `subbuilding` | Additional stored address text, including nonnumeric identifiers |
| `street_1`, `street_2` | Two stored street components, in original display order |
| `locality_1`, `locality_2` | Two stored locality components, in original display order |
| `town`, `county`, `postcode` | Postal place fields and reconstructed full postcode |
| `residency`, `gender` | Original source codes; exact historical meanings remain unverified |
| `organisation`, `business`, `sic`, `sicHeading`, `sicDescription` | Original business fields; their contents may be blank |

Address identifiers are stable for the exact source files, not across releases. They identify
stored addresses and do not establish distinct households or kinship.

## PC Format 78 output fields

Use `--data extracted78` with the same search and export options. `--segment` and `--field`
are rejected for this format. JSON Lines writes unresolved surnames as `null`; CSV writes
an empty surname cell while retaining its source warning and raw code.

| Field | Interpretation |
|---|---|
| `record_offset`, `entry_id` | Source record byte offset and `offset:forename_ordinal` |
| `surname`, `firstName` | Original name text; invalid surname reference becomes `null` |
| `surname_code` | Original three-byte surname code as hex |
| `place_id`, `place`, `place_type` | Original place reference, text and type code |
| `locality`, `town`, `county` | Resolved place hierarchy; missing components are empty |
| `street`, `houseno`, `building` | Stored address text; house identifiers remain strings |
| `postcode`, `postcode_raw` | Formatted postcode and original text; nonstandard values are unformatted |
| `phone_area_code`, `phone_subscriber`, `phone` | Stored numeric components and historical display form |
| `source_warning` | Semicolon-separated anomaly codes, or empty text |

`source_warning` can contain `invalid_surname_reference`, `nonstandard_postcode` or
`place_navigation_mismatch`. Multiple entries in one record share its address; this is not evidence of kinship. The early format
does not emit a `household_id`. Queries exclude the one known mismatched index pointer and
unindexed physical records. A source note is printed to stderr. See [format details and
source defects](formats/pcf78.md#source-defects-and-validation-limits).

## 2004/2005 residential output fields

Use the same search/export options after [extracting all required files](formats/modern-se.md#source-extraction).
The default includes both electoral and residential telephone occurrences, with 2005
telephone deletions applied and indexed updates included. Business databases are not read.

| Field | Interpretation |
|---|---|
| `entry_id`, `source_stream`, `record_offset` | Stable occurrence ID, base/update stream and physical byte offset |
| `entry_type`, `kind` | Electoral/telephone occurrence and original physical kind |
| `surname`, `firstName`, `surname_id` | Stored name text and source surname reference |
| `name_code`, `name_flags` | Original 24-bit name code and retained flag bits |
| `postcode`, `postcode_raw`, `postcode_precision` | Precision-limited display, stored dictionary text and source precision control |
| `street`, `houseno`, `building`, `subbuilding` | Joined address components; house identifiers remain text |
| `building_raw`, `building_reference`, `building_reference_raw` | Stored substring, effective offset after any exact correction, and original offset |
| `place_id`, `place`, `place_type`, `locality`, `town`, `county` | Source place and resolved hierarchy |
| `phone_area_code`, `phone_subscriber`, `phone` | Historical telephone components; populated only for telephone occurrences |
| `source_warning` | Semicolon-separated source anomaly codes |

Warnings include `building_reference_inside_string`, `building_reference_out_of_range`,
`partial_postcode_source`, `unknown_telephone_name`, `unterminated_place_text` and
`invalid_dictionary_text`. An ambiguous building is JSON null (empty CSV cell), with its
raw reference retained. Undefined Windows-1252 bytes become replacement characters and
are flagged. No building replacement or missing name is guessed.

An electoral occurrence and a telephone occurrence can have the same name/address.
Both are retained, since they are separate source occurrences. Some stored names are
initials; `--first-name` matches that stored text exactly and does not expand initials. Electoral rows do not
inherit another contact's telephone number. A source precision control can hide address
components; `postcode_raw` is not proof of a complete postcode for that record.

## Validate

```sh
ukinfo-validate --data extracted --output validation.json
```

The validator checks source hashes first, then every block/record and relationship
appropriate to that format. Progress goes
to stderr; the final JSON report is printed to stdout and saved to `--output`. Reports contain
metadata, hashes and counts, not directory records. See the [SE 4.0](formats/se4.md) and [PC Format 78](formats/pcf78.md) format documents
for the checks and their limits.
