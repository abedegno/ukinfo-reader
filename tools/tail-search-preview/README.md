# Tail latency and coverage evidence

The [research report](../../docs/research/tail-search-2026-10-27.md) describes the
fixed original 294-case comparison, matched 292-case load and separate twelve-case
coverage suite. Real workload inputs and full-native references stay on the NAS.

`ukinfo-parade-trial --stage extend-coverage` creates the deterministic separate
suite from existing matched inputs without running candidate searches to select
successes. It requires a new private output file and emits only scenario shapes
in its manifest. `ukinfo-native-plans --page 10` can diagnose a later cursor page;
it traverses earlier pages privately and exports only plan structure and timings.

After copying only the controller's allowlisted aggregate reports, generate the
value-free summary with:

```sh
python3 tools/tail-search-preview/summarize.py \
  --final private/tail-release-v15-2026-10-27 --prefix tail15 \
  --output docs/research/tail-search-2026-10-27.json
```

The summarizer opens only named aggregate files, validates unchanged cohort sizes,
first-page coverage, contiguous cursor pages and exact per-case load frequencies.
It retains baseline errors and both ordered equal-rate repeats. Candidate errors
make it exit unsuccessfully after preserving the failed aggregate. It never opens
private queries, full responses, hashes or cursor files. Source-only file eviction
is independently verified for nine cold probes; the shared target cache is
uncontrolled. Scheduled runner limits and acceptance are documented as code in
homelab's `docker/ukinfo/regression/`.
