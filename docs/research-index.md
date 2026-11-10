# Research index: grouping, source references and business filters

The optional additive research index reads existing lossless catalogue blocks. Original
records are kept. It enables **Group identical listings across this search**, archival
entry lookup when a catalogue position changes, and resolved business industry/SIC
filters. Alphabetical, nearest and latest-publication order work across the matching
result set, including ordinary source-entry mode; they do not sort just the visible page.

## Build and disk use

Desktop: **Manage archive → Build research index** for each required edition. It reports
progress and can be cancelled. Search-wide grouping is offered when the selected
sources have complete indexes. A business search skips editions without business
entries. Ordinary searches and grouping within a page remain available without it.

Self-hosted operators use the owner role for migration/indexing and the SELECT-only
role for serving:

```sh
ukinfo-server migrate
ukinfo-server index-research --dataset se2005-pcf175-2005-06
```

Reapply the read-role grants from [server setup](search-app.md) after migrating an
existing catalogue: the new `research_keys` and `research_indexes` tables need SELECT.
The initial four-edition research-index measurement covered **189,064,266
source entries**, including **6,152,296 businesses** in 2005. Together they add
**45.84 GiB**, before WAL/build/recovery headroom. Individual local builds took
6.1–21.4 minutes. See [per-edition measurements and validation limits](research/review-validation.md).
The later verified 2003 import brings supported source coverage to **248,951,107
occurrences across five editions**; the size/build-time measurements above exclude
that import and subsequent projections.
The native builder was outside the database's two-CPU/one-GiB Docker cap; these are
development-machine build times, not a VPS performance promise. Check free space
before building. Source media are not needed for this pass.

The build scans bounded compressed-block windows, writes a new edition partition and
publishes its completion marker only after validating its row count. An import or
another index build cannot run concurrently in the same database. Cancelling/failure
rolls back the new partition. The built-in free-space guard retains 5 GiB locally;
CLI checks observe the CLI host's temporary filesystem, not a remote database server.
Monitor that server's data/WAL volume separately. Removing an edition also removes its
research partition after checking ownership. No source export or private keys are added
to the code repository.

## Grouping contract, version 1

A listing group requires a complete **displayed** postcode, street and a house number
or building, with supported address fields, full postcode precision, and no unresolved
source warning. The explicitly invented-demo marker is allowed. Missing names,
ambiguous addresses, limited postcodes and other warned entries keep a separate key.

Eligible keys include listing type, surname/business name, given names, secondary
initial, all canonical displayed premises components, telephone, resolved industry and
SIC. Case and repeated whitespace are normalized; spelling, punctuation and diacritics
are not silently matched. A SHA-256 of the versioned canonical tuple identifies the
group. Unknown industry text is not reconstructed from raw references.

Group mode pages unique keys across the whole query. **View record** opens a paged
list of all source occurrences in the chosen edition scope. Source records can be
opened individually; **Export source page** preserves their original JSON values.
The main page export is labelled **Export representative page** in group mode: it
exports one representative per visible group. Publication sources remain separate and
reachable. A group is neither a verified person nor an inferred household/family.

The old **Group identical listings on this page** mode remains an explicit presentation
option; it cannot suppress repeats on other pages. These modes are deliberately labelled
differently. All-edition queries omit a sample when a complete import of that edition
exists. Group membership follows the selected scope, not every installed copy.

## Archival source identity, version 1

`archive_id` is SHA-256 of the versioned, sorted original component-name/SHA-256 pairs
from the source manifest. Decoder version, catalogue key, SQL dataset ID, sample/full
flag and ingestion position are excluded. The original `source_id` identifies the
entry/stream within those components. Together they define the archival reference.
Invalid/missing source checksums do not produce an archival ID.

New record/address links carry both fields plus the former catalogue key/ID as lookup
hints. The source-entry guard is always checked. If a hint no longer matches, a complete
research index can resolve the original entry in another import of the same components,
including a full import replacing a sample. Hash candidates are checked against their
original source IDs. Missing source scope is reported; a different entry is never used
as a substitute. An original component set with changed names or hashes is a different
version-1 source identity; future aliases/migrations must be explicit.

