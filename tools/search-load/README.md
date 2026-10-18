# HTTP search load generator

This standard-library Go tool measures the actual search API. It accepts a **private**
JSON array of `{ "path": "/api/search", "query": "dataset=...&kind=person&surname=...", "sha256": "..." }`.
Validate every case's result count, source identity, fields and filters against an
independent reference before recording its exact response-body SHA-256. Cases contain
search values and must stay under `private/`; do not commit them or response bodies.

```sh
cd tools/search-load
go run . -url http://127.0.0.1:8080 -cases /path/to/private-cases.json \
  -output /path/to/aggregate-report.json -duration 10s -arrival-duration 30s -rate 100
```

The tool warms every case, tests 1/8/32 closed-loop callers and then schedules arrivals
at the requested rate. Scheduled latency starts at the intended arrival time and
includes generator queue delay; drops and response errors are counted. Every timed
response is body-hash checked. HTTP/body transfer and hashing are included; client
JSON parsing is excluded. Percentiles include failed attempts, so do not interpret
latency without errors/drops. The bounded generator is not a distributed load tool.

Reports contain only aggregate measurements. Record corpus size, sample method,
hardware, runtime versions, server/client CPU and memory quotas, connection-pool size,
warm/cold conditions and test duration alongside results. A short repeated-key test
on one machine does not establish internet or whole-corpus production capacity.

`path` is optional for older fixtures (defaults to `/api/search`). The allowlist also
accepts `/api/export`, `/api/group`, `/api/address` and `/api/record`, so mixed searches
and page exports can share one run. Validate each response before recording its hash.
Reports expose only the number of cases per fixed endpoint, never queries or hashes.
The HTTP request timeout now defaults to eight seconds, exceeding the API's five-second
search budget; override with `-request-timeout`. Older reports used a three-second
client timeout. Do not compare failure rates without accounting for that difference.
