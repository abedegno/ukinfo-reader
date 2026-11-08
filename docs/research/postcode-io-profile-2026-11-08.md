# Postcode timeout I/O investigation — 8 November 2026

The unchanged original serving executable was tested privately against all 355
frozen cases with independent references. No production image, source/target
settings, index, resource limit or edition visibility changed. v14 remains hidden.

| Stage | Requests | Errors | p95 |
| --- | ---: | ---: | ---: |
| Initial | 355 | 5 | 2,184 ms |
| C1, two fixed rounds | 710 | 4 | 209 ms |
| C4, two fixed rounds | 710 | 2 | 653 ms |

This run failed. A passing percentile does not override errors. Eight failed
requests were postcode-only person searches sorted by name; three were other
address/grouped shapes. The earlier address-prefetch change was not in this binary.

## Evidence

A 244-second numeric-only capture, nominally sampled once per second, produced
235 samples. Median collection overhead was 143 ms; maximum was 2,006 ms.

- Derived search database: 5,289.9 MiB block-storage reads, mean 0.472 CPU cores,
  32.4 seconds of cgroup CPU throttling. Private query samples included 204
  `DataFileRead` waits; the longest sampled query ages, up to 5,037 ms, were reads.
- Source database: 78.8 MiB reads, mean 0.076 cores, no CPU throttling. Its longest
  sampled private active query was 386 ms.
- No lock waits or temporary files were observed in sampled metadata.
- Both databases use the same shared HDD RAID volume. An independent five-second
  snapshot after the run measured Bitcoin reads at about 118.9 MiB/s and Splunk
  reads at 7.8 MiB/s. This is a plausible competitor, not a controlled comparison.
- High host pressure was observed, but a RAM snapshot near completion showed
  about 16 GiB available out of 31 GiB. Existing swap use does not prove host RAM
  exhaustion. Neither database was OOM-killed.

PostgreSQL's absence of a reported wait is not proof of CPU execution. Docker
block-storage counters include mmap/index reads not reflected in PostgreSQL
buffer-read counters. Database-level counters include other connections; wait
sampling was restricted to the private API's client address. Cache state and
other NAS workloads were uncontrolled. Monitoring itself adds overhead.

## Consequence

The observed long waits are dominated by derived-search storage reads. This
capture does not justify replacing BM25, changing query plans, extending timeouts
or raising CPU/memory limits. A fixed comparison with Bitcoin paused briefly
would test the storage-contention hypothesis; the user subsequently approved the
[paused/resumed comparison](bitcoin-contention-comparison-2026-11-08.md). Both warm
phases passed, with cache differences preventing a causal conclusion; Bitcoin
is running again. Preserve this failed run independently
of any later comparison. Fresh full correctness and release gates remain required.

## Operations

Homelab contains `docker/ukinfo/postcode-profile/` with the reader-only runner,
counter filtering tests, analysis and detailed run history. The first container
creation timed out before queries started; inspection confirmed an owned,
never-started lock, which was resumed without duplicating a workload.

All private APIs are stopped, the lease is released and evidence is retained at
`/home/jonw/ukinfo-backups/postcode-profile-2026-11-08`. No real query, record,
source ID, cursor or response hash is included here. Production remains five
editions / 248,951,107 occurrences. This investigation is not release acceptance.
