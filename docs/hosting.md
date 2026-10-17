# Hosting options

Checked **17 October 2026**. These are a shortlist and a budget, not a capacity
certification or a provisioned deployment. The complete 2005 catalogue is **11.79 GiB**;
[local full-corpus measurements](research/compact-catalogue.md) tested the Go API and
PostgreSQL together under a 2-CPU/1-GiB container cap. A CPU quota on an M4 Pro does not
reproduce an inexpensive VPS's cores or disk performance. Benchmark the chosen host.

## Shortlist

| Option | Relevant allowance or base price | Fit |
|---|---|---|
| Oracle Always Free A1 | 2 OCPUs, 12 GB RAM, 200 GB combined boot/block storage | Potential zero-cost self-hosted Go/PostgreSQL instance |
| Hetzner CX23, EU | 2 vCPUs, 4 GB RAM, 40 GB NVMe; €5.49/month before VAT and IPv4 | Candidate for the complete 2005 edition |
| Hetzner CX33, EU | 4 vCPUs, 8 GB RAM, 80 GB NVMe; €8.49/month before VAT and IPv4 | Candidate for multiple editions, after measuring their combined footprint |
| Supabase Free | 500 MB database quota | Too small for the full catalogue |
| Cloudflare Pages | Free static requests and bandwidth | Frontend assets only; PostgreSQL and Go still need a host |

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

## Proposed first deployment

Use one VM with Go and PostgreSQL 18, plus a TLS reverse proxy. For a paid experiment,
start with **4 GB RAM** and enough disk for the measured catalogue, OS, WAL, temporary
index builds and recovery space. Treat the local 1 GiB benchmark cap as an experiment,
not a production RAM recommendation. Before choosing a multi-edition disk size, use
`ukinfo-server stats` on the actual combined catalogue. Leave at least 5 GiB available;
more space is needed for a local restore alongside the live copy.

Serve with the [SELECT-only role](search-app.md#run-without-docker), keep port 5432
private, preserve Host through the proxy and set `--public-origin`. Apply edge quotas
before opening public access. Retain bounded API admission and SQL deadlines. A second
API replica adds its own pool and CPU demand; it does not increase PostgreSQL capacity.

The acceptance gate on that VM is a longer run of varied searches and concurrent page
exports, with cold and warm observations, latency/error/drop counts, memory/disk/IO
monitoring and a dump/restore rehearsal. Use the [load tool](../tools/search-load/README.md)
with private validated fixtures. Account signup, paid provisioning, public publication
of directory records and final DNS configuration have not been performed.
