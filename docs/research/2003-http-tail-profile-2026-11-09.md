# Private 2003 HTTP tail profile — 9 November 2026

**Completed diagnostic: all 635 responses correct; concurrent tail reproduced.
Nothing deployed or newly qualified.** The evidence prioritises candidate-query
CPU pressure over API CPU, pool queuing or source-record hydration in this run.
It does not prove that raising a quota will fix latency.

## Fixed reproduction

One instrumented run reused the exact deployed executable, unchanged 127 frozen
2003 cases and response hashes, private full-edition partitioned index/API view,
and previous qualification quotas. No tuning variant or timed retry ran. The
subset includes 39 business cases and no HTTP cursor cases; 228 other scopes
remain unqualified. No query/index/production configuration changed.

| Stage | Requests | Errors/mismatches | Median | p95 | Maximum |
| --- | ---: | ---: | ---: | ---: | ---: |
| Initial sequential | 127 | 0 | 6.9 ms | 420 ms | 853 ms |
| C1, two rounds | 254 | 0 | 5.9 ms | 398 ms | 792 ms |
| C4, two rounds | 254 | 0 | 17.0 ms | 1,330 ms | 2,618 ms |

The C4 1 s threshold was missed again. No timeout, unverified case or budget
exhaustion occurred. The faster initial stage than the preceding qualification
is consistent with uncontrolled cache conditions; it is not a measured code
improvement. Original failed qualification and observations remain unchanged.

## Where the time went

Before/after allowlisted application counters recorded:

| Phase | Calls | Aggregate seconds | Mean per call |
| --- | ---: | ---: | ---: |
| ParadeDB candidates | 590 | 73.746 | 125.0 ms |
| ParadeDB inventory | 590 | 1.959 | 3.3 ms |
| Records | 645 | 1.910 | 3.0 ms |
| Native candidates | 35 | 0.080 | 2.3 ms |
| Vocabulary | 75 | 0.064 | 0.9 ms |

All 645 record-phase observations were within 50 ms; 26 candidate phases
exceeded 1 s and three exceeded 2 s. Source-pool waits totalled 14.6 ms over two
waits; derived-pool waits 20.0 ms over three, with zero cancelled acquires.
Cache counters recorded 12,375 misses, zero hits and 6,740 decoded blocks, but the
short record phases do not justify treating this as the dominant bottleneck.
There were zero expanded ordered-name pairs. These are whole-run aggregates,
not per-request traces or C4-only measurements. Nested phases must not be added
into percentages of end-to-end latency.

## CPU and I/O evidence

41 parallel samples covered application, database and client containers. Quotas
remained PG 1.25 CPU/1,408 MiB, API .5 CPU/512 MiB, client .25 CPU/128 MiB: total
2 CPU/2 GiB. The deployed API normally has 1 CPU.

| Container | CPU seconds | CFS throttled seconds | Throttled / total periods | Peak memory |
| --- | ---: | ---: | ---: | ---: |
| Private PostgreSQL | 40.564 | 31.906 | 152 / 426 (35.7%) | 264 MiB |
| API | 2.275 | 0.019 | 3 / 251 (1.2%) | 55 MiB |
| Load client | 1.023 | 0.991 | 16 / 265 (6.0%) | 28 MiB |
| Source PostgreSQL | 3.731 | 0 | 0 / 306 | 115 MiB |

CFS throttled time is a scheduler counter, not additional request latency. The
client's throttling may affect timings, which include transfer, JSON validation
and hashing. An idle completion barrier retained its final live CPU counters.
Client block-I/O coverage was unavailable; other containers had supported counters.

Private PostgreSQL block reads were 72.5 MiB, source 7.2 MiB (Docker accounting
can include storage layers). API-peer activity samples observed private PG
CPU/running in 31 samples, maximum active age 2,109 ms, and DataFileRead in three,
maximum age 802 ms. The source had one CPU/running sample. Sparse observations
do not exclude unsampled I/O waits. Database-wide statistics include other
clients and the observer and are not attributed to these requests.

The strongest next hypothesis is private database CPU contention. A subsequent
experiment should change only its CPU quota under an explicitly declared NAS
budget, preserve this result, use the same frozen workload and retain client
signals. This report does not authorise a resource increase or demonstrate a
causal improvement. Source-address prefetch is a lower priority for this cohort.
Shared HDD/cache state and sampler overhead remain uncontrolled.

## Reproducibility and cleanup

Homelab code commit `5d4758a`, under `docker/ukinfo/http-tail-profile/`, contains
the fixed protocol, reviewed controller and five local helper tests. Four signal
tests also ran during the NAS image build. Image:
`sha256:f94fffae8a657964ea974892d169e9723f4eb23d46926be936f4ed93ca02f528`.
NAS root: `/home/jonw/ukinfo-backups/layout-http-profile-2026-11-09`.
`reports/profile.json`, `samples.json`, before/after metrics, workload summary
and `control/closed.json` retain the evidence. Query strings, source records,
response hashes, credentials and raw data remain NAS-side. Local numeric
log/state remain gitignored under `private/operations/`.

After-metrics were persisted before analysis. Completeness required exactly
127+254+254 verified requests. Owned API/client/private PG are stopped with exit
zero, lease released, and original private PG quotas restored. Fresh public health
is OK on the original serving image. No live database restart, index rebuild,
SSD migration, Bitcoin pause or v14 publication accompanied this diagnostic.
HTTP cursor and multi-edition qualification remain outstanding.
