# Counterbalanced postcode prefilter spike

> Implement inline with executing-plans. One whole-change independent review before merge.

**Goal:** Compare original and redundant-regex-free SQL for the two retained P8 postcode-only alphabetical searches, proving result equivalence within this small diagnostic while retaining all full qualification gates.

**Architecture:** A separate Go tool module imports the unchanged server catalog. Its target tracer captures and cancels the first eligible candidate SELECT before execution. Only the single known `postcode @@@ pdb.regex($n)` predicate is replaced with a non-null check on the same nonempty string parameter; all scalar precision predicates, ordering, cursor behavior and arguments remain identical. Use actual serving `force_generic_plan`, read-only SQL and4second target deadline. Execute regex/scalar/scalar/regex for the first case, scalar/regex/regex/scalar for the second. Hash returned positional rows in memory; output equality/counts/timings/closed errors only, no raw rows/queries/args/hashes/case IDs. This is a diagnostic, not full reference or cold qualification.

**Stack:** Go1.26.11, pgx5.10.0, existing catalog module via local replacement. No serving source changes, local Docker/database copies, index builds or PostgreSQL setting changes.

**Ruling:** The older trial-plan command always runs scalar before regex and custom before generic, and does not prove returned-row equality. Use counterbalanced actual generic execution instead; this costs a small tool module but avoids selecting an apparent winner from fixed cache order or unverified results. Shared/uncontrolled caches still prevent original cold-failure attribution.

## Files and interfaces

`tools/postcode-spike/go.mod`, `go.sum`: separate command module with server import-prefix compatibility for internal packages, local replacement `../../server`, existing pinned dependencies.
`capture.go`: strict two-case eligibility, target-only tracer, one exact SQL/argument capture and context cancellation, known-predicate transformation.
`compare.go`: fixed counterbalanced schedule, bounded read-only pools and60second total context, actual row digest/equality in memory, closed error classification. Optional database-wide read/hit deltas may be unavailable and never claim exact request attribution.
`main.go`: private file I/O, no argument/error dumps, complete/partial aggregate on termination.
`*_test.go`: meaningful pure tracer, SQL transformation, schedule, equality/error/privacy tests; no database needed locally.
`.github/workflows/search-app.yml`: run module race tests/vet alongside existing tools, without changing serving tests or corpus.
`README.md`: private use, bounds, counterbalancing and cache/qualification limits.

## Task 1: Capture the exact candidate SQL

- [ ] RED/GREEN capture and transformation tests: only postcode-only person/name-sort/first-page legacy scopes, reject extra filters/duplicate keys/invalid subset; cancel capture without canceling parent; exact one regex predicate, nonempty string arg, no placeholder/argument change, copy mutable argument list, reject malformed/unknown SQL. No unbounded query string output.
- [ ] RED/GREEN comparison/report tests: exact8-query schedule and2 cases,32second worst statement allowance within60second context; all successful variants must return identical ordered rows and counts; changed rows, failures and partial schedules cannot claim equivalence. Numeric/enum-only output and sanitized errors, no private digests or values. Parent context stops bounded work; no retry.
- [ ] Implement private CLI/runtime and add race/vet CI. Source pool8/target4, target force_generic_plan/4000ms/read-only, source existing reader settings. Capture only requested two cases and never call public HTTP. Validate output path and file mode0600; no overwrite of private input/report. Build explicitly Linux/amd64/CGO0; verify ELF and Go settings.
- [ ] Focused race/vet/format checks, independent whole-change review, fix pass, reader PR exact-head/main CI. Record tool source commit/binary digest separately from unchanged serving source46ee39c. Then homelab adapter pins both, uses existing ownership/lease/CI/reader guards and60second child/90second controller bound (tool60second context plus bounded termination grace).
- [ ] Run once on NAS only after both exact-main check sets, recover aggregate, independently verify PID0/removal/lease/backend/public identities. Keep original P8 failure and all six-edition proof gates. Choose the next change from measured results, not from a successful warm run.

## Constraints

Original nine-phase/355-case/250-reference/1536-field workload, SQL/HTTP/pool/resource/cold/publication limits remain unchanged. No qualification retry in this spike. Source search code must remain byte-identical to46ee39c; new module lives outside `server`. Raw inputs, ordinals, SQL, parameters, rows and hashes remain NAS-private. No telemetry exporter. No public visibility changes. A benchmark success cannot mark the v14-live goal complete.


## Task 2: Counterbalanced execution and private output

- [ ] Write failing tests for the exact eight-observation ABBA/BAAB schedule, complete row-result equality, query errors/timeouts, finite timings and output privacy. Implement connection-independent comparison with injected runner, then actual read-only pgx execution and CLI output. SQL error/row digests remain in memory; only fixed error enums, row counts and equality flags leave the tool. Capture is deliberately cancelled and cannot be counted as a serving failure or benchmark execution.

## Task 3: CI and private operator integration

- [ ] Add race/vet execution to the existing search-app workflow, document use/limits and build Linux/amd64/CGO0 with verified ELF/build settings. Require reviewed exact tool-source/main CI plus homelab exact-main CI before one NAS invocation. All eight executions retain four-second SQL, sixty-second total and ninety-second controller bounds. No code under server changes, no local Docker/database copies, no cache eviction or unbounded index creation. Integrate strict retained-three-failures/two-ordered selection and fixed-field aggregation in homelab, verify cleanup and choose the next actual change from results.


## Verification ledger

Capture/transformation, counterbalanced equality/error/cancellation, bounded row consumption, exclusive output reservation, safe failure receipts and malformed-URL regressions were observed failing before implementation, then passing. Whole-change independent review identified three Important issues (standard catalog logging, DSN execution/cache overrides and fallback routing) and two Minor issues (exact SQL structure and setup timing). All were resolved in one tested fix pass. Normal-cache race tests and vet pass; sandbox cache-denial attempts were not counted as passing vet. Serving files remain unchanged from46ee39c. No NAS execution or qualification retry has occurred from this tool. Reader exact-head/main CI and separate pinned homelab integration remain required.
