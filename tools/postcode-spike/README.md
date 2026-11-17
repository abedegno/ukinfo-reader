# Private postcode query comparison

An operator-only diagnostic for the two postcode-only person searches, ordered by name, retained from P8. It imports the unchanged catalog package and uses nonprivileged private source/target readers. It is not a serving backend, reference qualification or cold-performance test.

The tool captures each exact first-page target statement through the real API handler and cancels it at the query tracer. It changes only the redundant postcode regex prefilter to a bound non-null check; scalar postcode range/precision predicates, parameters, ordering and limits remain unchanged. It suppresses catalog standard logging in the standalone process and enforces cached prepared execution even if DSN settings conflict. Primary and fallback destinations must use the same approved host/port. It then executes eight actual prepared queries, using prefilter/scalar/scalar/prefilter for the first case and the reverse for the second. Ordered row digests stay in memory; the aggregate reports equality, row counts, outcomes and timings without values, hashes, SQL or case identifiers.

The measurement pool opens after the capture pool closes. Source/target pools retain their 8/4 bounds; sessions are read-only, source statements use 2 seconds and target statements 4 seconds with generic prepared plans. Connection setup is limited to five seconds and acquired before timing; its duration is reported separately. Total context is 60 seconds. No database setting, index or public visibility is changed. No telemetry exporter is initialized; catalog standard logging is suppressed within the standalone run. DSN execution-mode/cache overrides cannot disable prepared queries, and every fallback must retain the approved destination. The measurement connection is acquired before timing, with setup time reported separately. Caches are shared and uncontrolled; counterbalancing reduces fixed ordering bias but does not reproduce or attribute P8's original failures. Equality here covers these candidate rows only, not the full API/reference/cursor corpus.

Use the reviewed homelab controller with NAS-private runtime/subset files and exact source/main CI. Standalone invocation requires `DATABASE_URL` and `TARGET_DATABASE_URL` for the approved private reader identities. Both paths must be absolute; input must be a private regular file containing exactly two supported cases. The output is reserved exclusively before connecting to the database, written as mode0600, and never overwritten. Capture/comparison failures retain an incomplete aggregate when an output path was reserved; raw errors are not printed.

```sh
./postcode-spike --cases /work/cases.private.json --output /work/comparison.json
```

Both strategies use the exact supported SQL structure and verified canonical postcode arguments; conjunction or extra-clause changes are rejected.

The separate module lives outside `server`, preserving the serving source digest. Build with Go1.26.11, `GOOS=linux GOARCH=amd64 CGO_ENABLED=0`, `-trimpath`; verify the ELF header and Go build settings before NAS upload. No local Docker image or archive/database copy is required.

```sh
cd tools/postcode-spike
go test -race -count=1 ./...
go vet ./...
```
