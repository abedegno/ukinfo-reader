# Street-prefix serving candidate — 9 November 2026

**Implemented and correct on the frozen 2003 HTTP cohort; C4 performance still
fails. Nothing deployed.** All 635 responses matched. Three real candidate pages
worked; a separate unchanged-baseline cursor diagnostic timed out on page two.
The SQL spike's improvement does not establish a whole-workload HTTP speedup.

## Implementation and review

[Draft PR #48](https://github.com/abedegno/ukinfo-reader/pull/48) adds one bound,
regexp.QuoteMeta-escaped literal-prefix OR across street and street_2 to the
serving SQL builder. Original scalar predicates, nonempty checks, ordering,
lookahead and cursor seek logic remain authoritative. Exact/absent streets,
native paths and scalar-only diagnostics are unchanged. No index rebuild.

Server source b485487 is deployed ce6117f plus this six-line predicate and tests.
CI selector commit 7a6946c adds the new test to the existing ParadeDB job; server
content is identical. Focused SQL tests were observed red then green; full local
Go suite passed with DB integrations skipped. Independent code/harness review
found no critical/important issue. All 21 reported CI checks passed at 7a6946c,
including the new database integration and desktop builds.

The NAS built exact committed source with Go 1.26.10, max 2 CPU/2 GiB, in 303 s.
Image: sha256:ecf25b2b925bba6aa580e127e822dc4774dc653d1585f021b05b3beba5b36ed2.
The image retains the unchanged deployed binary separately; its code hash was
checked against the live executable. Candidate identity differs and is pinned
to the archived source/context. No local Docker image or database copy.

## Synthetic HTTP and real cohort

Both TestParadeStreetPrefixIntegration and existing TestParadeIntegration passed
on the NAS (74.18 s combined) with disposable Alpine source and separate pinned
pg_search 0.26.3 target. Coverage includes both record kinds, street positions,
empty/both-field rows, punctuation, Unicode, long prefixes, tied names, three
orders, combinations/refinements, full paging and cursor exchange in both
directions against native results. Synthetic owner credentials reach no live or
retained full-edition database. Fixture services stopped before real load.

The same 127 frozen explicitly-2003 cases ran once: 39 business cases, unchanged
query strings/hashes, initial127 plus C1/C4 two rounds each254. The other 228 cases
and all-edition merging remain outside scope. No frozen cursor cases were in
this cohort; additional cursor work is documented separately below.

| Stage | Requests | Errors/mismatches | Median | p95 | Maximum |
| --- | ---: | ---: | ---: | ---: | ---: |
| Initial sequential | 127 | 0 | 43.7 ms | 681 ms | 2,350 ms |
| C1 | 254 | 0 | 6.1 ms | 394 ms | 963 ms |
| C4 | 254 | 0 | 15.2 ms | 1,299 ms | 1,976 ms |

No timeout, unverified case or exhausted budget in this cohort. C1 passes 500 ms;
C4 misses 1,000 ms. The performance gate remains false. Prior errors/gates remain
unchanged; no adaptive quota/code variant or timed load retry occurred. This is
not a paired HTTP comparison or a cold-cache run. Do not attribute small changes
versus earlier runs to the predicate under uncontrolled HDD/cache/host load.

## Remaining tail and cursor evidence

C4 address-family p95 is 1,589 ms across 122 requests; ordered p95 1,195 ms across
112. Other families are much smaller samples. Candidate phases total 82.791 s/
590 calls, records 1.756 s/645 calls. These aggregates may nest/overlap and do not
identify individual C4 traces. Private PG CPU 38.544 s, CFS throttled 32.665 s,
144/525 periods; Docker block reads 753,926,144 bytes. API CPU 2.241 s with no
observed throttling; client CPU 1.107 s with .823 s throttled. Sparse private-PG
activity samples observed DataFileRead in 15 and CPU/running in 34. Both CPU pressure
and variable reads remain relevant; no single causal remedy is established.

The candidate returned three real 50-row HTTP pages with 150 distinct records,
lookahead on every page, and three verified ParadeDB backend decisions. Saved
candidate tokens were then used against the preserved deployed binary with the
same private index/readers/quotas. An initial baseline launcher failed before
requests because its Python import path was missing. A cursor-only continuation
fixed that wrapper without replaying load/candidate reads, then received HTTPError.
The old helper retained no status/body; that failed comparison stays false.

One separately labelled diagnostic captured the same three baseline queries once:

| Page | Status | Time | Comparison |
| --- | ---: | ---: | --- |
| 1 | 200 | 3,678 ms | Matches candidate |
| 2 | 503 | 4,029 ms | query_timeout; no content comparison |
| 3 | 200 | 3,213 ms | Matches candidate |

This capture supports an execution-timeout explanation, not cursor-format
rejection. It cannot retroactively establish the original HTTPError's exact
status/page or turn its failed gate into a pass. Real three-page baseline content
equality remains incomplete. Candidate paging and synthetic compatibility passed;
full real-query exhaustion, other editions and public edge behavior remain untested.

## Conditions, retained failures and cleanup

Real runtime limits stayed PG 1.25 CPU/1,408 MiB, API .5 CPU/512 MiB, helper/client
.25 CPU/128 MiB: 2 CPU/2 GiB. Public API normally has 1 CPU. Synthetic stack used
2 CPU/1.25 GiB. Only one real API ran at a time. Metrics stayed loopback-only;
read-only full-edition/source credentials, no host ports/socket, no storage
migration, Bitcoin pause, Cloudflare change or public v14 publication.

Write-once NAS roots under /home/jonw/ukinfo-backups/ preserve every attempt:

| Root | Retained observation |
| --- | --- |
| street-app-2026-11-09 | Build successful; tests stopped at read-only /tmp before query checks |
| street-app-fixture2-2026-11-09 | New prefix test passed; existing nearby fixture failed on Debian source |
| street-app-source3-2026-11-09 | Correct source stack; both integrations, one 635 cohort, three candidate pages; baseline import-path failure |
| street-app-cursor4-2026-11-09 | Baseline wrapper fixed; comparison HTTPError |
| street-app-cursor-diagnostic5-2026-11-09 | Diagnostic-only status/body capture; original gates remain false |

The fixture corrections supply bounded tmpfs and the exact production Alpine
source image (c293117f...), matching CI's source/target architecture. The earlier
Debian source showed a different nearby fixture result; libc/collation is the
leading explanation, not a controlled proof of a root cause. Invented source
prefix-boundary comparisons were verified before continuing. Guards required
successful cleanup, owned lease, exact failed checkpoint and absent subsequent
workload artifacts. No completed real timed stage was replayed.

Homelab keeps protocol/controller under docker/ukinfo/street-app/. Source3 reports
hold synthetic.json, http-load.json, profile.json, candidate cursor summary and
NAS-private pages; later roots hold baseline failure/diagnostic and private bodies.
Every root retains control/closed.json. Query values, records, IDs, cursors, hashes,
credentials and raw plans remain NAS; numeric local state/logs are gitignored.

Fresh closeout verified all relevant helpers/APIs/fixture/full PG stopped with
no OOM, original full PG 1.5 CPU/1,536 MiB restored, lease released, public health
OK and original image unchanged. No merge or deployment. Next: inspect remaining
broad address/ordered tail and baseline continuation work before another fixed
variant; retain the 1 s gate. All-edition qualification remains required for rollout.
