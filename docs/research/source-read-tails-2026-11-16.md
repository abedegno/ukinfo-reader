# Source-read tails — 16 November 2026

The unchanged `.16.1` five-edition qualification failed two requests: an all-edition person partial-postcode search and page two of an all-edition grouped town search. Neither contains a given name; both also timed out in the frozen comparator. There were no content mismatches. Load and all ten source-cold cases passed, but the candidate remains unaccepted and undeployed.

A retained isolated trace found time in native postcode candidate reads and compressed source-block reads, and in source canonical group counts on grouped town page two. All actual input values, SQL, cursor contents and full plans remain NAS-only.

## Bounded group-member reads

The implementation retains the authoritative covered source index, scalar edition pruning, complete counts and min(dataset,id) representative. It deduplicates keys before splitting, preserving SQL ANY set semantics; keys are sorted without changing caller input. Each query receives at most 16 keys, and at most two queries overlap. Edition labels retain the database's key ordering. SQL/HTTP deadlines, source/target pool sizes and the query itself are unchanged.

An invented 65-group, three-edition fixture supplies 40 duplicate keys across batch boundaries. The old implementation failed the work gate with 105 keys per read in both planning modes. The replacement passes complete counts, canonical records, edition ordering, empty/missing input and cancellation/pool-cleanup checks. All 47 PostgreSQL catalog tests, full Go tests and vet pass; independent review found no important issues.

A private ABBA comparison covered both retained failure shapes, first page and continuation in fresh API/pool processes. All 16 requests completed and full responses, including exact cursor contents, matched. On grouped pages, member-query size fell from 50 keys to at most 16, with 20 bounded queries instead of five. Warm timings were broadly neutral with some overhead: this establishes bounded statements and equivalence, not a cold-tail fix. Caches were shared and uncontrolled.

## Rejected postcode routing spike

A throwaway overlay routed only all-edition plain person/postcode/default-order searches of at least three postcode characters through the existing derived numeric-cursor path. Both compared versions included group batching, isolating the routing change. The ABBA controls included the original failed prefix, another original prefix, derived six/seven-character full-postcode controls from retained queries, and the unchanged grouped-town control. No actual inputs or frozen acceptance cases were modified.

All eight postcode requests through the proposed route timed out near two seconds; all 16 native postcode requests completed. Grouped-town control responses matched. The routing change was rejected and is not in production code. Native postcode routing remains intact.

Both custom and generic estimated prepared target plans used `TopKScanExecState`, so the result is not explained by a plain PostgreSQL sort fallback. The pinned upstream [sort-feature limit](https://github.com/paradedb/paradedb/blob/v0.26.3/pg_search/src/index/reader/index.rs#L77) is five; the four per-edition postcode ordering columns do not exceed that limit. An older three-column-limit explanation would be incorrect for this deployed version.

Next, measure the native source-record path with an isolated source-only cold diagnostic before adopting another postcode change. Any successful prototype still requires regression tests, independent review, a fresh immutable runtime and the original complete acceptance. The public service remains `.15.2`, five editions; v14 remains hidden.
