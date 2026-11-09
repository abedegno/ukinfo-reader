# Private street-prefix SQL spike — 9 November 2026

**Promising result for one SQL shape: original 604/558 ms, prefiltered 178/195 ms,
with synthetic and real-page equality checks passing.** No serving code changed;
HTTP performance, opaque cursors and other editions remain unqualified.

## Change and fixed protocol

Keep the original scalar predicate, including nonempty street/street_2 checks,
both prefix ranges, kind and exact name/first_full/id ordering. Add a redundant
OR of `street @@@ pdb.regex($n)` and `street_2 @@@ pdb.regex($n)`, binding the
normalized literal prefix after regexp.QuoteMeta-compatible escaping plus `.*`.
No raw user regex syntax, changed ordering, result cap or index rebuild.

The only real query is the previously failed 2003 street-prefix/name-order case.
Its text and identity remain NAS-side. First require synthetic equality against
an independent plain-SQL oracle, then three matching real keyset pages. Only
then run the predeclared original/prefix/prefix/original sequence once, four
EXPLAIN ANALYZE/BUFFERS observations with 4 s deadlines. No adaptive extra run.
Validation warms caches; this is not a cold-storage or HTTP concurrency test.

## Semantic checks

28 synthetic cases across 132 fixture rows passed baseline, candidate and plain
SQL equality, including 108 combined baseline/candidate page traversals. Coverage:
person/business kind isolation; first/second/both street fields; empty fields;
no-match prefixes; escaped punctuation; Unicode; long prefixes; tied names;
continuation order, duplicates and end of pages. The fixture has its own tiny
PostgreSQL/index and synthetic owner credential, isolated from full-edition writes.

Three real 50-row pages plus lookahead matched exactly across variants: 150
distinct displayed candidates, correct order and no duplicate continuation rows.
These compare all six candidate projection fields. This is SQL keyset validation,
not production HTTP cursor-token traversal or complete real-query exhaustion.
Local literal-escaping and continuation-parameter checks also passed before launch.

## Fixed full-edition observations

| Order | Variant | Execution | Shared hits | Shared reads | Total accesses |
| --- | --- | ---: | ---: | ---: | ---: |
| 1 | Original | 604.400 ms | 15,593 | 18,081 | 33,674 |
| 2 | Prefix | 177.511 ms | 9,556 | 9,127 | 18,683 |
| 3 | Prefix | 194.974 ms | 4,174 | 14,509 | 18,683 |
| 4 | Original | 557.715 ms | 23,196 | 10,478 | 33,674 |

All four succeeded and used TopKScanExecState; each returned/fetched 51 heap
candidates, included all four segments and recorded zero temporary blocks.
The two-observation mean fell from 581 to 186 ms (about 3.1 times faster here).
Shared-buffer accesses fell 44.5%. Physical disk reads are not established by
PostgreSQL shared-read counts; their ranges overlap between variants. Shared
HDD/OS cache, host load and execution overhead remain uncontrolled. Four sequential
observations do not establish a percentile, sustained capacity or C4 improvement.

The generic plan retains five original postgres_expression clauses and adds
two parameterized regex-query builder expressions under an OR. Its serialized
Tantivy query does not display a static regex node. Redacted inspection confirms
the added expressions refer to the street fields, regex function and parameter.
Do not claim the serialized plan proves a particular FST execution path. Measured
lower latency/work and equality support a serving implementation trial, not a
proven engine-level causal explanation.

## Resources, retained failure and cleanup

Peak limits were 2 CPU/2 GiB: full private PG 1.25 CPU/1,408 MiB; synthetic PG
.5 CPU/512 MiB; helper .25 CPU/128 MiB. Existing pinned images, no build or local
DB/image copy. Full-edition access used only the sealed SELECT-only target role;
no source-network access, source credential, Docker socket or host ports.

The first attempt stopped at extension setup: SQLSTATE 42704, bundled vector
dependency missing. No synthetic checks, real queries or timed observations ran.
That failure and successful cleanup remain retained under
`/home/jonw/ukinfo-backups/street-prefix-2026-11-09`. A guarded new namespace used
CREATE EXTENSION pg_search CASCADE in the isolated fixture DB. It verified the
exact prior failure and absent workload checkpoints before continuing; it did
not repeat a timed stage. Original failed qualifications remain unchanged.

Completed root:
`/home/jonw/ukinfo-backups/street-prefix-bootstrap2-2026-11-09`.
Evidence: `reports/synthetic.json`, `real-pages.json`, four observation summaries,
four private raw plans, `summary.json`, and `control/closed.json`. Query values,
real rows, IDs, credentials and raw plans remain on NAS. Numeric local log/state
remain gitignored under private/operations. Homelab protocol/controller/helper:
`docker/ukinfo/street-prefix/`, initial commit e0c7362, bootstrap fix a6ef940.

Fresh verification confirmed full private PG, fixture PG and SQL helper stopped
exit zero, original full private PG 1.5 CPU/1,536 MiB restored, lease released,
public health OK and serving image unchanged. The separate read-only inventory
supervisor was boundedly stopped (exit 137, not OOM). No source/live restart,
full-index rebuild, storage migration, Bitcoin pause or v14 publication.

Next: implement the redundant condition in the serving SQL builder with focused
regressions, then qualify the exact candidate binary against the unchanged HTTP
corpus and cursor checks before any rollout. Keep the 1 s C4 target and original
failures; this SQL spike cannot reclassify them as passing.