Research JSON now exports **version 2**, retaining optional `archive_id`. Version 1
files are accepted and upgraded in memory; their existing guarded references remain
usable. Before the first write upgrading a version-1 file, the app retains its original
bytes as `research-lists.v1-backup.json` (or the browser key
`ukinfo.research.v1.v1-backup`). It never overwrites an existing compatibility copy.
Older apps cannot import version 2; a rollback needs the earlier app and its version-1
backup, which excludes subsequent edits. Export a current full backup before switching
versions. Source snapshots/notes remain in the list even when a source cannot
be opened. `research-lists.json` and its browser storage key keep their existing names.

## Ordering and classifications

- **Name A–Z** uses normalized source name, given name and stable ties, with C collation.
  It is deterministic text order, not locale-aware surname interpretation.
- **Nearest** requires a nearby centre and orders eligible postcode estimates by distance.
  Equal distances have deterministic name/source/group ties. Incomplete/missing coordinate
  exclusions are the same as nearby filtering. Distances are not property locations.
- **Latest publication first** extracts the cover-disc `YYYY-MM` from the edition identity;
  unknown dates sort after known dates. A grouped listing uses its newest publication.
  It does not assert a date of residence or a move.

All cursors bind the submitted filters, ordering, geography release and selected
catalogue generation. Change the criteria/catalogue and rerun from the first page.
The small-backend [national-query probes](research/review-validation.md#wider-national-queries-remaining-performance-limits)
still time out for common-name/broad-prefix grouping and national street alphabetical
order. Narrow by location or edition, or use occurrence mode. The warm bounded-query
load result does not establish fast national grouping.

Search deadlines/admission still apply: expensive national sorts can require narrower
criteria; they are not returned as an incomplete successful result.
Ordered SQL has a four-second statement budget inside the API's five-second request
budget. Name-anchored queries without address/geography constraints use bounded windows
of 128 distinct name pairs before indexed source/group lookups; the page limit is at
most 100, and cursor predicates apply before the lookahead. This bounds intermediate
work without capping the search's total matching terms or entries. Other orders and
address/geography constraints use the selective materialized candidate path. A timeout
returns an explicit error; it never turns a partial scan into a successful final page.

Business **Industry description** and **Source SIC code** match the resolved source
strings, exactly or with one trailing `*` (at least two prefix characters). They can
anchor a search without a business name. No modern SIC labels or fuzzy categories are
inferred. Missing/ambiguous industry descriptions remain unknown and are excluded by
an industry filter. The 2004 SE disc has no business file. See the
[modern business format evidence](formats/modern-se.md#v105-business-layout-and-selection).

## Optional business search projection

For ordinary SIC/industry searches, operators can add a smaller projection of
business search keys, classifications and canonical address references. Original
records, source references and grouping keys remain authoritative. Build after
complete research and address indexes:

```sh
ukinfo-server index-business-search --dataset se2003-dpcs0104
```

`deploy/production/business-search.sql` provides the equivalent owner-only,
resource-bounded PostgreSQL procedure. It verifies the source prerequisites and
business row count, attaches a new edition partition, then publishes matching
versioned metadata atomically. Cancellation rolls back the unfinished partition.
Ordinary VACUUM follows commit for covering index reads. The builder is idempotent;
an incompatible or unexpected relation is rejected. Check both database volume
headroom and the CLI's temporary filesystem; the CLI retains its 5 GiB guard.
Only the owner may call the SQL builder. The serving role needs SELECT on
`business_search` and `business_search_indexes`.

The API automatically uses the projection only when every relevant selected
edition has a complete matching build. Missing tables, grants or edition metadata
retain the current complete search path. Street, locality, town, county, name,
postcode and displayed-field refinements retain their existing meaning and cursor
format. Grouped, specially ordered and nearby searches retain their current path.
No total-match cap or extra public export route is introduced.

For controlled comparisons or rollback, use `serve --disable-business-search`.
Removing an edition also removes its verified business partition and metadata.
Archives without this optional index remain supported; this does not change the
research-index requirement for classification filters.
