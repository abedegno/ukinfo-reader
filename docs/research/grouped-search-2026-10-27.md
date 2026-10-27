# Grouped cold source reads

The first expanded contained rehearsal of `hosted-2026-10-27.2` returned correct
results with zero candidate errors, but a grouped source-cold request took 3.116 s
against the unchanged 3 s gate. The run failed. All six original and three other
expanded cold checks passed, as did all fixed load gates and 64 full-native checks.
Its 17 baseline errors are retained. The grouped reference is explicitly a frozen
pre-change hybrid snapshot: its full-native offline oracle timed out at ten
minutes. It is separate from the 64 full-native checks.

## Implementation

Count occurrences and resolve the canonical min(dataset,id) source record in the
same covered group-index pass. Metadata ordering still comes from the database's
edition-key collation. Each selected edition contributes every occurrence; no
count, result or cursor cutoff is introduced.

Read scattered source group keys in complete batches of at most 32 IDs, overlapping
at most two queries within the existing eight-connection source pool. Source
record loading, canonical group keys, published visibility, ordering and response
fields remain authoritative. A failed sibling discards the whole partial mapping.
SQL/HTTP deadlines and public quotas are unchanged.

## Retained experiments

The single-pass-only experiment reduced group lookup calls but failed acceptance:
its three candidate cold requests took 2.149 s, 1.700 s and 5.001 s (a timeout).
The old build also exceeded 3 s once. All six results are retained in the numeric
JSON. Three additional source-cold profiles used bounded covered index scans;
warm explained count queries took under 2 ms, compared with cold original calls
of hundreds of milliseconds. No broad scan or spill appeared in those profiles.

After bounding scattered key reads, three balanced candidate cold requests took
2.058 s, 1.841 s and 1.933 s. Their frozen grouped snapshots matched. The paired
old timings overlap at 1.789–2.154 s: these six samples alone do not establish a
tail-latency or capacity improvement, or eliminate interference from shared NAS
storage. Source-clone pages were independently evicted to <=1 MiB before every
request. Target pages and global caches were never flushed.

The initial focused harness attempt failed before making a search because its
aggregate-report directory had not been created. The corrected attempt used a
new write-once label; no measured first request was replaced.

## Validation and provenance

Native integration/race and pinned pg_search 0.26.3 integration/work-budget tests
pass. An independent complete SQL query verifies counts, minimum source records
and edition ordering, including a lower ordinal in a later edition and duplicate
occurrences within one edition. A 129-key fixture verifies the complete mapping,
per-statement work bound and cancellation without partial output or connection leaks.

The binary was compiled from working-tree runtime sources corresponding to the
recorded candidate commit. The runtime digest hashes newline-preserving
`git ls-tree -r --full-tree <commit> server` entries excluding only `_test.go` paths.
Later documentation or test changes can therefore be distinguished from runtime
changes without overwriting the measured provenance.

## Complete contained acceptance

The complete unchanged protocol passed, with zero candidate errors or content
mismatches: 1,416 original request pairs across all 294 cases/two rounds and 99
expanded pairs across twelve cases/two rounds, through up to ten pages. Baseline
failures terminate some paired cursor chains; every first-page case still occurs
twice. These counts are not a guarantee that every case has ten paired pages.
All 64 full-native checks passed. Sixteen baseline errors across comparison/load
phases remain visible, and the original matched 292-case load cohort is unchanged.

Candidate C1 p95 was 120 ms. Maximum repeated candidate C4 p95 was 204 ms. All
candidate initial/measured load requests succeeded without mismatches. All ten
independently cold requests passed the unchanged 3 s gate: original maximum
1.292 s, expanded maximum 2.928 s, grouped request 2.219 s. See the adjacent numeric
JSON for every phase and cold request. The expanded maximum is close to its gate;
NAS I/O variance and limited repeats still preclude an assurance about unmeasured
tails or sustained Internet capacity.

The coordinator used its existing 4-CPU/8-GiB affinity profile. Only the candidate
executable was replaced for this run; workload, source/target settings, quotas,
pools, deadlines, reference checks and cleanup protocol were unchanged. Serving
API/source/target identities were unchanged and no fixture child processes remained.
The first failed contained rehearsal and failed single-pass-only experiment above
remain retained; this acceptance does not overwrite them.

Real inputs, records, coordinates, response hashes and cursors remain on the NAS.
