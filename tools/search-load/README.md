# HTTP search workload runner

This standard-library Go tool measures the actual API, including response transfer,
JSON validation and response hashing. Store its private case file under `private/`;
never commit query values, response bodies or reference hashes.

Each case has `path` (default `/api/search`), `query`, and `sha256`. The endpoint
allowlist also accepts `/api/export`, `/api/group`, `/api/address` and `/api/record`.
Exports are validated as NDJSON. Before recording a reference hash, independently
validate counts, identities, fields, filters and ordering.

```sh
cd tools/search-load
go run . --url http://127.0.0.1:8080 --cases /path/to/private-cases.json \
  --output /path/to/report.json --repeats 2 --concurrency 1,2,4,8 \
  --duration 10s --rate 2 --arrival-duration 30s \
  --max-requests 2000 --max-duration 5m
```

## Stages and budgets

1. Sequential observations run **before warming**. `--repeats` adds later rounds;
   the first observation does not imply a cold database or disk.
2. Closed-loop clients run at each specified concurrency (default `1,8,32`).
3. Scheduled arrivals run at `--rate` requests/second, including generator queue
   delay in latency. `--rate 0` skips this stage.

`--probe-only` runs only sequential observations and permits missing hashes for
exploration. These successes are explicitly **unverified**, not correctness passes.
Load mode requires every reference hash. Failures are retained and do not silently
remove slow cases from subsequent measurements. `--request-timeout` defaults to
8s, longer than the API's 5s refinement budget.

All stages share `--max-requests` (default 10,000, maximum 100,000) and
`--max-duration` (default 5m, maximum 30m). Budget exhaustion or any failed request
saves the report and exits unsuccessfully. The runner refuses redirects and limits
response bodies to 8 MiB. Use a private endpoint for capacity tests so perimeter
quotas do not masquerade as database limits.

## Correctness across imports

The default digest is SHA-256 of the exact response bytes. Optional
`"digest_mode":"cursor-neutral-v1"` replaces only the opaque `next_cursor` string
with a boolean indicating another page, then hashes the top-level JSON object with
sorted keys. Other field bytes, number representations and item ordering remain
significant. This handles physical dataset IDs changing inside opaque cursors on
reimport. It does **not** validate following those cursors: test complete pagination
separately. Do not regenerate expected hashes from the implementation under test.

## Interpreting reports

Schema 2 reports include every sequential observation by case number/round, fixed
query-family and edition/all-edition scope; per-case/family percentiles; status and
outcome counts; closed-loop stages; scheduled-arrival drops; and budget exhaustion.
They omit queries, bodies, hashes, server URLs and raw error messages. Inspect
reports before publishing them; private case indices can still identify workloads.

Percentiles include failures. Read latency alongside errors, drops and throughput.
Record corpus size, sample selection, versions, hardware, index/visibility state,
client/server CPU and memory quotas, pool size, background imports and cache history.
A repeated small workload does not establish whole-catalogue Internet capacity.
The runner never flushes caches or restarts PostgreSQL.

## Paired API comparison

Use two private APIs on the **same fixed catalogue generation** to check an adverse
result under similar storage conditions:

```sh
go run . --url http://candidate:8080 --compare-url http://baseline:8080 \
  --probe-only --cases /path/to/private-cases.json --output /path/to/paired.json \
  --repeats 2 --max-requests 400 --max-duration 15m
```

This mode runs baseline/candidate sequentially for each case and alternates which
goes first by case and round. The request budget includes both servers. It compares
both successful responses using the case digest mode and retains mismatches,
before-only/after-only successes, failures and incomplete pairs. Missing reference
hashes are permitted; matching implementations do not establish independent
correctness. Existing expected hashes are still enforced for each response.
The command fails on candidate errors, content mismatches or incomplete work.

Schema 3 includes per-pair observations and baseline/candidate summaries, without
URLs, query values, response records or hashes. Summary request rates divide by
the entire paired run, including the other server's work; they are not independent
capacity measurements. Alternating order reduces cache/order confounding but does
not isolate changing background I/O. Keep corpus generation and resource limits
fixed, record maintenance, and investigate differences before deployment.

### Paired pagination

Add `--compare-pages 3` (range 1–10, default 1) to a paired, probe-only run to
follow each server's own cursor. Paging stops on exhaustion, mismatch or failure.
Later pages compare complete response digests without reusing a first-page
reference hash. Query strings, cursors, records and hashes stay private. The
report adds a `page` number and `max_pages`; compare the fixed first-page cohort
separately from the variable number of successful later pages. Include later
pages in the total request/time budget.
