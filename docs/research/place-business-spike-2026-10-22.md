# Town/county postings and a smaller business projection

Measured on 22 October 2026, following the
[full-edition projection experiment](hosted-projection-2026-10-22.md).
This is an experiment report; neither candidate is deployed on the public site.
The existing hosted build and query budgets remain unchanged.

## Scope and method

The complete 2003 SE DPCS0104 edition contains 59,886,841 entries, including
2,471,038 businesses. Original compressed source blocks remain authoritative.
Only private additive tables and temporary, unexposed containers were used.
The database retained its 2 CPU / 6 GiB limits, 1 GiB shared buffers, and existing
serving indexes. Temporary clients retained bounded pools, memory and deadlines.

A temporary restricted role inherited the serving role's permissions and received
read access to the private schemas. Both roles saw the same five published
editions and 248,951,107 entries. The public serving role received no new schema
or table grants. Private APIs had no public ports or external telemetry export.

Reports contain aggregate timings and counts. Cases, source records, IDs, hashes,
cursors, passwords and connection strings remain private.
[Machine-readable aggregates](place-business-spike-2026-10-22-results.json) use
nearest-rank quantiles including failed requests. First-page cohorts are fixed;
later pages stop after a failure, so their populations differ between runs.

## Town/county postings

[Reproducible build and cleanup](../../tools/place-postings/README.md).
For each edition, normalized field and place key, sorted source IDs are split into
ranges of 8,192 records. Each occurrence belongs to at most one town and one
county. Prefix searches therefore preserve uniqueness; multiple fields intersect.
Blank fields have no searchable list. The builder checks complete coverage and
per-field counts, then vacuums/analyzes before publishing private build metadata.

The full edition built in **364 seconds** and occupied **969,195,520 bytes
(924 MiB)**. Original records were not re-decoded or copied. Town/county candidates
are intersected with existing name/postcode/classification source indexes, and
the API retains its record decoder, refinements, lookahead and keyset cursor.
Street/locality, grouping, lexical ordering and incomplete mixed-edition builds
retain the existing complete path.

The HTTP workload comprised 62 private cases with exact/prefix places, sparse and
empty intersections, people, businesses, classifications, and the reported search
shape. Each case followed up to three pages of 50 records. Complete successful
responses and page availability were compared; each server followed its own
opaque cursor. Baseline and candidate order alternated per case/page. A separate
invented-data integration suite compared bodies and cursors across editions,
combined fields, business kinds, refinements and fallback paths.

| Fixed 62 first-page attempts | Baseline errors | Candidate errors | Baseline p50 / p95 | Candidate p50 / p95 |
|---|---:|---:|---:|---:|
| Initial prototype | 15 | 14 | 1,323 / 4,333 ms | 466 / 4,028 ms |
| Business-source anchor refinement | 2 | 5 | 426 / 2,634 ms | 216 / 4,004 ms |

The first prototype performed badly on unanchored business searches because it
walked person IDs before rejecting their kind. The refinement starts those
searches from the existing business index. This removed most such regressions,
but broad materialized intersections still timed out. The initial run
overlapped an owner aggregate sizing query; the repeat had no operator build or
sizing work. Cache state was uncontrolled and the repeat was warmer.

All 120 mutually successful page comparisons in the refined run matched. Across
all 133 comparisons, the baseline had eight errors and the candidate five; the
fixed first-page cohort nevertheless regressed. Successful-response agreement
does not validate candidate-only successes against an independent real-data
oracle. No cold-cache or standalone throughput claim follows from these runs.

**Decision: leave `serve --experimental-place-postings` disabled.** Lower median
latency and smaller storage do not offset first-page timeout regressions. There
is no match cap, timeout increase, source rewrite or public format switch.

## Business-only classification projection

[Reproducible build, benchmark and cleanup](../../tools/business-projection/README.md).
This table contains only businesses and the search/group/address/classification
columns needed by the experiment. It has a primary key and covering nonempty SIC
and industry indexes; it omits the full projection's name, postcode and location
indexes. It is deliberately restricted to classified-business searches.

The complete business population built in **73 seconds**. The table occupied
230,121,472 bytes and indexes 204,701,696 bytes: **434,823,168 bytes (415 MiB)**
total, compared with 13.34 GiB for the earlier full-edition prototype. These
projections support different search scopes; the smaller table cannot replace
the full catalogue.

The workload selects the 39 classified-business cases from the previous spike,
including its 18 guaranteed-positive SIC/place variants. Two rounds follow up to
three 50-record pages. Both sides load/decode the original source records and
validate grouping identity separately. The source edition lacks industry
descriptions, so empty industry searches are separated from guaranteed-positive
SIC searches. The benchmark rejects people, unclassified business queries,
refinements, grouping, lexical ordering and nearby searches.

| Fixed 78 first-page attempts | Baseline errors | Projection errors | Baseline p50 / p95 | Projection p50 / p95 |
|---|---:|---:|---:|---:|
| One worker | 0 | 0 | 243 / 1,564 ms | 48 / 369 ms |
| Four workers | 7 | 0 | 169 / 4,095 ms | 81 / 612 ms |

For the 36 guaranteed-positive first-page attempts, p95 fell from 1,564 to
200 ms with one worker and from 4,102 to 781 ms with four workers. The four-worker
baseline had four errors in this subset; the projection had none.

All 148 mutually successful one-worker page comparisons and all 121 mutually
successful four-worker page comparisons matched. There were no candidate errors
or setup failures in either stage. The four-worker command returned nonzero
because the baseline had ten errors across its variable 131-page comparison
population; it completed its bounded workload and retained the aggregate report.

**Decision: this is the stronger candidate for integration.** It retained the
classification gains with 415 MiB of additional storage and no observed candidate
errors in this workload. It remains a private SQL experiment: adoption still needs
an actual HTTP access path, mixed-edition completeness/fallback, every supported
edition, refinements and independent real-data acceptance before publication.

These are interleaved SQL candidate/record measurements, with an eight-connection
pool shared by bounded workers, rather than complete HTTP-handler or standalone
throughput tests. They retain the restricted serving permissions and exclude
identity-validation time from ordinary query/record latency. Cache state,
checkpoints and unrelated NAS activity were uncontrolled. ID-ordered pages also
have better record-block locality than scattered lexical results.

## Verification and cleanup

- Race-enabled backend integration tests compare complete bodies/cursors and prove
  that the opt-in posting path is exercised; default behavior remains unchanged.
- Business projection tests compare positive/negative classifications and paging,
  reject unsupported scopes, and detect deliberately corrupted classification
  data. Existing grouping-identity corruption checks remain in place.
- The actual SQL builders and guarded cleanup scripts were executed against
  invented disposable data, including a blank county and overwrite refusal.
- The HTTP runner has independent-cursor pagination tests and keeps all cursors,
  records and response digests out of aggregate exports.

After retaining aggregate evidence, guarded RESTRICT cleanup removed both private
schemas. The temporary role and all six experiment containers were removed.
The database returned to about 153 GB; the serving role still saw five editions
and 248,951,107 entries. Three existing public search probes returned HTTP 200.
These are availability observations, not capacity evidence. Production stayed on
`hosted-2026-10-21.4`; no source/rollback edition, serving grant, public port,
Cloudflare rule or query budget changed.

Next: integrate the business-only classification path behind complete edition
metadata and preserve the current fallback. Validate actual HTTP responses,
refinements, every supported edition and tail latency before enabling it. The
place-posting experiment needs further selectivity work and should remain off.
