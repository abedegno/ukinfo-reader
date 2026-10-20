# OpenTelemetry and Splunk Observability Cloud

Hosted UK-Info can export sampled backend traces through standard OpenTelemetry
OTLP/HTTP. A private upstream Collector also scrapes the existing Prometheus
metrics and forwards both signals to Splunk Observability Cloud. Application code
contains no Splunk SDK or endpoint. Replace the Collector exporter to change backend.
Dashboards and detector query languages remain specific to the backend.

**Default:** no export. The desktop app (`--data-dir`) and maintenance commands do
not configure telemetry, even when telemetry environment variables are present.
Existing health checks, local monitoring and the private `/metrics` listener remain
independent of the Collector. This feature does not enable browser telemetry.

## Versioned configuration

| File | Purpose |
| --- | --- |
| `deploy/compose.observability.yaml` | Optional sidecar; pinned upstream image and digest, memory/CPU limits, no published ports |
| `deploy/observability/collector.yaml` | Receivers, privacy filters, bounded queues, batching and Splunk OTLP endpoints |
| `deploy/observability/splunk/main.tf` | Dashboard, charts and pool-saturation detector; pinned Terraform provider |
| `tools/check_observability.py` | Runs the real Collector with invented canaries and no network egress |

The Collector shares the API's network namespace to reach loopback metrics and
receive loopback traces. It has no database credentials, catalogue mounts, Docker
socket or host metrics receiver. It can access the same networks as the API; only
its configuration and ingest token are mounted. OTLP listens on `127.0.0.1:4318`.
The public HTTP service never proxies this port. Collector diagnostics remain
private; no debug exporter or telemetry payload logging is configured.

## Credentials and account checks

Obtain the realm from your Splunk account, such as `eu0` or `us1`. Confirm your
account includes the Infrastructure Monitoring/custom metrics and APM entitlements
you intend to use, along with retention and ingestion allowances. A free account's
limits are account-specific; this repository does not assume unlimited ingestion.

Prefer two separate credentials:

- **Collector:** an organisation token scoped to **Ingest**, stored outside Git in
  a file readable by container UID/GID `10001:10001`. For example, install it as
  owner `10001:10001`, mode `0400`, inside a private directory. Do not print it or
  place it in a Compose command line. The file must contain the exact raw token bytes with **no trailing newline**;
  the Collector's file provider preserves line endings, which make HTTP headers
  invalid. Write it with `printf '%s' "$SPLUNK_ACCESS_TOKEN"` redirected to the
  private token file, rather than a line-oriented writer.
- **Terraform:** an API-authentication token with permissions to manage dashboards
  and detectors. Supply it through `TF_VAR_api_token` in the local process environment
  or a CI secret. An explicitly authorised token with both scopes can serve both roles, but a
  dedicated ingest-only token reduces the Collector's privileges.

No credential values belong in `.env.example`, `.tfvars`, plans, documentation,
issues or CI logs. Environment files, Terraform state and plans are ignored by Git.
The provider lock file is committed. Protect and back up Terraform state separately;
for team use, configure an encrypted remote backend with locking.

## Production Compose setup

Use the production baseline, or translate the optional service into your own
configuration repository. Set these nonsecret values in your private Compose env file:

```dotenv
SPLUNK_REALM=eu0
UKINFO_SPLUNK_TOKEN_FILE=/absolute/private/path/splunk-ingest-token
UKINFO_OTEL_ENABLED=true
UKINFO_OTEL_SAMPLE_RATIO=0.1
```

The realm must be your actual Splunk realm, containing only its documented realm
identifier, such as `eu0`. It is interpolated into an HTTPS hostname. The token file
path must refer to the deployment host, not a different machine running Docker CLI.

Validate the merged configuration before changing the stack:

```sh
docker compose --env-file deploy/production/.env \
  -f deploy/compose.production.yaml -f deploy/compose.observability.yaml \
  --profile observability config --quiet
```

Then use the same arguments with `up -d`. If the deployment already uses tunnel
or monitor overlays, include those files in both commands. The API image must
contain this OpenTelemetry implementation. Recreating the API also requires
recreating sidecars sharing its network namespace. Keep validation ahead of any
Portainer update, as described in [production deployment](production.md).

`UKINFO_OTEL_ENABLED` must be exactly `true` to enable traces. The application uses
`OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` (an explicit full URL ending in `/v1/traces`)
and `UKINFO_OTEL_SAMPLE_RATIO` (0–1). Only HTTPS or explicit loopback HTTP is accepted.
Other generic SDK settings are not a supported configuration interface here. The
Collector overlay uses the local endpoint, so Splunk credentials stay out of Go.

To disable export, set `UKINFO_OTEL_ENABLED=false` and remove/stop the Collector.
Removing the overlay alone does not stop an already-running sidecar; remove it
explicitly using the deployment's Compose/Portainer lifecycle.

## Signals, privacy and cost controls

Request spans contain only a fixed route, bounded HTTP method, response status and
search category. Child operations are `db.pool.acquire`, `db.query`, `records.load`
and `address.select`. Database spans measure duration without SQL text, parameters,
connection details or error messages. Record loading includes decompression time;
there is no per-record span or identifier.

Names, addresses, postcodes, search terms, arbitrary paths, query strings, IPs,
user agents, source IDs, SQL parameters and original records are excluded. Errors
use fixed descriptions. Exception events and span links are disabled. Resource
attributes are allowlisted again at the final Go export boundary, because the
upstream SDK merges resource environment variables. Incoming public trace context
and baggage are ignored, so visitors cannot supply telemetry fields or force sampling.

