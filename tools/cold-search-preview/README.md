# Cold and concurrent search evidence

`summarize.py` is not a query runner. Use it to validate and combine the
allowlisted aggregate reports from the private NAS trial:

```sh
python3 tools/cold-search-preview/summarize.py \
  --final private/cold-final-v5-2026-10-26 \
  --cold private/cold-validation-v2-2026-10-26 \
  --initial-load private/cold-concurrency-v2-2026-10-26 \
  --output docs/research/cold-search-2026-10-26-results.json
```

The tool never opens workload, response or golden files. It rejects incomplete
case frequencies, missing cold-residency checks, reference mismatches and changed
production lifecycle. Run it with ordinary Python (assertions enabled).

The [report](../../docs/research/cold-search-2026-10-26.md) explains the cache
boundary and the latency/reliability tradeoff. Real queries, cursor tokens,
response hashes and records remain on the NAS. Do not replay these workloads on
the public URL. Invented integration fixtures and deterministic work assertions
run in CI; the real-data protocol is maintained in the homelab repository under
`docker/ukinfo/regression/` and runs only against the existing private source
clone and a SELECT-only derived reader.
