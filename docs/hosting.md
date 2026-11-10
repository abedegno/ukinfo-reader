# Hosting and operations

## Current deployment and capacity

The [public archive](https://ukinfo.6153112.xyz) runs the Go service and PostgreSQL 18
in Docker on a NAS, through a dedicated Cloudflare Tunnel. It serves five complete
editions (248,951,107 source occurrences) with no sample/demo datasets. The database has no public port; the API uses
a SELECT-only role, explicit tunnel-peer trust, per-client quotas and bounded work.
See [public service controls](#public-service-through-cloudflare-tunnel).

The original 17 October 2026 hosting shortlist below is a dated planning record.
Prices, provider allowances and availability must be checked before provisioning.
The complete 2005 catalogue measured **11.79 GiB**; the initial four-edition base together
measured **38.26 GiB** (189,064,266 source occurrences).
The optional [ONSPD nearby-search reference](geography.md) adds about **840 MiB**
for postcode coordinates and indexes, plus import/WAL headroom.
The later [research-index measurements](research/review-validation.md) add **45.84 GiB**
for complete grouping/archival lookup/business classification indexes. The combined
local database, including the existing address/coordinate indexes and small development
samples, measures **86.40 GiB**. These are initial four-edition measurements, before the 2003 import and later hosted
projections; they are not the current five-edition footprint. The 38.26 GiB figure is that base catalogue. An 80 GB
disk cannot hold the measured fully indexed installation; OS, WAL, temporary builds,
growth and backups need additional space.
The [local full-corpus measurements](research/compact-catalogue.md) tested the Go API and
PostgreSQL together under a 2-CPU/1-GiB container cap. A CPU quota on an M4 Pro does not
reproduce an inexpensive VPS's cores or disk performance. Benchmark the chosen host.

## Shortlist

| Option                | Relevant allowance or base price                               | Fit                                                                    |
| --------------------- | -------------------------------------------------------------- | ---------------------------------------------------------------------- |
| Oracle Always Free A1 | 2 OCPUs, 12 GB RAM, 200 GB combined boot/block storage         | Potential zero-cost self-hosted Go/PostgreSQL instance                 |
| Hetzner CX23, EU      | 2 vCPUs, 4 GB RAM, 40 GB NVMe; €5.49/month before VAT and IPv4 | Candidate for the 2005 base catalogue; measure optional-index headroom |
| Hetzner CX33, EU      | 4 vCPUs, 8 GB RAM, 80 GB NVMe; €8.49/month before VAT and IPv4 | Base catalogue only; extra storage needed for full research indexes    |
| Supabase Free         | 500 MB database quota                                          | Too small for the full catalogue                                       |
| Cloudflare Pages      | Free static requests and bandwidth                             | Frontend assets only; PostgreSQL and Go still need a host              |

Oracle's current allowance is **2 OCPUs / 12 GB**, rather than the older 4 / 24 figures
still repeated in tutorials. Free resources must be in the home region. A1 capacity
can be unavailable, and Oracle can reclaim idle instances under its seven-day CPU,
network and memory criteria. This is worth trying for a personal archive or demo that
can tolerate reprovisioning; keep a recoverable copy outside the VM. See the
[official allowance and reclamation rules](https://docs.oracle.com/en-us/iaas/Content/FreeTier/freetier_topic-Always_Free_Resources.htm).

Hetzner specifications come from its [cost-optimized plans](https://www.hetzner.com/cloud/cost-optimized/);
base prices come from the [15 June 2026 price adjustment](https://docs.hetzner.com/general/infrastructure-and-availability/price-adjustment/).
Stock is limited and region-dependent; confirm console availability and a current
quote. [Server backups cost 20% of the server price](https://docs.hetzner.com/cloud/billing/faq/):
approximately €1.10/month for CX23 or €1.70 for CX33, in addition to the base price.
IPv4, tax, domain registration and any separate backup/object storage are extra.
Managed disk snapshots do not replace a tested PostgreSQL recovery procedure.

[Supabase's free database quota](https://supabase.com/docs/guides/platform/database-size)
is much smaller than the measured complete edition. Free static hosting such as
[Cloudflare Pages](https://www.cloudflare.com/developer-platform/products/pages/)
can serve assets, but it does not supply this application's database/runtime.
The current Go service already embeds the small frontend, so a separate static host
is optional. Keeping UI and API on one origin also preserves the existing Host/Origin
checks without introducing a cross-origin API configuration.

## Response size and bandwidth

A separate pass over the same 1,550 validated full-2005 cases measured mean response
bodies of **26,832 bytes** uncompressed. Offline gzip level 1 produced a mean of
**3,150 bytes** (88.3% fewer bytes); p95 sizes were 43,527 and 5,985 bytes respectively.
This is an independent compression-size experiment, not a deployed proxy benchmark.
[Raw aggregate measurements](research/full-catalogue-results.json) contain no records.

For this uniform case mix, one average request/second over 30 days would produce
approximately **69.6 GB** of uncompressed response bodies or **8.2 GB** at the measured
gzip size. HTTP/TLS overhead is additional. Actual user queries, page sizes and traffic
patterns can change these estimates substantially. The current Go endpoint does not
compress HTTP responses itself. Enable JSON/NDJSON compression on the chosen TLS proxy,
then measure real transfer and CPU cost; see the [nginx gzip module](https://nginx.org/en/docs/http/ngx_http_gzip_module.html).

## Planning another deployment

Use one VM with Go and PostgreSQL 18, plus a TLS reverse proxy. For a paid experiment,
start with **4 GB RAM** and enough disk for the measured catalogue, OS, WAL, temporary
index builds and recovery space. Treat the local 1 GiB benchmark cap as an experiment,
not a production RAM recommendation. The 40 GB CX23 disk is smaller than the measured
four-edition base database even before OS space. The 80 GB CX33 can accommodate the
base catalogue, but **cannot hold the current 86.40 GiB fully indexed database**.
For all optional features, budget a larger disk or attached volume; 150–200 GiB is a
planning allowance to verify against actual WAL/build/retention needs, not a measured
minimum. Keep recoverable backups on separate storage. Additional volumes change the
quoted base budget. Before ordering, use
`ukinfo-server stats` on the actual combined catalogue. Leave at least 5 GiB available;
more space is needed for a local restore alongside the live copy.

Serve with the [SELECT-only role](search-app.md#run-without-docker), keep port 5432
private, preserve Host through the proxy and set `--public-origin`. Apply edge quotas
before opening public access. Retain bounded API admission and SQL deadlines. A second
API replica adds its own pool and CPU demand; it does not increase PostgreSQL capacity.

The acceptance gate on that VM is a longer run of varied searches and concurrent page
exports, with cold and warm observations, latency/error/drop counts, memory/disk/IO
monitoring, transferred-byte/egress estimates and a dump/restore rehearsal.
Use the [load tool](../tools/search-load/README.md)
with private validated fixtures. These VM measurements/provisioning gates are distinct from the current NAS deployment.
Cold HDD group/record reads on the NAS can exceed deadlines; a new host still needs
its own acceptance checks. A production backup/restore rehearsal remains outstanding.

## Public service through Cloudflare Tunnel

For an anonymous hosted service, `serve --public-limits` enables in-process quotas:

- Two API requests/second per IPv4 address or IPv6 /64, burst ten.
- Six page exports/minute per client, burst two; exports also consume API quota.
- Eight API requests in flight globally, with at most two expensive searches,
  exports, record/group/address/map requests per client. Startup metadata does not
  share that smaller per-client concurrency cap.
- At most 8,192 client buckets; expired buckets are reclaimed under pressure.
  If the table is full of active buckets, new clients receive HTTP 429.
- HTTP 429 with `Retry-After` on quota/capacity rejection; GET/HEAD only, query strings
  capped at 8 KiB. Local loopback health checks retain independent admission.

These limits are per process and reset on restart. Shared NATs share a quota;
replicas multiply global capacity. Limits reduce abuse; they cannot prevent copying
public search results or guarantee availability against distributed application attacks.

With Cloudflare Tunnel, pin the connector's Docker address and pass only that peer,
for example `--trusted-proxy 192.0.2.10/32`. The address is an illustration: use your
actual private connector address. Only those socket peers may supply a single valid
`CF-Connecting-IP`; malformed/missing headers are rejected. Untrusted forwarded headers
and `X-Forwarded-For` never choose a bucket. Keep the Docker network dedicated to this
API and its connector, and do not publish the API to the LAN/Internet. Without a
trusted proxy, the socket address is used. `--trusted-proxy` requires `--public-limits`.

Cloudflare Free provides managed WAF/DDoS protection and limited custom/rate rules.
Check the zone's **existing** quota use and expression-field limits before assigning
its single rate-limit slot. A rule protecting another service must not be overwritten.
A separate NAS proxy is optional when application admission and client quotas are
already enforced. Keep SQL timeouts, the SELECT-only role and container resource caps.

HTTPS deployments also send HSTS for this hostname (no includeSubDomains/preload),
frame protection and a restrictive Permissions-Policy, alongside CSP, nosniff and
no-referrer. Private metrics add `ukinfo_public_denied_total`, `ukinfo_public_active`
and `ukinfo_public_client_buckets`; no IPs, queries or identities are labels/logged.
Do not expose the metrics or desktop control listeners through the tunnel.

References: [Cloudflare rate-limit availability](https://developers.cloudflare.com/waf/rate-limiting-rules/),
[Cloudflare client-IP headers](https://developers.cloudflare.com/fundamentals/reference/http-headers/).

## Edge quotas and private metrics

A reviewable nginx example is in [deploy/nginx/ukinfo.conf](../deploy/nginx/ukinfo.conf).
Replace the example hostname/certificate paths, run `nginx -t`, and start Go with the
same `--public-origin https://…`. It limits each socket client to 30 API requests/minute
with a burst of 10, exports to 6/minute with a burst of 2, and four concurrent
connections. These are starting settings to tune with observed traffic, not measured
capacity promises. Rejections use HTTP 429. Clients sharing a NAT share a quota.
The example assumes nginx is the internet edge: if placing a CDN in front, configure
its trusted proxy CIDRs and real-client-IP handling before using these zones. Never
trust arbitrary forwarded headers. See the [nginx rate-limit module](https://nginx.org/en/docs/http/ngx_http_limit_req_module.html).

Run the service using the SELECT-only role described in the setup guide:

```sh
# DATABASE_URL is supplied privately by the service manager, not committed in a unit.
ukinfo-server serve --listen 127.0.0.1:8080 \
  --public-origin https://archive.example.org --metrics-listen 127.0.0.1:9091
```

The search pool has at most eight connections; health has a separate pool with at most
one connection and a one-second deadline. `/api/health` bypasses the 32-request search
admission queue. A genuine database failure still returns 503. A health success does
not prove that a representative search or catalogue verification succeeds.

`http://127.0.0.1:9091/metrics` exposes Prometheus text counters/histograms by fixed route
and status, active requests and search-pool occupancy/cancelled acquisitions. The
listener rejects non-loopback bind addresses and is absent unless requested. Do not
publish it through the proxy. There are no query strings, source IDs, names, IPs or
unbounded request paths in metric labels. Monitor latency histograms, 503/error rates,
429s at the edge, sustained pool saturation, process RSS, disk/WAL and PostgreSQL I/O.
Metric counters reset on restart; retain them in an operator-managed monitor if needed.

The example's access log omits requests, IPs and referrers. nginx error logs can still
include the failed request: keep them private, apply rotation/retention and redact
before sharing. Disable request logging if it is not needed. Do not enable SQL
parameter/debug logging on this dataset. Metrics and private query history serve
different purposes; the server does not collect the browser's research history.

Before launch, run the load tool against the intended host with exports and expensive
address/radius searches, verify quotas with more than one client, and rehearse restoring
a PostgreSQL backup into a separate database. Retain the original until verification
and representative searches pass. The local desktop cold-copy format is not a cross-OS
or cross-major-version server backup. For a live hosted database use PostgreSQL's
[supported backup methods](https://www.postgresql.org/docs/18/backup.html).

The edge recipe is exercised by `python3 tools/check_hosting_edge.py`. It starts a
network-isolated nginx with a temporary verified self-signed certificate and an invented
static upstream. A burst yields 11 accepted / 9 HTTP-429 search requests and 3 accepted /
3 HTTP-429 exports. It tests TLS verification and the configured quota semantics; it
does not test a public certificate, DNS, a production VM or database throughput.
