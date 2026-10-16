# Command-line reference

These commands currently support **UK-Info Disk SE 4.0 from PC Format 94, April 1999**.
Run `ukinfo-read --help` or `ukinfo-validate --help` for built-in help.

## Search

| Option | Meaning |
|---|---|
| `--data DIRECTORY` | Location of `ukid.idx` and `ukid.dat`; defaults to `extracted` |
| `--surname NAME` | Exact surname, ignoring case |
| `--first-name NAME` | Exact full first name, ignoring case |
| `--sector 'AB10 1'` | Restrict to a postcode sector, ignoring case |
| `--postcode 'AB10 1AL'` | Exact full postcode, ignoring spaces and case |
| `--segment INTEGER` | Restrict to a zero-based source segment ID |
| `--limit INTEGER` | Stop after this many matching rows; must be positive |
| `--format jsonl\|csv` | Output format; defaults to JSON Lines |
| `--all` | Explicitly export every entry |
| `--field FIELD` | Decode one stored column; requires `--segment` |
| `--schema` | Show edition ID and field definitions |
| `--editions` | List supported releases; needs no database files |
| `--identify` | Confirm the candidate release using SHA-256 fingerprints |
| `--version` | Show the reader version |

Name, postcode, sector and segment filters combine with AND. A name query without a sector
scans the entire disc. Queries are exact matches, not substring or fuzzy searches.

Successful searches with no matches produce no records and exit 0. Invalid options exit 2;
source-file and format errors exit 1. Diagnostics go to stderr. Records go to stdout.
CSV writes a header when the first row is found; no matches produce an empty file.

## Export

```sh
ukinfo-read --sector 'AB10 1' --format csv > results.csv
ukinfo-read --surname Smith > results.jsonl
ukinfo-read --all --format csv > directory.csv
```

A full export contains over 46 million rows and requires substantial time and disk space.
`--limit` is useful for inspecting output. The command streams rows and does not assemble
the full export in memory. Output is UTF-8; original spelling/case is preserved.

## Output fields

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

## Validate

```sh
ukinfo-validate --data extracted --output validation.json
```

The validator checks source hashes first, then every block and relationship. Progress goes
to stderr; the final JSON report is printed to stdout and saved to `--output`. Reports contain
metadata, hashes and counts, not directory records. See [the format document](formats/se4.md)
for the checks and their limits.
