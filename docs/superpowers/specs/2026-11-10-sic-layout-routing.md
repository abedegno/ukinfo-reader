# Private SIC layout-routing spike

Authorised private trial following the matched SQL evidence, not a release.
Baseline is2ff7886 (b485487 street-only runtime plus SIC regression tests).

Keep one pool/database. Default-off --experimental-sic-unpartitioned flag is
hosted ParadeDB-only. Source API visibility, predicates, sorting, lookahead,
refinements, fallback semantics, response schemas and query/cursor fingerprints
remain unchanged. No UI control or production manifest change.

Approaches considered: two connection pools duplicate inventory/permission logic;
a UNION view risks losing Top-K. Use an optional same-database secondary view
for each complete generation, after independent readiness verification.

For business SIC, no postcode/near/group, normal/name order and already-ParadeDB-
eligible query: require an optional ukinfo_search.sic_editions registry matching
all selected source generations, entries, version and completed watermark; all
fixed generation-derived _sic views must exist. Missing/incomplete readiness uses
original main path before execution. Metadata errors and active candidate errors
return failures without an expensive retry. Never partially switch an all-scope
request. Other query families and flag-off instances never probe this registry.

Use a request-local, unexported edition table override (excluded from generation
JSON) so no shared mutation, generation invalidation or cursor changes. Overlay
metric paradedb_sic_unpartitioned proves route decisions; main paradedb counts
remain comparable. Source data and existing main-index builders untouched.

Tests: policy/default-off and native independence; old generation hash unchanged;
only FROM differs; CLI invalid combinations; actual synthetic registry
missing/partial/stale/alias-missing/readiness, independent native result and full
paging/cursor exchange, active errors do not retry. Existing suites remain.

NAS: exact committed source build max2CPU/2GiB, disposable source/pg_search0.26.3
fixtures; private owner installs only registry/view/read grants over retained
verified baseline copy. No full data copy/index build. Real API readers remain
SELECT-only. PG2CPU/6GiB,API.5CPU512MiB,client.25CPU128MiB. Keep raw queries/rows/
IDs/cursors/hashes NAS. Once correctness/cursors pass, unchanged127case635HTTP
cohort, initial127+C1/C4two254 rounds, original zero-error/hash/C1<=500/C4<=1000
gates. No adaptive rerun/threshold change. Restore private quotas/stop helpers/
verify public/live runtime and release lease; retain on drift. v14 stays hidden.

Implementation sequence: (1) red/green policy/identity/CLI tests and runtime;
(2) synthetic readiness and cursor tests; (3) review, exact-source build and
fixture gates; (4) private setup/cursor proof/full frozen cohort; (5) evidence,
documentation/config as code in both repos. Keeping/promoting this spike needs
its real gates; no automatic merge or deployment.
