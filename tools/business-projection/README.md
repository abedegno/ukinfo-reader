# Classified-business projection experiment

Private, additive, single-edition experiment. This repeats the earlier full-edition
projection comparison using only business rows and three indexes: primary key,
covering nonempty SIC, and covering nonempty industry. It keeps the original
source records and decoder. General name, postcode, location and person indexes
are omitted; this is not a replacement search catalogue.

Run `psql -v dataset=EDITION -f build.sql` as the catalogue owner. It refuses an
existing schema, verifies complete source/research/address coverage, checks the
projected count against the complete business source set, then vacuums/analyzes
before marking the build finished. There is one business-only insertion rather
than a full-edition copy. It uses bounded statement, memory, temporary-file and
lock budgets. No serving grants, publication or source mutation occurs.

Use `ukinfo-benchmark-projection --business-only` with private cases. Every case
must explicitly select businesses and include SIC or industry; unsupported scopes
are rejected. The command retains ID ordering, lookahead, paging, original record
decoding, grouping-identity validation and redacted aggregate reporting. Use a
private restricted read-only role with source-serving permissions plus SELECT
and schema USAGE on the private build; never use the public API to create it.

The benchmark compares SQL candidate selection plus record retrieval. It is not an
HTTP capacity test and excludes UI refinements, lexical ordering, deduplication
and nearby searches. Empty results, especially unsupported source industry
descriptions, must be separated from guaranteed-positive cases. Judge errors and
p95 on the same first-page cohort, along with disk cost and correctness.

After saving aggregate evidence, stop temporary clients and run `cleanup.sql` as
the owner. It removes only the named private tables/schema, transactionally with
RESTRICT; additional dependencies abort cleanup. Remove the temporary role and
containers as well. There is no production migration or deployment switch.
