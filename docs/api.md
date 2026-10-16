# Python API

Install the package, then open a directory containing the supported release's files:

```python
from ukinfo_reader import Database

with Database('extracted') as db:
    print(db.edition.id)
    db.edition.verify('extracted')  # Optional full SHA-256 identity check.
    print(db.segment_count)
    print(db.sector(0))

    # One fully joined directory record at a time.
    for record in db.records(0):
        process(record)  # Your own analysis function.
```

`Database` selects a candidate edition using exact file sizes. Only the release listed in
[Supported editions](editions.md) is supported. `edition.verify(directory)` confirms exact
source identity. The library opens both source files read-only and memory-maps them; using
a context manager closes the mappings and file handles even when your code raises an error.
Do not use the object after closing it.

## Methods

- `column(segment, field)`: decoded list for a field name or integer field ID. Treat this
  cached list as read-only. It contains stored column values; relationship arrays include
  their original sentinels.
- `block(segment, field)`: raw bytes for one source block.
- `sector(segment)`: display form of a segment's postcode sector.
- `segment_columns(segment)`: dictionary of decoded columns, keyed by field name.
- `records(segment, indices=None)`: generator of joined row dictionaries, optionally restricted
  to name/directory-entry ordinals. This resolves address and postcode relationships.
- `validate_segment(segment)`: check codecs, counts and joins; return aggregate counts.
- `close()`: release resources; repeated calls are safe.

`fields` holds schema definitions; `names` maps field names to IDs. These describe the source
format, not a normalized schema promised to be identical for future editions.

```python
from ukinfo_reader.validation import validate

report = validate('extracted')  # Entire source and every segment; may take a minute or more.
```

Catch `OSError` for file access problems, `UnsupportedEditionError` for an unrecognized release
or fingerprint mismatch, and `FormatError` for malformed compressed data. Both format error
classes are exported from `ukinfo_reader`. Use valid segment/field IDs and entry ordinals;
unknown field names raise `KeyError`.

The low-level codec functions are implementation details. Their format and regression evidence
are documented in [SE 4.0](formats/se4.md).
