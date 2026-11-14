# Preserve postcode precision in the redundant index prefilter

Partial postcode queries already retain scalar range and length checks to
distinguish an outward district from a spaced inward sector. They also accept an
identical partial value recorded in the historical source. The redundant inverted
index regex previously admitted every suffix length before those checks ran.

The prefilter now admits the stored partial itself or exactly the required full
length: two suffix characters for a sector, three for a district. The original
scalar predicates, ordering, pagination and final limit remain unchanged. Query
prefixes pass the existing ASCII postcode parser and are regex-escaped. Stored
postcodes are normalized by removing whitespace; suffix lengths count characters,
including unusual historical Unicode text, as PostgreSQL `length` does.

An operator experiment ran two retained ordered-postcode shapes in both directions,
with four complete ordered-position comparisons. All eight queries succeeded and
all comparisons matched. On the reversed round, statements changed from 34.49 to
24.55 ms and 66.22 to 33.87 ms. Caches were shared and uncontrolled, so these are
warm diagnostics, not cold acceptance or a capacity claim. A separate full-prefix
materialization experiment timed out and was rejected.

Unit tests cover district/sector ambiguity, identical stored partials and Unicode
character counts. PostgreSQL/ParadeDB integration also stores Unicode suffixes and
compares complete pages and cursors against the native source reader. Full fresh
NAS qualification remains required before deployment. This change does not alter
the native default-order postcode route or publish v14.
