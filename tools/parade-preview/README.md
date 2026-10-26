# Private serving-schema trial

This follows the full-edition [index spike](../real-search-spike/README.md), but
uses the actual optional API backend and source dictionary-ID ordering. Earlier
spike measurements are not acceptance evidence for this serving schema.

Build `server/cmd/ukinfo-parade-index`, `server/cmd/ukinfo-parade-trial`,
`server/cmd/ukinfo-server` and `tools/search-load` for the NAS architecture.
The deployed, private stack and orchestration live in
`~/homelab/docker/ukinfo/parade-preview/`; no public route is provisioned.

`ukinfo-parade-trial` provides bounded operator stages:

- `cases`: sample displayed original fields across all serving-visible editions;
  workload values are private and must remain on the NAS.
- `merge` / `subset`: retain previous regressions and select an early edition pilot;
  outputs are new mode-0600 files and existing evidence cannot be overwritten.
- `metadata`: export only case ordinal, edition ordinal, route/scope/kind and expected
  backend, without query text, names, postcodes, record IDs or hashes.
  Backend classification uses the same selector as serving.
- `verify`: exact per-edition coverage/index validity plus spread field samples
  against original compressed records and independent compact/address/research
  indexes. `--dataset KEY` is only a single-edition pilot; `all` is the final gate.
- `plans`: custom/generic prepared `ANALYZE, BUFFERS` diagnostics for at most twelve
  case ordinals. Only allowlisted nodes/counters leave the process. The scalar and
  postcode-prefilter strategies share the same rows/schema. Failed execution's
  unset work counters must not be interpreted as zero I/O.
- `cursors`: compare actual native/candidate continuations and exchange their tokens;
  verify changed-query binding without exposing tokens or record content.
- `calibrate`: save only mutually matching successful first pages as a private,
  reference-hashed load subset. Errors/exclusions remain in the full paired report.
- `snapshot` / `compare-snapshots`: capture a complete fixed first-page cohort or
  one case against a fixed private backend, then compare independently reset runs.
  Snapshots contain private response digests and stay on the NAS; separate reports
  contain only ordinal/status/latency/outcome fields. `--reverse` counterbalances
  corpus order without dropping failed cases.

HTTP calibration/cursor checks accept only the two internal trial hostnames,
ignore proxy environment and reject redirects. Never replay saved real queries to
an Internet URL or transfer workload/reference files into public artifacts.

Run complete HTTP comparisons (two rounds, up to ten pages) only after indexing
finishes. Use `search-load --fixed-rounds 2 --concurrency 1,4 --rate 0` independently
for each backend, then repeat in reverse backend order. Fixed shuffled cohorts
preserve identical case frequency instead of allowing a faster engine to cover
more of a duration window's corpus. Global request/time bounds still apply.

Record original-record retrieval, dataset visibility, fallback queries, errors,
first/later-page populations, resource limits, physical storage and cache state.
The candidate uses an additional 2-CPU/6-GiB database alongside the original store;
this is an end-to-end architecture trial, not an equal-total-resource engine test.
No OS/source cache is flushed. Pilot timings during maintenance are diagnostic,
not acceptance evidence. Keep the initial failed pilots and query formulations.

Before stopping the private database, capture aggregate metrics, work counters and
sizes. Restart only the private candidate with the native flag and the target DB
offline; verify reference-hashed responses. Stop remaining private services and
retain the derived volume explicitly. Source/rollback data, public containers,
Cloudflare controls, deadlines and quotas remain unchanged.

After orchestration and the final source snapshot complete, summarize the
allowlisted reports copied into the private local report directory:

```sh
python3 tools/parade-preview/summarize.py private/parade-preview-2026-10-26 \
  --output private/parade-preview-2026-10-26/summary.json
```

The summary checks complete edition coverage, fixed case frequencies, cursor
correctness, unchanged public health/source visibility and offline rollback.
Timeouts and other request errors remain in latency/error statistics. Load
measurements cover only the mutually successful calibration subset; the summary
reports its exclusions alongside the complete paired cohort. Review the aggregate
output and measurement limitations before using it in a published research report.
Experiment completion alone does not establish rollout readiness.

For controlled file-cache testing, `server/cmd/ukinfo-cache-probe` is a Linux-only
operator command. Stop the **private** databases and mount their PGDATA read-only
at its two fixed trial paths. It synchronizes/evicts individual owned files using
[Linux file advice](https://man7.org/linux/man-pages/man2/posix_fadvise.2.html) and
checks page residency using
[`mincore`](https://man7.org/linux/man-pages/man2/mincore.2.html). It rejects running
clusters, symlinks/hard links, foreign-owned files and RAM-backed filesystems. A
successful eviction report requires at most 1 MiB still resident. Never apply it
to the live source or use host-wide cache controls.

The follow-up NAS configuration is in `~/homelab/docker/ukinfo/hybrid-trial/`.
It makes a separate physical source copy using
[`pg_basebackup`](https://www.postgresql.org/docs/18/app-pgbasebackup.html) and
[`pg_verifybackup`](https://www.postgresql.org/docs/18/app-pgverifybackup.html),
preserving source image/collation/planner settings. Both native and hybrid read
that copy, leaving the original source's buffers and file pages outside the test.
Full cold-start batches warm as they progress; predeclared individual regressions
also reset independently before each request. Drive/controller caches and
filesystem metadata are outside the measured eviction boundary.

The equal-budget follow-up and hosted promotion are documented in
[the rollout report](../../docs/research/parade-rollout-2026-10-26.md). Homelab
keeps their procedures in `hybrid-control/` and `parade-serving/`. The control
uses a Go build overlay for a uniform four-second SQL backstop without changing
production HTTP limits. `ukinfo-native-plans` captures only structural/counter
fields for bounded private query diagnostics; `--estimate-only` avoids executing
the explained statement. It still sends one bounded original API request per
selected case to identify that request's actual SQL.

`ukinfo-parade-trial --stage hosted-check --cases PRIVATE_REFERENCES --output NEW_REPORT`
checks twelve saved references at a paced rate through the fixed internal
`http://ukinfo:8080` Docker service. It sets the production Host header, disables
proxies and redirects, and exports only match counters/status/latency. It never
replays saved searches through the Internet hostname. Old private controllers
refuse to stop the promoted database.
