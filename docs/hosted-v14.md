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

Status: selection implemented and tested; NAS staging/import and acceptance are
in progress. v14 is not yet published to the public website.
