# Candidate-driven broad first-name filtering

The six-edition qualification remains blocked by four preparation timeouts. One selective native search materializes about 200,000 global dictionary terms despite producing one source candidate. A read-only prototype returned identical responses and reduced shared-cache latency. This does not resolve the other cold-read failures.

Scope: retain bound prefix predicates and downstream given-name/middle-initial refinement, but perform broad first-name membership through each candidate's first_id when an exact surname and postcode anchor exist. Person searches only, without address or nearby filters. The entries name remains visible inside both ordinary and ordered source predicates. No deadline, concurrency, acceptance workload, source data or public edition changes.

1. Add invented PostgreSQL integration tests before production changes: actual dictionary work in custom/generic plans; complete paged identity sets for given/exact/prefix/wildcard names, middle initial, postcode precision and supported orders.
2. Observe the dictionary-work test fail against the current code. Implement the smallest candidate-correlated lookup with a planner fence. Re-run regressions and the existing broad-prefix completeness control.
3. Run the Go suite and review query consumers and unsupported-shape routing. Keep actual archive validation on NAS and private values out of reports.
4. Only after review, build and qualify a new immutable release through the existing acceptance process. Preserve all failures and cache-state qualifications. v14 remains hidden until complete six-edition gates pass.
