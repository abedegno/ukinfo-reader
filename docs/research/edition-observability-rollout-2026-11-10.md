# Edition observability rollout — 10 November 2026

The feature is merged in [reader PR #51](https://github.com/abedegno/ukinfo-reader/pull/51)
at `7ac36db`. It adds bounded edition/stage/outcome timings and errors while
preserving SQL, pools, deadlines, sampling and public edition visibility.

Fresh qualification of `hosted-2026-11-10.1` passed in 797 s: 1,407 original and 99
expanded page pairs, 64 native references, six balanced load phases and ten
source-cold probes. Candidate errors/mismatches were zero. C1 p95 was 121.7 ms,
maximum C4 p95 was 200.3 ms and maximum cold latency was 2.547 s against 3 s.
All 28 baseline errors and two bootstrap failures are retained. The separate
broader historical performance gate remains failed.

Focused homelab [controls PR #2](https://github.com/abedegno/homelab/pull/2) and
[promotion PR #3](https://github.com/abedegno/homelab/pull/3) are merged. Exact-main
app and policy CI passed. The normal controller attempted promotion, but native
acceptance matched 11/12 checks: an exact person-name/town search in one edition
returned 503/`query_timeout` after 2.33 s. The controller restored the previous
stack before ParadeDB acceptance. The same frozen search had succeeded in both
private baseline and candidate rounds. Both comparison processes used ParadeDB
mode; those successes do not validate the live native-only path or establish its
timeout cause.
No search replay, retry-until-green, changed fixture or relaxed deadline was used.

Actual running release remains `hosted-2026-10-27.3`. Exact previous images were
restored, serving database lifecycles stayed unchanged, all services are ready,
and the lease was released. The public catalogue is still five editions /
248,951,107 occurrences, with v14 hidden. The desired manifest is not deployment
proof. Collector policy and two Terraform charts are applied, with an existing
state and retained NAS config backup. Existing metrics, sanitized logs and sampled
traces were verified in Splunk. New edition telemetry is not verified live, and
the failed request had no downloadable APM trace. Source queries, case IDs,
response hashes, SQL and trace IDs remain private.

Next: investigate and qualify the native live-data timeout, then repeat the same
guarded release. Numeric evidence and actual state are maintained in
[homelab CURRENT](https://github.com/abedegno/homelab/blob/main/docker/ukinfo/CURRENT.md)
and its linked observability artifacts. No local Docker image or new database
copy was needed.
