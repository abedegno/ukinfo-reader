# Hybrid source retrieval — 14 November 2026

The patched-compiler candidate's fixed private qualification failed with three
request errors and a grouped cold probe above the unchanged three-second gate.
The failure remains recorded in [homelab](https://github.com/abedegno/homelab/blob/main/docker/ukinfo/go-http-security/README.md).
No public promotion is implied by the changes below.

## Evidence and changes

One declared private diagnostic profiled the three retained shapes with one
source-only cold start and one warm repeat per shape, using the original serving
budgets. Plans were disabled between repeats; target cache remained shared and
uncontrolled. No saved searches were replayed through the public hostname.

| Shape | Source-cold diagnostic | Warm repeat | Dominant work |
|---|---:|---:|---|
| Household across editions | 1,445 ms | 25 ms | Eleven metadata and eleven record queries; sequential edition loading |
| Grouped town across editions | 2,783 ms | 152 ms | 195 grouping-key queries, about 2,533 ms summed SQL time |
| Surname, fourth page | 333 ms | 8 ms | Record reads, about 332 ms wall time |

All diagnostic requests succeeded. These observations do not replace the failed
qualification or prove that the surname failure is fixed. The household's request
scope is **all editions**; its original failure summary mistakenly described the
anchor edition as the requested scope.

The earlier global record loader was adopted only by ordinary native pages.
Ordered/grouped and ordinary ParadeDB pages now use that same loader: up to four
source blocks per SQL statement and two concurrent statements across the page.
Original bytes, archive/source identity, result order, refinement checks and
cursors remain authoritative. The request cache remains bounded at 4 MiB.

Complete indexed household lookups collect an ordered candidate page across
editions before loading records together. Edition date order is explicit, since
import IDs are not chronological. Every candidate and lookahead still undergoes
original displayed-address validation. Rejected keys advance only the scan;
continuation remains on the last accepted source. Missing/partial optional index
coverage retains the complete legacy postcode scan.

The optional source group-key covering index supplies `group_key` directly from
`(dataset_id,id) INCLUDE(group_key)`. This avoids the heap lookups required by the
existing primary key. It changes neither grouping identities nor source rows.
It is operator maintenance, never a migration automatically run at API startup.

## Maintenance and validation

Run one selected edition at a time as the source catalogue owner:

```sh
psql "$OWNER_DATABASE_URL" -v dataset=YOUR_EDITION_KEY \
  -f deploy/production/group-key-indexes.sql
```

The script requires complete matching research coverage, validates the attached
source partition and any prior index, and builds concurrently with 256 MiB
maintenance memory and no parallel maintenance workers. An invalid/incompatible
existing index fails closed; investigate it rather than silently retrying or
dropping it. Retain existing indexes. Size and build duration depend on the host;
complete the guarded private measurement before public maintenance.

`tools/check_group_key_indexes.py` runs a disposable network-isolated PostgreSQL
fixture with 50,000 invented rows in each of two editions. Its negative control
fails without the covering index. Both custom/generic plans then use index-only
scans with zero heap fetches and 97 buffers for 32 scattered IDs. It also checks
exact results, idempotence, explicit edition isolation, incompatible-index rejection
and incomplete research coverage rejection. The search CI runs this check.

New source and ParadeDB regressions fail the old five-query edition loops and
pass with two block statements. Household comparisons include mixed business and
person listings, import order differing from edition order, false-positive keys,
exact pagination equivalence and mid-read cancellation without partial results.
The full invented source catalog and ParadeDB integration suites pass, as do Go
unit tests/vet and targeted race checks. Synthetic source and target databases use
the supported C-collation configuration; the first combined fixture attempt
exposed an unrelated locale mismatch and is not counted as a passing run.

Private covering-index build and fresh unchanged qualification are still pending.
No timeout, pool size, acceptance client, reference, public quota or trace sampling
has been increased. No database/archive copy or original-record rewrite is needed.
