# Town/county posting experiment

Private additive prototype; disabled by default. Original compressed records,
address projections and published editions remain authoritative.

The posting key is `(dataset_id, field, normalized_place, source_ID_range)`.
Each array contains sorted IDs from a range of 8,192 source records; fields 1
and 2 are town and county. Empty fields have no list. Prefix ranges use the
same normalization and C-collation bounds as ordinary address search. Multiple
fields intersect. Each source ID appears at most once per field, so prefix
searches need no deduplication.

`build.sql` takes `psql -v dataset=EDITION -f build.sql`. Run as the catalogue
owner with an operator-controlled SQL file. It refuses an existing schema,
commits bounded batches, checks complete source coverage and per-field list
counts, then vacuums/analyzes before setting `finished_at`. It grants no public
or serving-role access. A failed build is unpublished; inspect its private log
and remove only its experiment tables before retrying. There is no resume mode.
Do not run it through the public API or put it into production migrations.

To compare actual HTTP responses, use a private API with
`serve --experimental-place-postings`, no published ports, and a temporary
read-only role inheriting the serving role plus SELECT/USAGE on this schema.
Keep the normal database/API CPU, memory, pool and query budgets. The baseline
must use the same role, settings and source edition. Remove the temporary role
and containers after collecting aggregate results.

The flag falls back to the existing complete path unless every selected edition
has a finished matching build. Street/locality, grouped and lexical ordering use
the existing path. Candidate IDs are intersected with the original source
indexes, and the API still loads the original record blocks, applies refinements
and produces its existing keyset cursor. No cap truncates the search universe.

Use `tools/search-load --compare-url BASELINE --probe-only --compare-pages 3`
with private cases and explicit request/time budgets. Each server follows its
own opaque cursor; complete response digests and page availability are compared.
Report the fixed first-page cohort separately from later pages, since failures
stop further paging. A smaller index alone does not justify deployment.

After stopping the private clients and retaining aggregate evidence, run
`psql -f cleanup.sql` as the owner. It removes only the named private experiment
objects, transactionally with RESTRICT; unexpected dependencies abort cleanup.