The Collector keeps only `ukinfo_` metrics and approved dimension keys. It drops
scrape internals and resource metadata. Its trace attribute filter adds another
layer; names and descriptions are fixed by application instrumentation. Review any
new instrumentation before enabling it—this is not a general-purpose redaction
pipeline for arbitrary applications.

- Metrics scrape every 60 seconds and are **not sampled**. Counters remain accurate.
- Default trace sampling is 10% of requests; children follow the parent's decision.
  This also samples failures at 10%; it does not guarantee capture of every slow or
  failed request. Aggregate error metrics remain available for alerting.
- SDK queue: 512 spans, nonblocking; export timeout: 2 seconds, no SDK retries.
- Collector: 128 MiB container limit, 80 MiB memory limiter, 0.25 CPU, queue 32 batches
  and retries bounded to two minutes. Telemetry can be dropped under pressure or
  outage. No catalogue writes or search failures depend on successful export.
- No logs, profiling, host discovery or browser telemetry are uploaded.

Check Splunk's actual usage after enabling the service. Sampling bounds a fraction
of traffic, not a fixed bill or monthly quota. Lower the ratio or disable traces
if necessary. Do not increase label cardinality to track individual searches.

## Dashboards and alert definitions as code

With Terraform 1.11 or later, run from `deploy/observability/splunk`:

```sh
terraform init
terraform fmt -check
terraform validate
# Supply TF_VAR_realm and TF_VAR_api_token through your local environment.
terraform plan -out=private.tfplan
terraform apply private.tfplan
```

The dashboard includes request concurrency, pool utilisation, search rates by
category, error percentage, classified timeout/error rates, decoded block rates,
successful pool wait rate/mean, canceled acquisitions and a telemetry heartbeat.
The p95 chart uses the request histogram, aggregated by route, rather than taking
percentiles of counter values. Its seven finite bucket boundaries give an estimate;
use APM for individual sampled request/operation breakdowns. The Collector converts
cumulative Prometheus latency histograms to delta buckets; it drops the first
observation after startup and needs a second scrape before exporting that histogram.
Counters retain their original cumulative temporality.

`ukinfo_pool_wait_seconds_total` and `ukinfo_pool_waits_total` measure **successful**
acquisitions that encountered an empty pool. Their ratio gives the mean waited
acquisition time; canceled waits are counted separately and do not enter that mean.
`ukinfo_api_failures_total` uses bounded structured reasons (`query_timeout`,
`catalogue_integrity`, `unavailable`). Search, ordered/selective-address candidate and
source-read errors classify PostgreSQL cancellation/deadline errors from the actual
error type. Other legacy failure paths still count as `unavailable`; this metric
must not be interpreted as exhaustive timeout diagnosis for every endpoint. Quiet
series may be absent until their first event; a zero denominator produces a chart
gap rather than a fabricated zero-latency or zero-error observation.

Detectors warn about pool utilisation above 80% for five minutes and missing pool
heartbeat for five minutes. Missing telemetry is evaluated after the first observed
stream; it cannot detect a service that has never reported. It does not identify
whether the API, Collector, network or NAS failed. Notifications default to an empty list; add only approved destinations
through `notifications`. A dashboard and detector require the account's applicable
entitlements. Terraform validation does not prove that live ingestion or SignalFlow
queries work with your account; verify incoming names/dimensions and chart values
before treating the dashboard as operational.

After applying, maintain these resources through Terraform. Import existing
resources into state before adopting them to avoid duplicates. The provider emits
a deprecation warning about its automatic custom app URL with the pinned stable
release; this configuration does not set that deprecated field.

## Verification and rollout

```sh
cd server
go test -race ./internal/telemetry ./internal/catalog ./cmd/ukinfo-server
cd ..
python3 tools/check_observability.py
```

Go tests inspect actual OTLP protobuf exports using invented SQL, parameters,
errors and resource canaries. Request tests cover arbitrary paths and public
headers. The real Collector test verifies OTLP reception, Prometheus translation,
filtering and authenticated OTLP/HTTP export without external network access,
including loading the token through the production file-provider configuration. Required search CI runs both. A separate workflow validates Terraform against the
locked provider without account credentials; no CI job applies infrastructure.

For live rollout: enable the Collector, verify invented/local smoke telemetry in
the expected realm, confirm token/permission errors are absent, and compare counters
and chart dimensions with the private metrics endpoint. Confirm the sample ratio
and usage before raising traffic. Keep the current local monitor during rollout.

Splunk can detect missing telemetry once configured, but a separate external
uptime check remains useful for public DNS, TLS and tunnel failures. Local checks
cannot report a whole-NAS outage. Alert delivery needs a configured destination.

## References

- [OpenTelemetry Go exporters](https://opentelemetry.io/docs/languages/go/exporters/)
- [Splunk OTLP/HTTP endpoints](https://help.splunk.com/en/splunk-observability-cloud/manage-data/splunk-distribution-of-the-opentelemetry-collector/get-started-with-the-splunk-distribution-of-the-opentelemetry-collector/collector-components/exporters/otlphttp-exporter)
- [Splunk organisation token scopes](https://help.splunk.com/en/splunk-observability-cloud/administer/authentication-and-security/authentication-tokens/org-access-tokens)
- [Splunk Terraform provider](https://registry.terraform.io/providers/splunk-terraform/signalfx/latest)
