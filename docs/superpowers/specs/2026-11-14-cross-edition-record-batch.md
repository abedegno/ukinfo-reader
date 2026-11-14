# Cross-edition record retrieval

The completed private build contains all five editions, 248,951,107 entries.
The first cold postcode request failed at its unchanged 2-second HTTP deadline:
metadata 306 ms, vocabulary 109 ms, candidates 910 ms, records 675 ms before
cancellation. No pool contention was observed. Source queries individually
finished below their SQL deadlines. Native search currently reads editions
serially, even when several editions need only one block each.

Batch native-page record retrieval across editions. Keep four source blocks per
statement and at most two statements concurrently in the existing pool. Read
manifests once for the requested edition set, preserve each edition's archive
identity, and restore the original candidate order and cursor byte-for-byte.
Only decode requested original blocks, use the existing bounded request cache,
and discard all results on cancellation, missing/corrupt data or SQL errors.

Alternatives: extending the timeout hides serial latency; parallel edition
loaders multiply the existing per-edition concurrency limit. Neither is selected.
No new index, source copy, shared record cache, pool enlargement, target routing
change, deadline change, public replay or public promotion is part of this fix.

The user has authorized continuing diagnosis/correction and private qualification.
The batching choice is the implementer's decision under that scope.
