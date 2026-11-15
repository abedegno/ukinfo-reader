# All-edition name-prefix routing — 15 November 2026

The .15.1 five-edition qualification returned no candidate errors or content
mismatches, but failed a cold coverage probe at 3,584 ms against the unchanged
3,000 ms gate. Aggregate phase counters attributed 2,963 ms to native candidates
and 101 ms to record loading. The failed attempt remains retained.

Private estimated/executed-plan audits found five covering-index shapes and high
heap visibility estimates. Four partitions chose smaller ordinary indexes with
the same key order. Later cached candidate scans performed 19–37 index searches
per edition in under 0.2 ms each. This pointed toward cold index I/O rather than
justifying speculative index creation or maintenance.

A result-path comparison with ParadeDB matched original source records and full
numeric positions. Uncontrolled timings did not establish a winner. The next
control used a fixed native/target/target/native sequence, with zero private
source-file residency before every PostgreSQL restart:

| Episode | Result path |
|---|---:|
| Native | 1,222 ms |
| ParadeDB | 310 ms |
| ParadeDB | 231 ms |
| Native | 1,208 ms |

All four complete results matched the native source reference. Each returned one
record with exhausted continuation. Both pools had two distinct authenticated
readers before timing. Source inventory/generation were prechecked and warmed;
normal inventory checks, vocabulary/candidates and source records remained in
the result paths. HTTP admission/serialization were excluded. The live read-only
target cache was uncontrolled and never evicted. These controls do not reproduce
the original full qualification, prove target-cold speed or establish real-data
continuation behavior. [Operator code and aggregate evidence](https://github.com/abedegno/homelab/tree/main/docker/ukinfo/postcode-parallel)
remain separate from release acceptance.

The candidate change only selects the existing derived result path for all-edition
person searches with a surname prefix, a single-word non-wildcard given name,
default order and no additional filters. Other shapes retain their existing
routes. Original PostgreSQL records remain authoritative; generation/readiness,
refinement continuation, numeric order and native cursor encoding remain normal.
No SQL, indexes, request/statement deadlines, pools or quotas change.

Invented-data integration checks require actual derived routing, all 35 people
across five editions with tied IDs, a 258-person sparse population, first-word
boundaries, exact exhaustion and both native/derived cursor handoffs. Real
pg_search CI and a fresh immutable full five-edition qualification must pass
before rollout. Six-edition qualification and selected v14 cold controls follow
separately; this source change alone does not publish v14.
