# Hosted v14 selection and release

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

Homelab holds the reproducible staging, import, indexing, verification and
publication controls. The guarded rollout required complete source, research,
business and derived indexes, plus sealed six-edition search acceptance. The
public archive serves **292,695,740** listings. The six raw reader profiles
total **292,750,609** occurrences.

Real queries, source IDs, records, coordinates and response hashes stay on NAS.
Public smoke uses inventory/health/UI and invented input. Aggregate reports must
separate shared-cache full-source measurements from the older isolated fixture's
cold tests; neither is a claim of sustained Internet capacity. The existing CPU,
memory, pool, timeout and quota limits remain acceptance constraints.

### Public release — 18 November 2026

The selected v14 edition is available in the [public archive](https://ukinfo.6153112.xyz)
and [live catalogue](https://ukinfo.6153112.xyz/api/datasets). It contributes
43,744,633 listings, including 6,092,008 businesses. The deployment runs
`hosted-2026-11-18.4` with ParadeDB.

Private six-edition qualification matched 1,536 sampled fields and 896 paired
pages. The one-client and four-client load tests each served 710 requests with
zero errors, recording p95 of 87.94 ms and 174.54 ms. All six selected-partition
source-cold probes passed, with a maximum of 244.597 ms under the three-second
limit. The dictionary and ParadeDB caches were shared.

The public release passed 12 basic checks, 64 source-reference checks, one
grouped check and public smoke. It also verified the exact edition counts and
CO1 exclusions, then cleared the deployment lease and private verifier sessions.

### Retained rollout history — 15 November 2026

The live `.15.2` application served the existing five editions. Its
five-edition qualification and guarded rollout passed, including measured
first-name/postcode refinements and the selected surname-prefix route.

Selected source projections and indexes are complete for 43,744,633 listings,
including 6,092,008 businesses. The fourth six-edition attempt matched its initial
paired probe and 1,536 sampled fields, then failed four searches during the fixed
355-case preparation. The failed results were retained for comparison.

The remaining prerequisites at that point included compatible PostgreSQL and
extension engines, renewed qualification, a selected-partition cold check with
maintenance exclusion, and a separate six-edition live-reference bundle.

### Retained preparation history — 7 November 2026

On 7 November 2026: the complete selected source is committed and verified,
the derived index and native maintenance are complete, and all 1,536 spread field
samples match both original records and independent source projections. The
[aggregate progress report](validation/v14-2008-03-hosted-progress.json) retains the
initial timeouts and subsequent observations.

Search acceptance blocked publication at that point. Calibration matched
348 of 355 cases; seven native searches timed out, with zero candidate errors or
mismatches. The fresh legacy probe matched all 250 references. These observations
follow one fixed whole-cohort warmup and do not qualify six-edition cold latency.
Concurrent load tests had yet to run. The public site served five editions while
native-reference validation continued privately.
