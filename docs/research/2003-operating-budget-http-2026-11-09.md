# Existing-target 2003 HTTP cohort — 9 November 2026

**Warm concurrency thresholds passed; overall qualification still failed.** All
635 fixed requests completed, and 634 responses matched their original hashes.
One initial postcode-only person search in name order returned HTTP 503
`query_timeout` after 4,090 ms. No extra retry was made outside the fixed
635-request protocol. Nothing deployed and v14 remains hidden.

## Scope and fixed controls

Following the [native/SIC comparison](2003-native-sic-comparison-2026-11-09.md),
run all 127 explicitly-2003 cases from the original 355-case workload. Assert
exact equality with the previous scoped cohort, including strings/order/hashes/
digest modes. This includes 39 business cases and zero saved HTTP cursor cases;
the other 228 cases and prior pagination checks remain separate evidence.

Same pinned street-only b485487 executable/image
`sha256:ecf25b2b925bba6aa580e127e822dc4774dc653d1585f021b05b3beba5b36ed2`.
This is the candidate, not the deployed executable; the rejected SIC predicate
is absent. Use existing sealed read-only source/original-derived connections and
pg_search 0.26.3, without database restart, index build or setting changes.

Both existing databases retain 2 CPU/6 GiB. Images, start times and limits were
verified unchanged before/after. Private API retains .5 CPU/512 MiB and
GOMAXPROCS1; worker .25 CPU/128 MiB. Public API has 1 CPU, so this does not test
its exact full resource budget. No local image or database copy/build.

One fixed initial127 + C1 two shuffled rounds254 + C4 two shuffled rounds254.
Exactly635 request budget,20-minute whole-run budget, unchanged 8 s client and
application/SQL deadlines. No warmup, cache flush, exclusions, adaptive variants
or retry until green. Errors remain in percentiles. Gates remain zero errors/
mismatches/unverified cases, no budget exhaustion, C1 p95 <=500 ms and C4 <=1 s.

## Recorded result

| Stage | Requests | Errors | Median | p95 | Maximum |
| --- | ---: | ---: | ---: | ---: | ---: |
| Initial | 127 | 1 | 69.8 ms | 978 ms | 4,090 ms |
| C1 | 254 | 0 | 4.5 ms | 120 ms | 335 ms |
| C4 | 254 | 0 | 10.1 ms | 230 ms | 564 ms |

No content mismatch, unverified case or exhausted budget. The load runner exits
1 and the final gate stays false because of the initial timeout. All successful
responses matched frozen hashes; the failed response is not a successful content
comparison. Initial requests are not guaranteed cold: shared caches were already
exercised by earlier work.
The failing case remains in the two fixed rounds at both C1 and C4; those four
later protocol requests succeeded. They do not erase its initial failure.

Backend counters: 590 `paradedb`, 35 `paradedb_fallback`, five
`paradedb_radius_stream`. These reflect the existing hybrid route; not every
query used the main derived path. Counter totals must not be treated as a trace
that identifies the timed-out request's backend. No per-request plan capture was
made in this run.

An offline helper with network disabled examined only the saved failure and
query structure. It identified a person search with postcode as its sole filter,
name ordering, exact match mode and no grouping. It exported only that structure,
status/code and time. It made no additional HTTP or SQL requests and exported no
predicate, record, identifier, ordinal, cursor or response hash.

## Interpretation and next action

The smaller SIC comparison's warm result extends to the full 2003 cohort's C1/C4
thresholds on the existing target. It does not establish zero-timeout service:
the initial postcode/name timeout blocks qualification.

Earlier private-layout C4 was 1,299 ms with PG1.25CPU/1,408MiB; this existing-target
run used PG2CPU/6GiB. The same API budget and executable help comparison, but
layout, CPU, memory and shared cache/load remain different. Do not attribute the
gap solely to resources or the index layout. No layout choice follows yet.

Next inspect the failing postcode/name path's actual application SQL and plan
under the existing deadlines. Determine whether candidate selection, ordered
retrieval or record loading caused that failure before changing code or widening
the cohort. A matched-budget layout comparison remains separate work. Earlier
failed gates and the draft status of PR #48 remain unchanged; this is not an
all-edition, v14, cold-storage or sustained-capacity qualification.

## Reproduction and closeout

Homelab `docker/ukinfo/operating-budget-http/` preserves the frozen protocol,
controllers, offline classifier and safe numeric evidence. NAS root:
`/home/jonw/ukinfo-backups/operating-budget-http-2026-11-09`. Private queries,
hashes and raw runner evidence stay there. Original attempts remain retained.

All three owned API/load/offline helper containers were verified stopped. The
maintenance lease was released; public health and serving image were verified
unchanged. Existing database processes, images and limits were unchanged; no
database container was stopped or updated. Production configuration is unchanged.
