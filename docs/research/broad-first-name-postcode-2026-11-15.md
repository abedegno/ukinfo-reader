# Broad first names with a surname and postcode

**Superseded candidate:** the subsequent `.15.3` qualification failed with three source-query timeouts. The correlated entry path described here was not deployed. The [replacement investigation](given-name-boundary-2026-11-15.md) moves given-name boundaries into vocabulary lookup while restoring the previous entry-membership path. The measurements below remain diagnostic history.

A six-edition preparation search timed out while checking an exact surname, a broad first-name prefix and a partial postcode. Its native source plan produced one candidate but materialized 200,879 global dictionary terms. The entries lookup was already index-only; adding another covering index would not remove that dictionary work.

For person searches with an exact surname and postcode, without address or nearby filters, that candidate’s broad first-name membership looked up each candidate's first-name key by primary key. A planner fence preserved the candidate-driven lookup. Bound prefix predicates, given-name and middle-initial refinement, response ordering and continuation remain unchanged. Other broad search shapes keep their existing paths.

## Verification

The regression fixture is entirely invented and contains 200,000 unused prefix terms. Before the change, both custom and generic plans examined 200,013 dictionary rows. The new resource assertion requires at most 32 rows for that fixture and passed after the change. It measures executed dictionary work rather than depending on a particular textual plan or elapsed-time threshold.

All 54 pagination combinations passed against a fresh PostgreSQL 18.7 instance: six name/postcode/refinement cases, three orders and page sizes 1, 2 and 50. The existing no-postcode broad-prefix completeness and latency control also passed in auto, custom and generic plan modes. These database tests run in an isolated, networkless NAS container with memory-backed storage and no archive mounts.

An earlier read-only prototype returned equal complete responses for the affected search and three control shapes. Shared-cache affected-search times were 53–78 ms before and 7–24 ms with the prototype. These are diagnostic timings, not cold-start qualification or deployment evidence.

The original six-edition preparation failure remains retained: four timeouts in 355 searches. This change addresses one source-plan problem. The other source/derived cold-read failures, complete qualification, source-cold checks and guarded v14 publication remain outstanding. No deadlines or acceptance gates were relaxed.
