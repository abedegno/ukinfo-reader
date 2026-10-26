# Ordered-search follow-up tools

These extend the [private serving trial](../parade-preview/README.md). They do not
download media, create a public endpoint or change serving configuration.

`build_reference.py` builds a Linux/amd64 operator executable in `/private/tmp`
using a Go overlay. Only this offline executable receives 120-second HTTP/ordered
SQL budgets, 256-MiB sort memory and JIT disabled. The normal server and its
deadlines are unchanged. The script rejects an unexpected source layout.

`ukinfo-parade-trial --stage native-reference` needs an existing private source
clone, its SELECT-only `SOURCE_DATABASE_URL`, private cases, and the candidate
service at the tool's fixed internal Docker hostname. Select up to four case
ordinals with `--plan-cases`; provide **different** `--output` and `--report`
paths. The output contains private hashes/cursors and must stay on the NAS. Only
the separate aggregate report is exportable.

The oracle supports grouped default/name and ungrouped edition address queries
without record refinements. It independently enumerates all source candidates
through an inclusive third-page ordering boundary and runs native finalization.
Candidate IDs do not restrict its population. Safety ceilings fail the run; they
never produce a successful truncated reference. Native fixture integration tests
compare this oracle with complete native results, including dense groups and tied
edition dates. `--stage compare-reference` checks two pages using both returned
and original-native cursors against that private reference.

`--stage hosted-reference` repeats those eight checks against the fixed internal
`http://ukinfo:8080` Docker service, using its public Host header, no proxy and no
redirects. Requests are paced 600 ms apart. This is post-deployment acceptance;
saved real queries do not traverse the public hostname or Internet.

Run real workload comparisons only on the internal network, using frozen input
cases and binaries. Keep the serving target SELECT-only, with no cache eviction
or restart. To measure source coldness, stop only the private source clone and
its clients, run the existing per-file Linux cache probe over that clone, require
its residency gate, then reopen it for exactly one request. Repeat independently
for both builds. Call this **source-cold** when the shared target cache is
uncontrolled. Never use a global cache flush or production source files.

Export a compact public artifact from the allowlisted aggregate files:

```sh
python3 tools/ordered-preview/summarize.py \
  --warm private/ordered-release-2026-10-26 \
  --cold private/ordered-cold-2026-10-26 \
  --output docs/research/ordered-search-2026-10-26-results.json
```

The summary requires the full 294-case/two-round comparison, four native oracle
pages/eight checks, cursor checks, both balanced 292-case load runs per build,
all seven source-cold pairs and unchanged public lifecycle. It retains error
counts: a performance improvement does not make the workload error-free.
The [report](../../docs/research/ordered-search-2026-10-26.md) explains limitations.
`experiments/native-v2.patch` preserves a rejected SQL approach for review only.
