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

`Database` selects a candidate edition using exact file sizes. Only the releases listed in
[Supported editions](editions.md) are supported. `edition.verify(directory)` confirms exact
source identity. The library opens the required source files read-only and memory-maps them; using
a context manager closes the mappings and file handles even when your code raises an error.
Do not use the object after closing it.

## SE 4.0 methods

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

## PC Format 78 API

`Database(directory)` dispatches explicitly by `edition.format`. For the earlier release,
it returns a `PCF78Database`, with `format == 'pcf78'`; the SE 4.0 object has `format == 'se4'`.
It supports the same context manager, `edition.verify()` and `close()` operations. It has
no SE 4.0 `segment_count`, `column()` or `sector()` method.

```python
with Database('extracted78') as db:
    for row in db.search(surname='Smith', first_name='John', sector='AB10 1'):
        process(row)
    # db.records() streams all indexed entries in original index order.
```

`search(surname=None, first_name=None, postcode=None, sector=None)` combines exact filters
with AND. Surname lookup uses the original index; searches without a surname scan it.
`records()` yields the same stream as `search()` with no filters. `fields` describes output
names. `decode_record(offset)` returns raw references for one physical record; it does not
establish that the record belongs to the live index. This low-level method is intended for
format research. [Record fields, joins and anomalies](formats/pcf78.md) are edition-specific.

`validate(directory)` selects the format's complete validator. Earlier-disc progress callbacks
receive `(completed_bytes, total_bytes, entries)`; SE 4.0 callbacks receive
`(completed_segments, total_segments, entries)`. Reports use different format-specific totals.
