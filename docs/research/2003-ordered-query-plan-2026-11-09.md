# Failed 2003 ordered-query plan — 9 November 2026

**The query uses Top-K, but its street-prefix predicates remain expression
filters inside the search executor.** One fresh read-only execution returned
51 candidates in 705 ms while recording 18,239 shared-buffer reads. This
identifies a concrete query-optimization hypothesis; nothing was deployed.

## Scope and method

The failed case from the stopped CPU experiment is a street trailing-prefix
search across the 2003 edition, sorted by name, requesting 50 results, with no
geographic filter, cursor or additional refinement. Its identity and text remain
NAS-side. Selection used the sole failed per-case counter; no preferred query
was substituted. This is broader than a name/postcode lookup.

The exact structural SQL was reconstructed from unchanged serving-source
`searchFieldConditions`, `paradeSQLBounds` and `paradeResults` (ce6117f): kind;
nonempty street OR street_2 prefix ranges; pdb.all(); name, first_full, id order;
LIMIT 51 for page lookahead. The inspected fields are ASCII, with normalization
parity asserted. Those serving-source files match this checkout. This is not a
capture of the historical SQL or historical executor plan.

One generic EXPLAIN and one EXPLAIN ANALYZE/BUFFERS ran through the existing
SELECT-only API role, each capped at 4 s. No longer deadline, repeated timing,
query/index/code variant or load benchmark ran. Peak private budget: PG 1.25
CPU/1,408 MiB plus helper .25 CPU/128 MiB. Raw SQL/arguments/plans remain on NAS.

## Fresh executor evidence

| Signal | Observation |
| --- | ---: |
| Generic/custom prepared observations | 2 / 0 |
| Executor | Custom Scan / TopKScanExecState |
| Included/pruned segments | 4 / 0 |
| Actual output / heap fetches | 51 / 51 |
| Execution time | 705.389 ms |
| Shared hits / reads | 15,435 / 18,239 blocks |
| Temporary read/write blocks | 0 / 0 |
| Columnar / heap buffer hits | 15,404 / 5 |
| Metadata / postings / other buffer hits | 13 / 12 / 1 |

There is no separate PostgreSQL sort or residual Filter node. The internal
Tantivy query includes nonempty constraints for both street fields, four
`postgres_expression` street range comparisons and a kind expression, with an
all-documents condition. Inclusion inside Top-K is not equivalent to a selective
prefix-term constraint. Columnar buffer work dominates the hit breakdown; a
51-row heap fetch is not the main observed work.

At 8 KiB/block, 18,239 shared reads account for about 142.5 MiB. These are
PostgreSQL buffer counters, not measured physical disk bytes. OS cache may serve
reads. track_io_timing was off, so the plan cannot separate I/O elapsed time from
CPU time. It also does not report total candidate documents examined. Do not
claim a proven full-table scan or attribute the original 5 s failure to one
resource from this single fresh execution. Shared HDD/cache/load remain uncontrolled.

## Next hypothesis

Investigate a redundant, escaped index-term prefix constraint over the two
street fields, retaining the complete scalar OR predicate and ordering. The
existing postcode implementation provides a local pattern. Such a constraint
could reduce candidates before expression evaluation, without rebuilding data.
This is a hypothesis, not an implemented or measured speedup.

ParadeDB documents regex term-prefix queries and the literal tokenizer for
matching original text. Its description supports testing this mechanism, while
installed 0.26.3 behaviour still needs direct validation. [ParadeDB regex
reference](https://www.paradedb.com/docs/reference/full-text/regex).

Before any timed comparison, verify result equality for both street positions,
empty fields, escaped punctuation, Unicode, combinations and cursor paging.
Keep the scalar predicate authoritative; preserve failed observations and fixed
latency targets. Neither raising CPU nor source-address prefetch has been
established as the remedy for this shape.

## Evidence and cleanup

Homelab code/protocol commit `703cf85`, `docker/ukinfo/ordered-plan/`, uses the
existing pinned profiler image. Five shared profiler tests passed; inspection
scripts compiled. Two network-isolated read-only helpers selected the structural
case and redacted the retained plan; they did not execute database queries.

NAS root `/home/jonw/ukinfo-backups/ordered-plan-2026-11-09` contains
`reports/query-private.json`, `plan-private.json`, `analyze-private.json`,
`summary.json` and `control/closed.json`. Only structural/numeric aggregates
were exported; credentials, query values, raw plans and source identities remain
on NAS. Durable local aggregate log/state are under gitignored private/operations.

Fresh verification confirmed SQL helper/private PG stopped, maintenance lease
released, original private PG limits restored to 1.5 CPU/1,536 MiB, and public
health OK on its original image. Read-only inventory supervisors were boundedly
stopped (sleep exit 137); the SQL helper exited zero. No source/live restart,
index rebuild, storage change, Bitcoin pause or v14 publication occurred.
