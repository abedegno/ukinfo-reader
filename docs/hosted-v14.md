# Hosted selection and import of v14

The raw archival reader validates 43,799,502 v14 source occurrences. The hosted
selection is explicitly different: the disc's own `data_sources.htm` describes CO1
as the publisher's record-removal request mechanism, and its original constructor
names postcode control bit `0x40` CO1. The bit's physical decoding was checked
against original routines. Complete Windows UI filtering has not been executed;
the hosting policy below is explicit rather than an inferred UI equivalence.

The [complete audit](validation/v14-2008-03-hosted-co1-selection.json) counts
29,410 flagged physical records supplying 54,869 occurrences. The hosted export
excludes every flagged residential occurrence, including both electoral and
telephone components. The archival reader and original files remain unchanged.

| Scope | Source | Excluded | Hosted |
| --- | ---: | ---: | ---: |
| Electoral | 26,963,431 | 40,694 | 26,922,737 |
| Residential telephone | 10,744,063 | 14,175 | 10,729,888 |
| Business | 6,092,008 | 0 | 6,092,008 |
| Total | 43,799,502 | 54,869 | 43,744,633 |

## Export contract

```sh
python -m ukinfo_reader.search_export \
  --data private/v14-reader --selection exclude-source-co1 --identify
```

The streaming command uses the same arguments without `--identify`. It emits
protocol 2 with dataset key `v14-2008-03:co1-excluded`, an explicit `selection`
object, original file fingerprints, selected type counts and a fresh digest over
only selected rows. Unknown listing types, missing/non-boolean residential CO1
flags, source counts, exclusion counts or emitted counts that differ from the
verified policy prevent a valid terminal frame. The Go importer rolls back an
incomplete stream. Explicit samples retain their sample labels and cannot satisfy
the publication checks. The ordinary CLI and default archival export remain raw.

No intermediate complete JSONL file is needed. The NAS importer pipes selected
rows directly into the existing Go catalogue importer, retaining original source
IDs and source flags. Complete catalogue verification checks that stored blocks
and projections match the selected stream's digest. No independent identity
matching or retrospective cross-edition removal is inferred from this source bit.

## Operations and acceptance

Homelab holds the reproducible staging/import/index/verification/publication
controls. Keep v14 hidden from the serving role until all source/research/business
and derived indexes, verification and six-edition search acceptance finish. The
existing five editions and hidden 2003 rollback copy are retained. The expected
six-edition hosted total is **292,695,740**; the six raw reader profiles total
292,750,609 occurrences.

Real queries, source IDs, records, coordinates and response hashes stay on NAS.
Public smoke uses inventory/health/UI and invented input. Aggregate reports must
separate shared-cache full-source measurements from the older isolated fixture's
cold tests; neither is a claim of sustained Internet capacity. The existing CPU,
memory, pool, timeout and quota limits remain acceptance constraints.

Status on 7 November 2026: the complete selected source is committed and verified,
the derived index and native maintenance are complete, and all 1,536 spread field
samples match both original records and independent source projections. The
[aggregate progress report](validation/v14-2008-03-hosted-progress.json) retains the
initial timeouts and subsequent observations.

Publication remains blocked by strict search acceptance. The first attempt matched
348 of 355 calibration cases with seven native timeouts and no candidate errors or
mismatches. A slower private native reference subsequently completed 851 paired
pages: 846 matched, five native requests exceeded 120 seconds, and the candidate
had no errors or mismatches. That attempt also retained six candidate preparation
timeouts and one timeout across 250 legacy references. These are retained failed
attempts, rather than passed acceptance or cold-latency qualification.

A fresh private reference now enumerates the complete matching source address
population before applying the original native ordering, grouping and cursors.
Nearby searches independently apply the authoritative source postcode geography to
every source member. No candidate record IDs or results select this population.
Incomplete indexes, duplicate/invalid IDs and populations over eight million fail
closed; results are never truncated to meet a deadline. The method passed 374
complete-page/cursor comparisons against the stock native path using six invented
editions on an isolated NAS PostgreSQL instance.

The complete-source reference improved the paired result to 876 matching pairs
out of 878 across all 355 cases, with zero candidate errors or mismatches. Two
native town searches still reached 120 seconds, so the run was stopped before
further cursor/load work. Its two preparation timeouts and all reports remain
retained. A private follow-up batches source labels and grouping keys, then keeps
the original native SQL for grouping, ordering and cursor pagination. It must pass
fresh exact-image synthetic and targeted continuation tests before another full
qualification. No serving code or candidate access path changes.

Every frozen case remains required in the fresh real-source qualification.
The private native request/SQL budget is 120 seconds; public/candidate deadlines,
resources, serving binary and normal performance client remain unchanged. Native
reference timings are not serving-performance measurements. Publication still
requires full source verification, all 355 cases, paging/cursors, all 250 legacy
references, and unchanged zero-error C1/C4 load gates at p95 <=500/1000 ms, followed
by public inventory, internal hosted and frozen release-client checks. The public
site continues to serve five editions while these checks run.

The next private image passed 374 synthetic page/cursor comparisons and prepared
1,179,789 person occurrences from a consistent source snapshot in 515.595 seconds,
using 77,834,516 bytes against a 128-MiB cache bound. Both pages of the two retained
shapes matched (four pairs, zero errors/mismatches). Source-generation checks
reject concurrent changes. This targeted result did not qualify the full suite.

The subsequent full attempt hit the private native service's 512-MiB hard memory
limit at case 91; its archived reports contain 473 pairs, 264 native errors and
one candidate error. The controller refused to continue with the dead service.
A same-binary experiment lowers its Go soft target from 384 to 256 MiB to test
encoding/buffer headroom, repeating bounded source preparation. No hard resource
cap, deadline or public/candidate setting is relaxed. v14 remains private.

That memory experiment also failed on the fourth grouped page. Its full report
retains 475 pairs: 211 equal pairs, 264 native errors and zero candidate errors or
mismatches. The repeated source preparation passed in 436.584 seconds; the fixed
candidate preparation retained seven errors. Neither attempt qualifies acceptance.

The next private reference changes transport only: six projection columns use
22 fixed-width bytes per occurrence rather than 46 bytes in PostgreSQL binary
arrays with per-element lengths. PostgreSQL decodes the values before applying the
same native predicates, grouping, ordering and cursor SQL. Additional packed
storage counts against the same 128-MiB cache cap. Exact-image isolated checks
passed all 374 page/cursor comparisons, integer-width boundary decoding and
source-generation race/invalidation tests. A new source snapshot and targeted
continuation proof precede another complete qualification. Hard limits, deadlines,
all 355 cases and the publication gates remain unchanged.

The packed reference passed fresh source preparation (518.647 seconds and
103,789,874 accounted bytes), all four targeted pairs and all 1,536 field samples.
All 355 candidate preparation requests ran, retaining three errors. The full
comparison was operator-stopped shortly after starting to investigate a separately
reproduced driver retention issue. This attempt was not OOM-killed and is not
completed full acceptance; its partial reports and both image proofs are retained.

An invented 4-MiB parameter query followed by a smaller query retained 4,194,308
bytes in unused driver parameter slots. A one-line reset fix clears those
references in the private correctness build only. Its dependency source has an
exact checksum guard and uses a private vendor copy; the shared module cache,
serving binary and normal load client remain unchanged. Review confirmed parameter
bytes are copied into the protocol buffer before reset. The new exact image
passed the driver regression, SQL boundaries and all 374 synthetic page/cursor
comparisons. Fresh source/full gates are still required to establish the actual
memory improvement. No hard cap, deadline or case has been relaxed.
