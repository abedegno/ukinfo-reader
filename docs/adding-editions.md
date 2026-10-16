# Adding support for another edition

Currently, only **SE 4.0 from PC Format 94, April 1999** is supported. The reader rejects
unknown database sizes and verifies exact source hashes on request. This prevents a familiar
filename or a partly readable table from being mistaken for established compatibility.

## 1. Establish the source

Record the product/edition, distribution, filenames, byte sizes and hashes. Include an archive
item page if available. Hash the original disc and each relevant database file. Retain source
material locally under an ignored directory. File timestamps are observations, not evidence
of an electoral-roll reference date.

Record both successful and uncertain identifications in `docs/editions.md`. Keep unimplemented
releases explicitly marked **unsupported**. Do not add them to `SUPPORTED_EDITIONS` yet.

## 2. Compare the format

Inspect the schema, record levels, block directory, codecs, string dictionaries, postcode
representation and relationships. Compare these against [the SE 4.0 format](formats/se4.md).
Use the corresponding release's reader for evidence when the format differs. Record addresses
relative to the correct binary and its identity; SE 4.0 addresses do not apply to another release.

Do not assume older/newer releases or Standard/Pro variants share a format. A nonempty name
column is insufficient: addresses must be joined correctly and all numeric encodings checked.
Preserve unverified values without inventing their meaning.

## 3. Implement the decoder

The implementation is organized as:

| Module | Responsibility |
|---|---|
| `editions.py` | Exact release identities, candidate selection and SHA-256 verification |
| `codecs.py` | SE 4.0 binary primitives with small synthetic regression tests |
| `reader.py` | Memory mapping, schema/block reading, columns, records and relationship checks |
| `cli.py` | User-facing search and export |
| `validation.py` | Whole-disc validation and a metadata-only report |

For a release with the same verified structure, reuse the decoder and add a distinct edition
profile. For a different layout, introduce a separate reader module and dispatch explicitly
by edition; avoid speculative conditionals based on dates or filenames. Share binary codecs
only when their behavior is verified to be the same. Keep edition-specific field names and
uncertain semantics documented.

The current registry intentionally has one decoder family; a general multi-format plugin
system has not been implemented in advance of evidence from another release.

## 4. Test with invented fixtures

Extend `tests/fixtures.py` or add an edition-specific fixture generator. Cover the behavior
that differs: headers, encodings, signed overflow, boundaries, dictionaries, address component
order and postcode reconstruction. Include malformed inputs and unknown-release handling.
All public tests must work without source databases. Never derive fixture records from real
people, even when they appear on the source disc.

## 5. Validate the complete source

Require complete block coverage, all available checksums, column lengths, parent/child
boundaries and inverse relationships. Add format-specific checks where needed. Compare some
output to the original application's output if feasible, and state explicitly if this was
not done. Do not advertise counts as unique people or households without supporting evidence.

Run the new decoder locally across the whole release. Save its data-free JSON report under
`docs/validation/<edition-id>.json`, including hashes, counts, reader version and checks run.
Run the existing SE 4.0 regression tests and full validation after shared decoder changes.

## 6. Declare support

Add the validated profile to `SUPPORTED_EDITIONS`, update the README support summary,
`docs/editions.md`, format documentation and changelog. Include a reproducible extraction
command using the exact filenames. Ensure `--identify`, search, export and validation all
work with the release. The support claim should name a specific release, not a whole year
or every product with a similar name.
