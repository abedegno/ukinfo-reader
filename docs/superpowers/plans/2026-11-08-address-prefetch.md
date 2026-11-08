# Cross-edition address prefetch implementation plan

> Execute inline with the executing-plans skill; request one independent review after implementation.

**Goal:** Reduce cross-edition address latency while returning exactly the same records, ordering, page boundaries and cursors.

**Architecture:** Extract the existing single-edition postcode scan. An ordered coordinator starts at most two edition scans, consumes them in edition order, and starts another only after consuming the oldest. Each edition emits matched records through an unbuffered stream; the coordinator stops immediately when the page and its lookahead are complete. Cancel and join outstanding work on completion or error; an unused speculative failure must not invalidate an already complete page.

**Constraints:** Keep BM25, HTTP/SQL deadlines, source pool size, address admission slots, source data and indexes unchanged. Retain at most two bounded postcode batches plus the output, retain the request-scoped block-cache budget, and preserve source identity checks. The previous frozen-build v14 run stays failed; this is a separate serving fix requiring fresh qualification. No local Docker images or data copies.

- [x] Add deterministic coordinator tests: actual overlap, bounded in-flight work, chronological output, continuation across editions, page lookahead, required versus unused speculative errors, parent cancellation and worker cleanup. Observe RED.
- [x] Extract the existing scan into `premisesEdition`; implement the coordinator and integrate the handler. Observe GREEN and run existing premises tests.
- [x] Run race/unit tests; extend the synthetic database integration checks for cross-edition pagination. Review the complete change.
- [x] Build code-only on NAS within 2 CPU/2 GiB; compare both serving versions on the frozen address cases and full correctness workload. Keep reports private and export aggregates only. No publication until applicable gates pass.

**Ruling:** A narrow concurrent read is preferable to adding a large source-verifier index: the reproduced serving timeout consists of six sequential record-loading phases. At most two editions may overlap, leaving the source pool unchanged. This is a hypothesis to measure, not a claimed performance fix.


## Execution evidence

Source `7ab9c71` passes all Go tests, eight coordinator tests repeated 20 times under
race detection, independent review and NAS PostgreSQL integration using six full
invented editions. Initial synthetic scratch-mount failure is retained; attempt 2
passed on the exact image.

The four-case address comparison matched every response; maximum latency changed
from 3.930s to 3.301s (small shared-cache sample). All 250 legacy hashes matched.
The complete 355-case load run is not qualified: initial 1 error; C1 zero errors,
p95 287.754 ms; C4 two errors, p95 492.798 ms. All three errors occurred in existing
BM25 postcode searches sorted by name. No address/record-load error was recorded
by the new API. No deployment or v14 publication occurred.

**Ruling:** Preserve this failed full run and investigate the remaining BM25 path
separately. Do not turn passing p95 or a warmed retry into a successful release.

The fixed four-case address contention comparison (baseline/candidate/candidate/
baseline, two rounds at C1/C4 plus initial probes) verified all 80 responses with
zero errors. Warm C4 p95 ranges overlap: baseline 186–217 ms, candidate 159–203 ms.
This small warm sample does not establish a broad speedup.

Actual prepared `EXPLAIN EXECUTE` confirms generic plans already use BM25 Top-K.
A clean source-postcode-ID experiment proves equal complete results on three
cases, but does not consistently improve execution; the broadest case regresses.
Keep BM25 and the existing postcode query. No new index or routing change is
justified by this experiment. The draft remains undeployed pending the separate
postcode timeout investigation and a fresh full release qualification.
