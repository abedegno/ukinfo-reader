# Search catalogue schema 2

This derived format belongs to this project; it is separate from the proprietary
[disc formats](../editions.md). PostgreSQL 18 holds both the query projection and the
original decoder output. Each edition is an independently searchable partition.

## Tables and identities

- `catalog_schema`: incompatible schema version, currently 2.
- `datasets`: smallint key, unique external dataset key, source manifest, count and
  aggregate import statistics. Dataset numeric IDs can have gaps after rollback.
- `terms`: shared integer-to-normalized-text dictionary using PostgreSQL `C` collation;
  ID zero means the empty key. Codes are implementation details, not alphabetical rank.
- `entries`: list partition per dataset; integer import ordinal, kind (0 person,
  1 business), name/first-name/postcode term IDs. The primary key is `(dataset_id,id)`.
  Separate name and postcode B-trees serve exact filters and bounded prefix searches.
  Prefixes expand through the C-collated vocabulary index before integer entry lookups.
- `record_blocks`: list partition per dataset; zero-based block ID and `bytea` payload.
  An ordinal's block is `(id-1)/128`, with offset `(id-1)%128`.

Source identity is the edition-qualified `source_id`, stored with every record. The
1999 decoder additionally qualifies IDs by source segment. Integer ordinals are only
internal paging coordinates. No household/person deduplication is performed.

## Lossless block encoding

The Zstandard frame enables its content checksum. Decompression is limited to 16 MiB.
The uncompressed structure uses little-endian integers:

```text
5 bytes: ASCII "UKIB1"
u16: number of records, 1..128
repeat for each record:
    u16: UTF-8 source-ID byte length (maximum 512)
    u32: original JSON byte length (maximum 65,536)
    bytes: source ID
    bytes: exact original decoder JSON
```

Only the final block can have fewer than 128 records. JSON objects preserve every
source field, null, warning and numeric token. The normalized search keys are derived
from displayed source fields; raw postcode references never override displayed values.
The API decompresses the needed blocks and extracts the selected original JSON slices.

The dataset digest is SHA-256 over the concatenation of each record's two length fields,
source ID and JSON bytes, in import order. Block headers are excluded so the digest is
independent of how records are grouped. Stored statistics include this digest, raw and
compressed byte counts and timing phases. `verify` checks every block and projection.

## Streaming protocol

`python -m ukinfo_reader.search_export --data FOLDER` writes one JSON object per line:

1. `manifest`: protocol 2, dataset/edition, title, expected count, sample flag, reader
   version and required source-file SHA-256 hashes.
2. Exactly `expected` `record` frames: `source_id` plus original `record` object.
3. `end`: decoded count and the independently calculated `sha256` described above.
4. EOF, with no extra frames.

The decoder checks source hashes both before and after iteration. `--limit N` changes
the key to `EDITION:sample:N`, expected count to N and sample flag to true. Protocol 1
remains accepted for older trusted producers and repacking; it lacks a mandatory
independent terminal digest. A manifest is an assertion by the producer, not a digital
signature. Source fingerprints and decoder version remain necessary provenance.

## Publication and limits

One advisory lock serializes ingestion per database. The importer writes new standalone
partition-shaped tables in one transaction, validates source IDs with private external
sort runs, builds indexes, then attaches the tables and commits the dataset metadata.
Existing partitions stay available during loading. Failure rolls back the new edition;
a new attempt starts at the beginning. Identity scratch files are removed before index
sorting. The table design supports at most 2,147,483,647 occurrences per dataset and
32,767 dataset numeric IDs; known editions are well below these limits.

Schema 1 used a wide JSONB table. It is never upgraded in place. Use a separate schema-2
destination with `repack-v1` or re-import source media; see [migration](../search-app.md#schema-1-preview-migration).
The repacker preserves original field values, not JSONB's discarded original lexical
whitespace/key order. Repacked source manifests remain unchanged. Old cursor tokens are
incompatible with schema 2; clients must restart pagination.
