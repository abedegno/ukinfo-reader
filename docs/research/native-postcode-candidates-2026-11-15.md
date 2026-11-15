# Native postcode candidates — 15 November 2026

The first .14.5 five-edition qualification failed one plain person/postcode search
at its original two-second deadline. The failure remains retained; no acceptance
or deployment followed. A separate private full-request replay failed cold at
2,001 ms and succeeded on repeat at 1,126 ms. These are observations, not a
reproduction of the qualification's precise cache state.

A bounded source-block comparison found no substantial cold benefit from changing
point branches to edition/block arrays. A direct derived-index postcode route also
failed before completing its first page comparison. Neither change was adopted.

The next control compared the original native candidate query with two concurrent
edition probes. Metadata, vocabulary and two connections were prepared before
candidate-only timing. Source files had zero residency before each private
PostgreSQL restart; SQL planning remained timed. The frozen selection and source
generation stayed identical.

| Episode | Candidate stage |
|---|---:|
| Original query | 751 ms |
| Two edition probes | 669 ms |
| Two edition probes | 580 ms |
| Original query | 775 ms |

All four complete 51-position sequences matched the captured original. Two native
50-record pages, full numeric tie-break tuples, lookahead and encoded continuation
handoffs also matched. The modest benefit supports trying the optimization; it
does not establish a whole-request fix. [Aggregate evidence and limitations](https://github.com/abedegno/homelab/tree/main/docker/ukinfo/search-refinements)
remain separate from release acceptance.

The implementation shares the existing name-prefix reader's two-query limit and
eight-connection source pool. Only plain person/postcode/default-order searches
across multiple editions with no shared term CTE qualify. Every edition supplies
a full page/lookahead before exact merging by postcode/name/first-name/record/
edition IDs. Other filters, business searches, single editions and broad shared
CTEs retain their existing paths. Failed branches cancel siblings and discard all
partial candidates. No request/SQL deadline, pool, quota or result cap is increased.

Invented-data tests independently verify complete populations, original record
bytes, native cursor encoding, tied IDs across editions, empty/short matches,
district/sector/full and Unicode boundaries, exact exhaustion, cancellation while
both connections are held, and an 8,300-term shared-CTE fallback. The full source
PostgreSQL suite, Go vet and race checks pass. Fresh fixed five-edition runtime
qualification, guarded rollout and then fresh six-edition/v14 cold validation
remain required. This change alone does not make v14 public.
