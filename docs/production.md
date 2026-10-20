# Production deployment and recovery

Use PostgreSQL 18 and the Go service with separate import and serving credentials.
The [production Compose file](../deploy/compose.production.yaml) supplies resource
limits, log rotation, a private database network and a loopback-only HTTP origin.
It is a deployment baseline; it does not provision DNS, TLS, remote backups or alerts.
See [hosting measurements](hosting.md) before sizing storage. Allow space for WAL,
a backup and a second database during a restore rehearsal, not just the catalogue.

## Configure and start

Run commands from the repository root. Use Docker Compose 2.24.4 or newer for the
optional tunnel overlay (`!reset` removes the origin's published port).

```sh
mkdir -p private/production-backups/monitor
chmod 700 private/production-backups private/production-backups/monitor
cp deploy/production/production.env.example private/production.env
chmod 600 private/production.env
openssl rand -hex 32
openssl rand -hex 32
```

Put the two independent passwords into `private/production.env`, set the actual
HTTPS origin and an **absolute** backup directory path. Hex passwords avoid URI
escaping mistakes in the DSNs. Set the monitor UID/GID to the numeric owner of its
state directory (`id -u`, `id -g`); the container must be able to write there.
Keep this file, backups, tunnel credentials and webhook credentials private.

For the commands below, define a shell function:

```sh
production() {
  docker compose --env-file private/production.env -p ukinfo-production \
    -f deploy/compose.production.yaml "$@"
}
production config --quiet
production build api
production up -d db
production run --rm admin migrate
```

The fresh PostgreSQL volume initializes `ukinfo_search` with SELECT privileges and
read-only transactions. Import using the admin profile, never the serving account:

```sh
python3 -m ukinfo_reader.search_export --help
production run --rm -T admin import < /private/path/verified-import.jsonl
production run --rm admin index-research --dataset YOUR_DATASET_KEY
production up -d api
```

Use the [import guide](search-app.md) to generate the verified JSONL stream and
optional address/research indexes. `admin` is an explicit Compose profile and does
not run as a background service. It holds the owner credential only for that job.
Check `production run --rm admin --help` for available maintenance commands.

Initialization SQL runs **only for a new data volume**. After restoring or migrating
an existing installation, explicitly apply the role policy as the owner:

```sh
production exec -T db psql -X -U ukinfo -d ukinfo \
  -f /docker-entrypoint-initdb.d/search-role.sql
```

The policy removes elevated role attributes and direct write/create grants from an
existing serving role. It refuses roles with object ownership or role memberships;
review those explicitly or use a dedicated serving role. Residual write access
through PUBLIC also causes the transaction to fail for explicit review. It does not silently
reassign objects or remove unrelated users' grants.

Changing the environment file alone does not rotate existing database passwords.
Rotate roles in PostgreSQL and recreate the affected clients together. Verify the
serving role cannot write even with `default_transaction_read_only=off`; that setting
is a second guard, not a replacement for privileges. Do not expose port 5432.

## HTTPS ingress

The baseline publishes HTTP on host loopback only. Put a TLS reverse proxy in front
and set its upstream Host to the configured public origin. The provided
[Nginx example](../deploy/nginx/ukinfo.conf) overwrites the client-IP header.
Set `UKINFO_TRUSTED_PROXY` to the exact socket peer IP/CIDR that the API actually
sees. Leave it empty for direct connections. Do not trust arbitrary forwarded
headers or broad networks. If a CDN precedes Nginx, configure Nginx's verified CDN
real-IP ranges before forwarding its resolved client address. Otherwise quotas
will be shared by the proxy's visitors or trust attacker-supplied addresses.

For a dedicated Cloudflare Tunnel, copy the
[example](../deploy/production/cloudflared-config.example.yml) into a private
0700 directory with its scoped tunnel JSON, both files 0600 and owned by the
configured UID/GID. Set `UKINFO_TUNNEL_DIR` to that absolute path. Use a subnet that
does not overlap your host or Docker networks; peer/API addresses must match it.
Then add the overlay to **every** Compose invocation:

```sh
docker compose --env-file private/production.env -p ukinfo-production \
  -f deploy/compose.production.yaml -f deploy/compose.tunnel.yaml up -d
```

The overlay removes the published HTTP port, fixes the trusted connector peer and
retains the private database network. Configure the tunnel's hostname/DNS on
Cloudflare. Edge WAF rules complement application quotas; they do not replace them.
Metrics listen on API loopback port 9091 and must not be routed through the tunnel.

## Backups and isolated restore rehearsals

```sh
production exec -T db sh /checks/backup.sh
```

The script refuses catalogues containing demo/sample datasets. It creates a private
custom-format archive, verifies its table of contents and writes a SHA-256 sidecar.
It never prunes older backups. Run it after catalogue/index changes and schedule it
on the host if regular refreshes are expected. Copy verified backups to a separate
failure domain; a second directory on the same NAS is not an off-device backup.
Protect backups as carefully as the live database.

A checksum and table-of-contents check do not establish recoverability. Rehearse
on a **separate PostgreSQL instance and data directory**, keeping the serving
instance intact. A second database in the serving instance shares its buffers, CPU
and autovacuum workers; it can disrupt live searches even with no table locks.
Cap rehearsal resources and give its disk I/O lower priority where supported.
Create a new, explicitly named destination database in that separate instance:

1. Check free space for a complete second catalogue plus restore/WAL headroom.
2. Verify the archive with `sha256sum -c FILE.dump.sha256` inside the DB container.
3. Create a dedicated database using `template0` and the source database's locale.
   Revoke PUBLIC access before restoring.
4. Restore with `pg_restore --exit-on-error --no-owner --no-acl --jobs=1` as the owner.
5. Compare dataset keys, manifests, expected and actual entry counts. Check all
   indexes are valid, inspect block checksums and run known search fixtures through
   an isolated service using the restored database. Run ANALYZE before timing it.
6. Record archive hash, elapsed time and aggregate verification results. Only after
   passing those checks, remove the explicitly named rehearsal database. Retain the
   backup and serving database.

Never restore over the live catalogue as a test. PostgreSQL physical backups require
matching major version/platform; custom-format logical dumps are the portable path.

## Monitoring and alert delivery

Add `-f deploy/compose.monitor.yaml` to the same Compose invocation (after the other
overlays). The monitor shares the API network namespace and reads only a private
state directory. It has no database credential, record mount or Docker socket.
Place its state directory on the same filesystem as the database/backup staging for
its free-space check to be meaningful. On separate filesystems, monitor each one
with your host monitoring system as well.

Every minute it checks health, private metrics, free disk space and backup freshness.
It detects a timeout surge when one interval contains at least 20 searches/exports,
at least five HTTP 503 responses and a failure fraction of at least 10%. This is a
coarse overload signal: 503 includes capacity failures as well as timeouts. Configure
`UKINFO_MIN_FREE_GIB` (default 20) for your actual restore headroom and
`UKINFO_BACKUP_MAX_DAYS` (default 30) for your update schedule.

Read `monitor/status.json` for the aggregate state. Docker health reports whether
the monitor loop is alive; the JSON reports service conditions. With no webhook,
checks still run and `delivery` is `disabled`. Set `UKINFO_ALERT_FILE` to a private
0600 HTTPS webhook URL file readable by the monitor UID. The receiver must accept
JSON POSTs; this is a generic payload, not a Slack/Discord-specific message format.
Two consecutive bad samples trigger a notification, repeated after 30 minutes;
a successful recovery triggers one more. Failed deliveries retry on the next sample.
Payloads contain only aggregate status, never queries, names, IPs or credentials.

A real delivery test requires an authorized destination. A configured URL alone is
not proof that anyone receives alerts. Test a controlled failure and recovery and
check the receiver. Run an **external** uptime check too: a monitor on this NAS
cannot report total NAS/network failure while it is offline.

## Repository safeguards

[Branch protection payload](../.github/branch-protection.json) documents the required
always-run Python matrix and search checks. It requires a PR and resolved review
conversations, blocks force pushes/deletion and applies to admins. A second maintainer
can increase the required approval count; the initial sole-maintainer policy uses zero.
Apply it with `gh api --method PUT repos/OWNER/REPO/branches/main/protection --input
.github/branch-protection.json` and verify the server response. The file alone does
not activate protection. Dependabot configuration likewise does not establish that
repository vulnerability alerts/security update settings are enabled.

## Optional OpenTelemetry backend

[OpenTelemetry and Splunk setup](observability.md) adds a private, resource-limited
Collector and versioned dashboards. Existing health checks and `/metrics` remain
available. Export is disabled by default and desktop telemetry stays disabled.
