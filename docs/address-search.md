# Address search

The optional address index adds **street, village/locality, town/city and county**
searches to the Go/PostgreSQL web and desktop interface. A name is optional. Combine
address fields with each other, name filters, a postcode or a nearby radius.
[House/building/flat and given-name refinements](search-refinements.md) work alongside
these indexed controls without requiring another index build.

## Matching

Fields match whole, case-insensitive text after trimming/collapsing whitespace.
A single trailing `*` enables prefix matching, for example `High*` or `Stafford*`.
The name matching selector applies to names only. Street searches match either
recorded street component; village/locality searches match either locality component.
Multiple fields are combined with AND, within the same source address.

Historical places are searched as recorded on the disc. Counties can be absent,
postal, historical or different from modern administrative boundaries. There is no
automatic replacement with modern ONSPD geography, abbreviation expansion or spelling
correction. Nulls, raw references and fields withheld by source postcode precision
are never used to reconstruct a searchable displayed address.

Address searches admit four active requests per API process; additional admitted
requests wait within a five-second total deadline. The main search statement gets
a four-second timeout in a read-only transaction. Existing name/postcode-only SQL
deadlines remain unchanged.

Address-filtered searches use stable **source occurrence ID, then edition ID** order.
This is not alphabetical or distance order. Keyset pagination and page exports retain
all address predicates; changing a filter invalidates the previous cursor.

## Add an index to an existing catalogue

Use an owner connection and the matching server binary. Original source files are
not required: this reads the catalogue's existing checksummed compressed blocks.
It does not rewrite or copy source records or the original name/postcode projection.

```sh
DATABASE_URL='postgres://owner:password@localhost/ukinfo' \
  build/desktop/ukinfo-server index-addresses --dataset se2005-pcf175-2005-06
```

Repeat for each imported edition or sample that should support address searches.
`index-addresses` applies the additive migration first. The index is published only
when the complete edition has been read and membership counts agree. Failure or
cancellation rolls back the new index; existing searches remain available. A duplicate
complete index is rejected. There is no replace/resume operation.

For a bounded, rollback-only measurement first:

```sh
DATABASE_URL='postgres://owner:password@localhost/ukinfo' \
  build/desktop/ukinfo-server index-addresses \
  --dataset se2005-pcf175-2005-06 --address-blocks 512
```

This samples 512 evenly spaced whole source blocks, up to 65,536 records. It is a
clustered deterministic sample, not a random population sample. Pilot dictionaries
and memberships are temporary and rolled back, including their identity sequences.
Reports contain counts, storage bytes and timing, not source address values.

The CLI checks the local temporary filesystem every two seconds and cancels below
**5 GiB** free, including while SQL grouping/indexing runs. This cannot inspect a
remote PostgreSQL disk or the Docker VM's internal filesystem: monitor those too.
Build one edition at a time. Allow space for a temporary two-integer row per source
occurrence, external sorting, dictionary indexes, final postings and WAL. Reported
relation sizes exclude temporary files, WAL and backups. The dictionary byte count
is the cumulative shared dictionary, not the incremental cost of that edition.

For a dedicated serving role, grant access after migration:

```sql
GRANT SELECT ON address_locations, address_members, address_indexes TO ukinfo_search;
```

`GET /api/addresses` reports indexed datasets and their source/location counts.
The form enables street/locality/town/county fields only when every selected edition is indexed. “All
editions” uses the same sample-exclusion rule as ordinary searches. Requests selecting
an unindexed edition fail explicitly instead of silently returning incomplete results.
New CLI and desktop imports build their address index after source ingestion. Existing
catalogues and low-level library imports use the separate command above. If indexing
fails after ingestion commits, the source edition remains available for name/postcode
search; rerun `index-addresses` to finish. Cancellation cannot undo that source commit.

## HTTP examples

```text
/api/search?dataset=all&street=High%20Street&town=Example Town
/api/search?dataset=all&surname=Smith&county=Stafford*
/api/search?dataset=all&kind=business&street=High*&postcode=AA1
```

Supported parameters: `street`, `locality`, `town`, `county`. All are optional,
maximum 256 UTF-8 bytes each. At least two characters precede a trailing wildcard.
`%`, `?`, embedded/multiple `*` wildcards are not supported. Original source JSON and
citation/export behaviour remain unchanged.

## Storage

`address_locations` stores shared normalized street/locality/town/county tuples.
`address_members` stores a sorted occurrence-ID array per edition/location tuple.
`address_indexes` is the complete-edition publication marker. These are an additive
schema-2 extension. A bounded dictionary cache resolves batches while a temporary
integer staging table allows PostgreSQL to group memberships on disk.

[House/building/subbuilding filters](search-refinements.md) now refine indexed searches
against the displayed source fields. Place autocomplete, fuzzy matching and cross-edition
entity linkage remain later work.
[Address detail views](address-exploration.md) use existing postcode indexes and compare
complete recorded premises, including house and flat distinctions.
A matching street/location tuple does not establish a shared household or identity.

See the [complete-edition measurements](research/address-index.md) for the retained
layout, storage, query experiments and benchmark limits.
