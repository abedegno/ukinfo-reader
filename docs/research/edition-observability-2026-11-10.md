# Per-edition observability preparation — 10 November 2026

[Draft PR #51](https://github.com/abedegno/ukinfo-reader/pull/51) adds privacy-safe
edition query context to ordinary OpenTelemetry and aggregate metrics. It is
based on deployed `ce6117f`, separate from the unqualified street/SIC/layout
experiments and the NAS-only diagnostic SQL logger. No production deployment,
Collector apply, Terraform apply or v14 publication occurred.

The [diagnostic runs](broad-failure-evidence-2026-11-10.md) did not reproduce the
original broader timeouts. The original failed qualification remains failed.
This change prepares better evidence for a future genuine failure; it is not a
performance fix or a claim that a slow request's physical I/O cause is known.

## Contract

Each positions query and pair-drain query creates `search.parade_edition` beneath
its search/request context. Attributes are bounded `ukinfo.backend=paradedb`,
`ukinfo.edition`, `ukinfo.parade.stage` and `ukinfo.outcome`. Only six known full
archive keys or `other` are exposed. Stages are `positions`, `pair_drain`, `other`;
outcomes are `ok`, `deadline`, `canceled`, `query_canceled`, `error`. SQLSTATE57014
is server cancellation, not proof of a statement timeout. No SQL, arguments,
record IDs, PIDs, source fingerprints, arbitrary import keys or raw errors.

`ukinfo_parade_edition_queries_total{edition,stage,outcome}` counts every completed
execution, including failures with tracing disabled or head-unsampled requests.
`ukinfo_parade_edition_duration_seconds{edition,stage}` includes all outcomes,
using existing histogram boundaries. Timers include acquisition, row consumption
and close, excluding subsequent span export/metrics-lock wait. Parallel durations
overlap; query failures/canceled siblings are not failed HTTP-request counts.
Maximum new Prometheus cardinality is 315 series, allocated only when observed.
The existing 2,000-sample scrape cap and head sampling policy are unchanged.

Both project Collector variants retain and normalize the new attributes/labels,
strip unknown attributes and convert edition latency histograms to delta buckets.
Fixtures include unknown string and numeric canaries, requiring both to survive
as bounded `other`/`error` values. Existing phase fields are retained in the log-
enabled variant as well. Terraform adds edition/stage p95 and execution-outcome
charts; percentile values remain histogram-bucket estimates.

Histogram metrics cannot identify one request, and a particular failed trace may
still be absent under head sampling. Correlate edition/error counters and sampled
child spans with infrastructure disk/CPU/memory signals. SQL builders, filters,
ordering, cursor fingerprints, pool limits, deadlines and source visibility are
unchanged. Query-path tests cover both successful and canceled positions/pair
execution, complete row lifecycle and once-only accounting.

## Validation

Feature head `24c4f2e84651a3bf79dec4a1f717c6780c2be90a`; all four latest PR
workflows passed:

- [Search application](https://github.com/abedegno/ukinfo-reader/actions/runs/42901792962):
  both real Collector variants, including numeric/string canary normalization,
  Go vet/race/native database/browser checks and disposable ParadeDB integration
  with successful/canceled positions and pair-drain accounting.
- [Tests](https://github.com/abedegno/ukinfo-reader/actions/runs/42901792332):
  lint/format plus Python 3.10/3.14 on Linux, macOS and Windows.
- [Observability configuration](https://github.com/abedegno/ukinfo-reader/actions/runs/42901792308):
  Terraform format, locked-provider initialization and validation; no apply.
- [Desktop packages](https://github.com/abedegno/ukinfo-reader/actions/runs/42901792567):
  Linux x64, macOS arm64 and macOS x64 packages passed; release publication skipped.

Local full Go tests/vet and catalog/telemetry race tests also passed. An initial
formatter failure was corrected with pinned ruff 0.14.0. Superseded/duplicate push
runs were canceled; they are not substituted for the successful final PR runs.
Homelab release-controller tests (24) and regression-policy tests (9) passed;
its unchanged selected legacy release manifest validates offline. Static policy
comparison confirms both project variants and homelab accept all six known keys
plus `other`, reject unknown keys and have identical trace privacy clauses.

Independent code review found and corrected one timing issue before validation:
measure elapsed at query completion before span export and the metrics mutex, so
a slow scrape cannot inflate the histogram's measured query interval. Tests cover
OTLP wire privacy/parenting, bounded labels/errors, unsampled metrics, concurrent
accounting and actual disposable database query/cancellation paths. The new tests
failed for undefined helpers before implementation, then passed.

No local Docker images, archive data or new NAS data/index copies were created.
All remote fixtures use invented data and invented tokens. Source changes are
committed on `feat/parade-edition-observability`, not merged or deployed.

## Homelab and rollout

Homelab `docker/ukinfo/collector.yaml` contains the matching prepared privacy and
histogram policy, retaining its existing regression scrape/HEC/TLS configuration.
`docker/ukinfo/edition-observability/README.md` records the bounded schema and
rollout status. This edits local configuration as code, not the running Collector.
Live credentials, endpoint settings, dashboards and other services were not changed.

Routine release uses homelab's reviewed-main hosted-release controller, which
rejects uncommitted deployment-scope changes or configuration not merged on main.
The draft application PR and prepared homelab branch therefore remain reviewable
inputs to a separate normal release; no experimental routing gate is bypassed.
Production stays on its existing image, with five public editions and v14 hidden.

Final read-only NAS verification confirmed the original serving image, healthy
public API, five editions / 248,951,107 occurrences, absence of the new edition
metrics and stopped private hybrid PG. Independent closeout review found no
remaining privacy, lifecycle, numerical or rollout-scope blocker.
