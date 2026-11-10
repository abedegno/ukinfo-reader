# Per-edition search observability

User approved privacy-safe per-edition timing/error signals after diagnostics
failed to reproduce broader timeouts. Base this feature on deployed ce6117f,
independent of unqualified street/SIC/layout changes. Production unchanged until
a separately qualified rollout; no ISO/data/image copies locally.

Use ordinary OpenTelemetry child spans plus always-recorded Prometheus histograms
through the existing Collector. Trace-only would miss head-unsampled failures;
private SQL auditing is unsuitable for ordinary observability. Retain existing
sampling and generic error status; no SQL/args/IDs/PIDs/request values/raw errors.

Span search.parade_edition attributes: ukinfo.backend=paradedb, ukinfo.edition
(one of six published archive keys or other), ukinfo.parade.stage (positions,
pair_drain,other), ukinfo.outcome (ok,deadline,canceled,query_canceled,error).
Unknown edition/stage maps to other; outcomes derive only from typed/wrapped
errors, context cancellation or SQLSTATE57014 (server cancellation, not proof
of a statement timeout). Metrics use the same bounded edition/stage/outcome.

Histogram ukinfo_parade_edition_duration_seconds uses edition/stage and existing
latency buckets, including failures without sampled traces. Counter
ukinfo_parade_edition_queries_total adds outcome. Max7*3*10 +7*3*5 =315
Prometheus series, created only on observation. Histogram includes all outcomes;
counters allow error-rate inspection without a backend-specific histogram count.
Collector sample cap remains2000; normal sparse cardinality is much smaller. No request values, arbitrary imported keys or unbounded SQLSTATE.
Time includes pool/query/row consumption and close, not only server execution.
Positions and pair draining both end on success/error/cancellation, once each.
Parallel edition durations overlap; never sum them as request wall time.

Update both project Collector variants and homelab privacy allowlists, normalize
new label values, add histogram cumulative-to-delta conversion and dashboard
charts. Prepare configuration as code; no token changes, notifications or live
mutation. Tests cover unsampled metrics, concurrent accounting, completion/error
paths, OTLP wire data and Collector stripping malformed/invented secrets. Existing
SQL/filter/order/cursor behavior remains byte-for-byte unchanged.
