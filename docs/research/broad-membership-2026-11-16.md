# Broad given-name membership planning

The `.15.4` immutable candidate failed the unchanged full five-edition qualification with one HTTP query timeout and no content mismatches. All ten source-cold cases passed (maximum 2.167 seconds); C1/C4 p95 were 93.127/148.759 ms. It was not deployed. The public service remains `.15.2`, five editions; v14 remains hidden.

A private retained trace identified the candidate-selection SQL. Its custom prepared plan estimated one matching given-name dictionary row but materialized 9,701. It then performed 9,701 surname/first-name index probes, applying the postcode restriction afterward. The generic plan instead used the surname/postcode anchor. Both entry paths had zero heap fetches; another covering index would not fix this join-order problem.

A read-only SQL experiment retained the complete materialized dictionary and changed only `first_id IN (...)` to `(first_id IN (...)) IS TRUE`. In a WHERE clause these exclude the same false/NULL values. The Boolean expression keeps the membership subquery from being pulled into a join driven by the underestimated dictionary. It permits hashed membership after the selective entry anchor.

| Prepared custom plan | Original | Membership fence |
| --- | ---: | ---: |
| Entry index probes | 9,701 | 1 |
| Shared buffer hits | 42,313 | 3,553 |
| Execution time in repeated diagnostic | 22.569–22.780 ms | 5.737–5.842 ms |

All eight prepared executions across both SQL variants and generic/custom modes returned identical ordered candidate values. Caches were shared and uncontrolled; these timings are diagnostic evidence, not cold qualification. Full SQL, input values and plans remain on the NAS.

The implementation limits the fence to the existing exact-surname broad first-name branch when the search uses a non-wildcard given name and a postcode. Wildcard/prefix and searches without a postcode retain their previous SQL. It adds no index, planner GUC, deadline, pool or pagination change.

An invented-data regression uses a coarse dictionary histogram, 9,700 valid middle-name forms and 20,000 irrelevant entries. Against unchanged code, explicit named prepared execution made 9,703 entry probes in custom mode and failed the <=32-probe gate; the generic control passed. The regression requires both planning modes to pass while retaining four literal expected record identities. Existing full pagination, broad-tail completeness and Unicode controls remain required.

A fresh immutable candidate and the original full acceptance are still required before deployment; this change alone does not qualify v14 publication.

## Verification

The final regression uses a named prepared statement for both execution and `EXPLAIN EXECUTE`, so custom/generic work measurements inspect the actual prepared plans. Review caught the earlier direct-EXPLAIN limitation; the corrected test was rerun against unchanged source (custom fails, generic control passes), then the fix passed both modes. All 46 catalog tests passed on disposable PostgreSQL 18.7, including pagination, complete valid-name tails and Unicode boundaries. The target-dependent ParadeDB integration test requires its own target and remains part of hosted CI. Full Go tests, vet and diff checks pass. Independent review found no production correctness or scope issue.
