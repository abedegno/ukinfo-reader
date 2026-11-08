# Cross-edition address prefetch implementation plan

> Execute inline with the executing-plans skill; request one independent review after implementation.

**Goal:** Reduce cross-edition address latency while returning exactly the same records, ordering, page boundaries and cursors.

**Architecture:** Extract the existing single-edition postcode scan. An ordered coordinator starts at most two edition scans, consumes them in edition order, and starts another only after consuming the oldest. Each edition stops after `limit+1` matching records. Cancel and join outstanding work on completion or error; an unused speculative failure must not invalidate an already complete page.

**Constraints:** Keep BM25, HTTP/SQL deadlines, source pool size, address admission slots, source data and indexes unchanged. Bound retained matches to two edition pages plus the output, retain the request-scoped block-cache budget, and preserve source identity checks. The previous frozen-build v14 run stays failed; this is a separate serving fix requiring fresh qualification. No local Docker images or data copies.

- [ ] Add deterministic coordinator tests: actual overlap, bounded in-flight work, chronological output, continuation across editions, page lookahead, required versus unused speculative errors, parent cancellation and worker cleanup. Observe RED.
- [ ] Extract the existing scan into `premisesEdition`; implement the coordinator and integrate the handler. Observe GREEN and run existing premises tests.
- [ ] Run race/unit tests; extend the synthetic database integration checks for cross-edition pagination. Review the complete change.
- [ ] Build code-only on NAS within 2 CPU/2 GiB; compare both serving versions on the frozen address cases and full correctness workload. Keep reports private and export aggregates only. No publication until applicable gates pass.

**Ruling:** A narrow concurrent read is preferable to adding a large source-verifier index: the reproduced serving timeout consists of six sequential record-loading phases. At most two editions may overlap, leaving the source pool unchanged. This is a hypothesis to measure, not a claimed performance fix.
