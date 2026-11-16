# Given-name vocabulary boundaries

The `.15.3` candidate's full five-edition qualification failed with three query timeouts across two surname/given-name/partial-postcode cases. No content mismatches were detected; load and all ten source-cold gates passed. That candidate was not deployed. Its correlated entry filter was fast when cached but showed expensive first reads in an isolated replay.

The replacement keeps the previous entry-membership path and narrows vocabulary lookup to the actual given-name rule: the exact name, or that name followed by a space and middle names. Looking for `J` need not expand unrelated first words such as `Jane` or `John`. Two disjoint, indexable branches precede the ordered vocabulary probe limit. Broad eligible expansions retain the complete word-boundary predicate in PostgreSQL rather than truncating the dictionary. The space-prefix upper bound uses its ASCII successor under the dictionary’s explicit C collation, preserving every valid UTF-8 suffix. Wildcard and prefix modes, record refinement, ordering, cursors, deadlines and pools remain unchanged.

An invented 200,000-term unrelated prefix tail made the new vocabulary-work regression fail against `.15.3`: both custom and generic plans examined 8,193 terms. The test requires at most 32 terms for that fixture. Existing source-work and pagination controls remain, plus a 9,000-term valid middle-name tail to check completeness beyond the probe limit, middle initials and multiword given names.

A private-fixture diagnostic compared the exact `.15.2` source, failed `.15.3` source and a throwaway boundary prototype in ABC CBA order, with two requests per fresh-pool process. It included both failed cases and the earlier one-letter control. All 12 prototype requests returned 200, with a maximum of 638 ms. All successful responses across the variants matched after neutralizing opaque continuation-token contents; `.15.3` still timed out on one control request. Caches were shared and uncontrolled, with no eviction. This is diagnostic evidence, not cold-start qualification or proof of pagination traversal.

The qualification's frozen comparator is distinct from the deployed `.15.2` source; its fast paired timings alone do not establish a release regression. The separate three-way diagnostic above preserves that distinction. Two earlier diagnostic setup failures are retained: an input-user permission mismatch and an absent retired diagnostic binary. Both occurred before search requests; the corrected run checked code-file identity and kept all inputs, SQL and responses on the NAS.

A fresh immutable runtime and unchanged full acceptance remain required. `.15.2` continues serving five editions and v14 remains hidden.

## Verification of the replacement

All 45 catalog tests passed on a fresh PostgreSQL 18.7 instance, including the vocabulary-work gate under both custom and generic plans, 66 paging combinations and the Unicode-boundary regression. The separate ParadeDB integration test requires its own target and is left to hosted CI. Local server `go test ./...`, `go vet ./...` and diff checks pass; independent review found no important issues. A sandbox-denied loopback bind in the initial local run was resolved by running the existing HTTP tests with loopback permission.

The initial full synthetic run exhausted its 512 MB temporary PostgreSQL filesystem with WAL and restarted PostgreSQL; the retained log identifies `No space left on device`. The passing full run used a disposable 2 GB memory-backed filesystem, a 3 GB container memory cap and the same one-CPU limit. This changes the test harness capacity, not the archive qualification workload or gates. No archive data was mounted in that instance.

## Full qualification result

The `.15.4` candidate failed the unchanged full five-edition qualification with one query timeout and no content mismatches; load and all ten source-cold gates passed. It was not deployed. The subsequent [membership-plan investigation](broad-membership-2026-11-16.md) identifies a remaining custom-plan join-order problem for a broad valid given-name expansion.
