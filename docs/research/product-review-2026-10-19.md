# UK-Info Archive: critical review and proposed improvements

**For the morning of 19 October 2026.** Prepared on 18 October, Europe/London.
Reviewed application commit: **`7d5a8bf1f2f47d152beb7c09662bba33d863a34b`**.

**Review and recommendations only. No recommendations in this report have been
implemented.** The previously authorised search-form redesign was completed, tested
and saved before this review began. The local preview remains at
<http://127.0.0.1:58318>. Releases remain unpublished drafts.

> Implementation follow-up: [recommendation status and evidence](../review-implementation.md).
> This report records the review boundary; its original feature-gap descriptions remain historical.

## Executive assessment

The project has a credible preservation and search foundation. Four precisely identified
sources have complete-stream preservation checks; the newer sources have working address
joins and 2005 business support. The Go/PostgreSQL catalogue is compact enough to make a
small persistent host plausible, and the desktop app imports original media without
running historical Windows installers. Those are substantial, useful capabilities.

The product is ready for a **controlled desktop beta after installation/recovery checks**.
An unrestricted public lookup service still needs separate operational and publication
work. Current local benchmarks and signed builds do not settle those questions.

My recommended order is:

1. **Protect users' work:** restore/upgrade rehearsal, clear backups, recovery after a
   crash, and a supported way to keep installed components current.
2. **Validate usability with researchers:** the new form is simpler, but long mobile
   forms, small text, result navigation and hidden matching rules still need observation.
3. **Make repeated research easier:** saved searches, a research log, explicit edition
   coverage, and clearer comparison/citations.
4. **Complete the promised search semantics:** whole-search listing grouping, suitable
   ordering and measured place suggestions. Each needs backend correctness/cost work.
5. **Establish public-service readiness:** rights/privacy decisions, rate limits,
   monitoring, recovery, and a benchmark on the actual host.

Keep the current Python decoder, Go service, PostgreSQL catalogue and shared UI. The
available evidence supports improving this architecture; it does not justify another
engine/language/framework rewrite. The largest remaining gaps are product assurance,
research workflow and operations.

## 1. What was reviewed, and what was not

### Evidence used

- Repository code, desktop/release workflows, format reports, existing full-catalogue
  measurements, source-reference comparisons, and current user documentation.
- The latest local **30-test browser suite**, four focused final responsive/mode checks,
  and the packaged desktop onboarding/import/search/export/research/diagnostics test.
- Read-only walkthroughs using **invented demo records**. No original directory records
  were copied into this report or its screenshots.
- Public forms/product documentation for 192.com, Yell, FreeCEN, Findmypast, Gramps Web,
  Zotero and OpenRefine; a FamilySearch-hosted search-strategy handout by Dana Palmer;
  GOV.UK, W3C, Electron, PostgreSQL and ICO guidance. Paid results and account-only
  workflows were not exercised. The specialist handout is not official product documentation.

The review did not run a new full import, restore a 38 GiB catalogue, provision a VPS,
perform a penetration test, certify accessibility, execute malware, open public access,
or publish a release. Suggestions about user behaviour remain hypotheses until tested
with people. Performance figures below are existing measurements, not new overnight
load-test results.

### Verification status

The reviewed commit's [Tests](https://github.com/abedegno/ukinfo-reader/actions/runs/39170638048),
[Search application](https://github.com/abedegno/ukinfo-reader/actions/runs/39170638052)
and [Desktop packages](https://github.com/abedegno/ukinfo-reader/actions/runs/39170638132)
CI runs all completed successfully, including ARM Mac, Intel Mac and Linux native packages.
Those are build/regression results, not completion of the customer-install or hosting gates.

### State at the review boundary

| Area | Implemented / established | Remaining qualification |
|---|---|---|
| Four supported editions | 1998 SE, 1999 SE 4.0, 2004 SE residential, 2005 V10.5 SE residential/business | Exact source profiles only; other Standard/Pro/SE media require validation |
| Source preservation | 189,064,266 complete-source occurrences; compact database previously measured at 38.26 GiB | Occurrences are not distinct people; preservation is not historical factual accuracy |
| Search | Names/prefixes, partial postcodes, address components, premises/given-name refinements, paging, all editions | No fuzzy/contains search, generic place parser, user-selected sort or whole-search grouping |
| Research | Saved records/addresses, notes, citations, four-entry comparison, JSON/CSV transfer | No saved query library, research task/log model or cloud sync; storage limits apply |
| Maps | Approximate postcode points, page/saved-entry map, nearby centres, opt-in background tiles | Not exact buildings, all-results maps, district-boundary search or nearest-first ordering |
| Desktop | First-launch import, ISO/7z preparation, samples/full editions, catalogue management, interrupted setup restart, diagnostics | Native macOS ARM/Intel and Linux x64; no native Windows local package |
| Releases | 0.3.3 Mac draft candidates passed real signing/notarization/stapling/Gatekeeper gates | New diagnostics/form code is 0.3.4 development code; no signed 0.3.4 draft yet |
| Hosted web | Local read-only preview; documented deployment approach | No public hosted deployment or production-capacity result |

Sources: [editions](../editions.md), [compact catalogue](compact-catalogue.md),
[research lists](../research-lists.md), [maps](../maps.md),
[desktop releases](../desktop-releases.md), [signing evidence](../macos-signing.md).

## 2. What is working well

### Preservation has evidence behind it

The project checks complete output streams and normalized search projections, retains
source warnings and unresolved references, and distinguishes supported editions from
similar-looking media. The modern place join comparison exercised **27,958 rows in
each of two editions**, with no bounded-text differences from the original functions.
Telephone-format and business-format comparisons provide additional checks with their
limitations recorded. This is much stronger than testing a few successful names.

However, the full-catalogue verifier compares storage to the Python producer. It proves
that ingestion preserved the producer's output; it is not an independent proof that every
inferred Python decoding rule is correct. Original-function comparisons cover specified
components, not all conceivable records/semantics. Retain this distinction in releases.
See [locality](../validation/modern-locality-reference-comparison.json),
[telephone](../validation/modern-telephone-reference-comparison.json),
[business](../validation/modern-business-reference-comparison.json), and
[source anomalies](../formats/modern-se.md#source-anomalies-and-validation).

### Architecture choices match the work

One Go/PostgreSQL API supports hosted and desktop operation. Compressed source blocks
retain complete records while small projections serve search. The existing two-million
record pilot measured a **79.1% storage reduction**, with a documented throughput tradeoff.
The full-source and mixed-source tests are useful evidence, with cache/CPU limits disclosed.
Keep that discipline when adding grouping, fuzzy matching or ordering.
[Measurements and limits](compact-catalogue.md).

### Local execution boundaries are sensible

The app uses an isolated renderer, disabled renderer Node integration, explicit IPC
sender/main-frame/origin checks, a separate capability-protected control listener, and
loopback PostgreSQL with private credentials. Media readers prepare required files and
never run the original installer. The server checks Host/Origin and emits a CSP. Source
files, source exports and clusters are excluded by repository checks. These are useful
controls; this review is not a security certification.
[Main process](../../desktop/main.cjs), [control IPC](../../desktop/import-ui.cjs),
[server](../../server/cmd/ukinfo-server/main.go),
[public-tree check](../../tools/check_public_tree.py).

### Research data is treated as user-owned work

Native research writes validate schema/revisions, use temporary-file replacement, and
preserve malformed files. Browser writes use Web Locks; conflicting saves fail visibly.
Research snapshots survive an edition's removal, and saved references reject changed
source identity. Diagnostics is built from allowlisted fields and excludes raw logs,
queries, credentials, paths and research notes. Keep those properties in future work.
[Research persistence](../../desktop/research-store.cjs),
[diagnostics](../diagnostics.md).

## 3. Critical findings and gaps

Priority here means **sequence**, not a claim of a discovered security vulnerability.
“Before public hosting” and “before a general desktop release” are different gates.

### R1. Recovery and backup need to become a normal user journey

**Before a general desktop release.** Research JSON export and a documented cold-copy
procedure exist. There is no integrated catalogue backup/restore wizard or proven
cross-version/cross-architecture migration journey. Physical PostgreSQL directories
are not a general portable interchange format. A user may mistake a copied application
bundle or diagnostics export for a complete backup.

The supervisor intentionally refuses a cluster containing `postmaster.pid` and asks the
user to check for an orphan. That protects a live cluster, but a power failure can leave
a nontechnical user needing manual investigation. Do not solve this by blindly deleting
PID files. Proposed work: inspect ownership/process/version state, explain what is safe,
preserve files, and offer a tested recovery/restore path. Logs also append indefinitely
without a rotation policy in the inspected code.

**Acceptance:** restore notes and a catalogue to a separate clean profile; survive an
interrupted import and an unclean app shutdown; preserve a genuinely live cluster;
show backup type/size/compatibility; verify disk use remains bounded. A full-catalogue
restore must be rehearsed when capacity allows, separately from tiny CI fixtures.
Evidence: [supervisor](../../server/internal/desktop/postgres.go),
[backup guidance](../catalogue-management.md#back-up-or-move),
[log creation](../../desktop/main.cjs). PostgreSQL documents distinct SQL-dump,
filesystem and continuous-archive approaches in its
[backup guide](https://www.postgresql.org/docs/18/backup.html).

### R2. The newest features have not completed the signed customer-install journey

**Before a general desktop release.** The 0.3.3 signed draft predates diagnostics and the
new search form. Main-branch packages passing CI do not turn them into signed release
assets. Signed 0.3.4 candidates, browser download/quarantine/open tests, installation
into Applications, real-disc imports and an upgrade preserving existing research remain
release work. A clean CI profile is not a clean customer's Mac.

**Acceptance:** ARM/Intel notarized candidates for one immutable source commit, all
asset digests checked, clean-download launch, upgrade/rollback rehearsal, supported OS
baselines documented. Keep release drafts unpublished until approved.
[Release gates](../desktop-releases.md), [signing workflow](../../.github/workflows/macos-signing.yml).

### R3. Runtime maintenance needs a shipped-component policy

**Before regular public releases.** Electron is declared in `devDependencies` because
it is a packaging tool, but its binary is shipped. The workflows run `npm audit --omit=dev`.
That check alone does not assess the packaged Electron/Chromium/Node runtime, PostgreSQL,
Python or native media libraries. Zero findings from it are not a clean bill of health
for the app. No specific vulnerable component was established in this review.

Proposed work: inventory actual shipped versions/licences (build a useful SBOM), monitor
advisories across runtimes, document update ownership/cadence, and offer a deliberate
update notification. Stabilize backup/upgrade behavior before adding unattended updates.
Pip's top-level pins also do not constitute a complete locked, reproducible environment.
Keep source hashes/notices, and record the resolved Python build environment.

**Acceptance:** every shipped executable/library has a version/source owner; a simulated
urgent patch can be built, signed, tested and communicated without modifying user data.
Evidence: [package metadata](../../desktop/package.json),
[desktop CI](../../.github/workflows/desktop.yml),
[staging dependencies](../../pyproject.toml). Electron's
[security guidance](https://www.electronjs.org/docs/latest/tutorial/security/)
explicitly includes keeping the framework current and validating IPC senders.

### R4. Public hosting has operational and publication gates still open

**Before unrestricted public access.** Global admission limits and SQL deadlines are
present; per-client quotas, a deployed edge policy, service metrics/alerts and a tested
remote recovery procedure are not established here. The API's health endpoint also
shares the global admission pool: overload can look like failed health to an external
monitor. Multiple API replicas multiply pools and decompression work; they do not make
the database capacity problem disappear.

The historical discs mix electoral, telephone and business sources. Internet Archive
availability and the code's MIT licence do not establish a blanket right to republish
all underlying records. The ICO distinguishes full and open electoral registers and
currently flags its guidance as under review. Those current rules do not, on their own,
classify these particular historical discs. Establish source rights, intended audience,
privacy handling and an appropriate correction/contact process for the proposed service
before opening it. This is a decision to investigate, not a conclusion that all local
research is prohibited. [ICO electoral-register guidance](https://ico.org.uk/for-the-public/your-data-and-elections/electoral-register/).

**Acceptance:** selected host measured under sustained mixed traffic, safe DB role and
private DB port, working TLS, explicit rate limits, privacy-conscious logs/retention,
monitoring, cost/disk limits, and a restore rehearsal. A public demo using invented data
can be assessed separately from publishing a real directory service.
Evidence: [HTTP admission](../../server/internal/catalog/http.go),
[hosting plan](../hosting.md), [benchmark limits](compact-catalogue.md).

### R5. Whole-search grouping is still missing

**High product priority.** “Group identical listings on this page” is accurately labelled,
but the same matching listing can reappear on another page. All-editions search therefore
does not yet fulfil the broader expectation of search-wide deduplication. Grouping also
preserves distinct listing types; it is not a count of people.

Proposed work: a separate grouped-result mode with an explicit normalization/key contract,
all underlying occurrences accessible, source-warning exclusions, and stable group-aware
pagination. Keep an occurrence view. Avoid auto-merging people just because names and
addresses coincide. Client-side grouping of each next page cannot solve this.

**Acceptance:** a duplicate split across page and edition boundaries appears once in
group mode; all source occurrences remain reachable/exportable; next/back/reload remain
stable; missing/ambiguous addresses stay separate; measure full-corpus cost.
[Current render grouping](../../server/cmd/ukinfo-server/web/app.js),
[known remaining scope](lookup-services.md).

### R6. Saved references are guarded catalogue references, not permanent archival IDs

**High research-integrity priority.** Dataset key, ingestion order, occurrence ID and
source-entry guard must agree. The guard prevents opening a wrong record, but a sample
reference is not automatically a full-import reference and future reader/repacking changes
can break links. Saved snapshots remain useful, but a reference should outlive storage
layout changes if this is to become a dependable citation tool.

Proposed work: define an archival identity based on verified source identity plus original
entry identity/stream, with explicit decoder/schema versioning where needed. Add a lookup
or migration mapping; do not silently guess a substitute. Keep display citations readable.

**Acceptance:** a saved entry reopens after a fresh same-source import with different
internal IDs, or clearly explains why identity cannot be established. Sample/full
resolution is tested and never binds to a different source entry.
[Documented limitations](../address-exploration.md#save-and-reopen-references),
[source identities](../../src/ukinfo_reader/search_export.py).

### R7. The new form needs user testing and a second accessibility pass

**High polish priority.** The duplicate postcode entry, competing nearby section and
misaligned controls have been addressed. The redesigned form still asks users to
understand exact source text, wildcard prefixes and optional indexing. The street/town
prefix explanation currently lives in the expanded address help, while those two inputs
are visible. Default first-name matching can include later given names while its rule
is inside Name search options. Both are candidates for clearer feedback.

A read-only Chromium check measured:

| Observation | Result | Interpretation |
|---|---:|---|
| Desktop form, options collapsed, 1240×900 viewport | 1,070px tall | Main search is still a substantial sidebar |
| Mobile form, options collapsed, 390×844 viewport | 1,109px tall | Initial mobile results begin far below the fold |
| Mobile results heading, document position | 1,577px from top | Submit scrolls to results; returning to edit can still be awkward |
| Label / helper / listing badge font sizes | 12 / 11 / 9px | Readability should be tested with older users and at zoom |
| “View record” target | 88×23px | Review target spacing/size; exceptions may apply |
| Mobile horizontal overflow | None observed | Width checks passed, not a full accessibility result |

[Measured aggregate observations](product-review-ui-2026-10-19.json),
[desktop form image](review-assets/search-form-desktop-2026-10-19.png),
[mobile options image](review-assets/search-form-mobile-2026-10-19.png).

Proposals: result-first mobile navigation with an **Edit search** affordance; more readable
secondary text; consistent action sizes; concise matching explanations beside results;
field-associated errors and a clear form-reset action. Test keyboard focus when submitting,
closing nested dialogs, returning from an address, and interacting with maps. The record
dialog initially focused its close button in the measured walkthrough; a screen-reader
review should decide whether another initial focus better introduces long record content.

**Acceptance:** representative users complete name, street-only, partial-postcode and
nearby searches without coaching; keyboard/screen-reader checks, 200%/400% zoom and touch
checks are recorded. W3C's [reflow](https://www.w3.org/WAI/WCAG22/Understanding/reflow.html),
[target-size](https://www.w3.org/WAI/WCAG22/Understanding/target-size-minimum.html) and
[modal-dialog](https://www.w3.org/WAI/ARIA/apg/patterns/dialog-modal/) guidance inform
that review. These measurements alone do not establish WCAG nonconformance.

### R8. Search ordering and workload coverage still need work

**Medium/high search priority.** Catalogue order is deterministic and supports paging,
but nearby users commonly want nearest first; name/street research benefits from clear
alphabetical or edition grouping. Sorting the current page in JavaScript would not sort
the search. Exact whole-result counts and facet counts can also be expensive at this scale.

The existing broad-prefix report measured one surname/first-prefix case at roughly
157–245ms across five warm local pages. That is useful evidence of variable query cost,
not a concurrency guarantee. Address/premises and geography also have specific admission
limits. Keep completeness and explicit timeout errors while measuring new access paths.

**Acceptance:** a full-result ordering contract with stable ties/cursors and tests across
page boundaries; realistic cold/warm and mixed concurrent measurements on the chosen host.
Until then, continue saying catalogue order and “more results”, not implying relevance,
nearest ordering or an exact person count.
[Prefix measurements](broad-prefix-pagination.md), [API](../api.md).

### R9. Research lists need query memory and recoverable drafts

**Medium/high workflow priority.** Lists save found evidence well. They do not record
what was searched, which editions were available, unsuccessful searches, or a plan to
check again after importing another edition. Notes require an explicit save and current
draft protection is window-local; a crash can still lose unsaved text. JSON transfer has
useful guardrails but no built-in merge/deduplication of repeated imports.

Proposals: private saved query recipes; an optional research log including zero results
and edition scope; recoverable local note drafts; tags/status such as “candidate”, “ruled
out” and “follow up”; a preview of duplicate research-backup imports. Keep actual search
history opt-in and local. Query recipes must not imply they froze the result set.

**Acceptance:** restart preserves a draft/query; research export includes explicit format
version and provenance; conflicting edits remain detectable; clearing history removes it;
queries and notes never appear in diagnostics. Existing JSON/CSV export and comparison
should be reused, not rebuilt.
[Current persistence and limits](../research-lists.md).

### R10. Edition coverage should be a research feature

**Medium priority.** The app already distinguishes samples and editions, but an empty
result can mean “not in this source”, “not covered by this sample”, “different spelling”,
“not indexed”, or “outside postcode-coordinate coverage”. It does not establish that a
person was absent from a location. The 2004 source has no separate business record file;
a tiny 2005 sample usually contains no businesses. These are important search context.

Propose a lightweight **About this search** panel: selected source editions, sample/full,
listing types available, applied interpretation/matching rules, index readiness, postcode
reference/coverage, and changes that would broaden the query. No inferred collection dates.

**Acceptance:** users can explain why a business or map result is unavailable without
reading decoder documentation. Source anomalies remain visible and are not “corrected”
by an attractive address formatter. [Edition coverage](../editions.md),
[postcodes and precision](../formats/modern-se.md#source-anomalies-and-validation).

### R11. Business discovery can use more of the verified data

**Medium priority, after core usability.** Business records expose industry/SIC information,
but the search projection does not provide a business-category filter. A user entering
“plumber” in **Business name** may expect a category search that the interface does not
promise or implement. Some industry references are ambiguous; 2004 lacks the business
records needed for an equivalent mode.

Propose a distinct, optional category/industry field using verified source classifications,
with edition coverage and unresolved values explicit. Do not silently apply current SIC
labels to historical codes or treat inferred descriptions as authoritative.

**Acceptance:** source classification/version is visible; ambiguous values stay unknown;
category/name combinations page correctly; index cost is measured before a full rebuild.
[Business decoder](../../src/ukinfo_reader/modern_business.py),
[search projection](../../server/internal/catalog/schema.sql).

### R12. Distribution language and maintenance docs need consolidation

**Medium/low priority.** The README begins with the Python reader while the product now
has a desktop app, a server and research features. Its screenshot still depicts an older
search form. Python reader version 0.5.0 and desktop version 0.3.4 are legitimate separate
versions but can confuse issue reports. “Portable” currently primarily means bundled
runtimes; default user data lives outside the app bundle and a PostgreSQL cluster is not
portable across arbitrary OS/architecture combinations. Windows CLI support is not a
Windows desktop package.

Proposals: a clear user download/setup route plus a separate developer/CLI route; a
platform/OS/source support matrix; current invented-data screenshots; a precise explanation
of app/profile/catalogue portability; a security-reporting policy (no `SECURITY.md` was
found in this checkout). Private GitHub reporting settings were not audited.

Main UI code and CSS have accumulated broad responsibilities. Split feature modules as
those areas are revisited, especially search state and persistence. Keep the shared model
and the same UI in web/desktop; a framework rewrite is not required to achieve this.

**Acceptance:** a newcomer can choose the right download, identify where their data lives,
know which version to report, and distinguish code licensing from source-data rights.
[README](../../README.md), [package versions](../../pyproject.toml),
[desktop package](../../desktop/package.json).

## 4. Ideas from other solutions

The “application” column below is a proposal, not a feature claim or instruction to build.
Sources describe patterns; their current forms were not subjected to a usability or
performance benchmark here. Do not copy interaction complexity merely because a competitor
has it, or infer that this project's source coverage matches theirs. Yell is a current
business-discovery service; this is a historical archive. Borrow its classification and
location concepts without implying that old telephone numbers or businesses are current
contacts, or that matching listings identify a present resident.

| Solution / evidence | Useful pattern | Application to this project |
|---|---|---|
| [192.com main search](https://www.192.com/) and [advanced people search](https://www.192.com/people/advanced/) | Names and location dominate the starting point; additional constraints are secondary | Keep the new hierarchy. Consider property/co-listed-person investigation later, with an explicit research hypothesis rather than a claimed relationship. Some advanced-page content was not exposed by the text fetch. |
| [Yell search guide](https://support.yell.com/support/solutions/articles/77000497072-using-yell-com) | Distinct business/category and location concepts; suggestions, classifications and broaden-search help | Add verified business categories and edition-aware place suggestions before a general natural-language box. Expose suggested interpretations; do not silently rewrite historical names/places. |
| [FreeCEN help](https://www.freecen.org.uk/help) | Coverage information, optional phonetics/name variants, nearby places, “About this Search”, citations and neighbouring dwellings | Coverage/explanation panel and a user-kept research log are immediate design candidates. Optional sound-alike variants need their own semantics and tests. Keep our complete paging instead of adopting its documented 1,000-result cap. Neighbouring entries are not automatically neighbouring buildings in these UK-Info formats. |
| [Findmypast address guide](https://www.findmypast.co.uk/help/articles/360009188397) | Address-led exploration across available source years | Make address/edition snapshots easier to compare and cite. Do not imply continuous residence or exact event dates. The help page's search excerpt was available; direct page retrieval returned 403, and paid results were not checked. |
| [FamilySearch-hosted handout, Dana Palmer](https://cms-z-assets.familysearch.org/b6/95/cd8274bce2216735782035501e8d/effective-search-strategies-on-familysearch.pdf) | Explicit search strategies combining name variation, wildcard use and location constraints | Provide short, task-specific search tips and optional expansion choices. Its supported wildcard grammar is not the same as ours; do not imply support for embedded `*`, `?` or arbitrary contains searches. |
| [Gramps Web data model](https://www.grampsweb.org/user-guide/data-model/) and [tasks](https://www.grampsweb.org/user-guide/tasks/) | Sources/citations are distinct from people/events; research tasks have statuses and evidence | Preserve a listing as a source observation, let users annotate confidence and planned checks, and export structured citations later. Avoid automatically creating people, family relationships or a life timeline from matching listings. |
| [Zotero saved searches](https://www.zotero.org/support/searching) and [notes](https://www.zotero.org/support/notes) | Query recipes are distinct from frozen collections; searchable notes and incremental note saving | Add local saved searches, recoverable drafts and optional research tags. Distinguish “rerun this search” from “this is the evidence I saved on this date”. Cloud sync is a separate project, not needed to adopt these ideas. |
| [OpenRefine reconciliation](https://openrefine.org/docs/manual/reconciling) and [clustering](https://openrefine.org/docs/manual/cellediting) | Potential matches are reviewable decisions; similar strings do not compel an automatic merge | Offer candidate comparison and explicit user labels. Keep algorithmic grouping of identical listings separate from human conclusions that two listings refer to one person. |
| [GOV.UK input](https://design-system.service.gov.uk/components/text-input/), [radios](https://design-system.service.gov.uk/components/radios/), [details](https://design-system.service.gov.uk/components/details/) | Persistent hints, visible labels, explicit alternatives, selective disclosure | The latest form already applies these patterns. Next research should validate discoverability and matching comprehension, rather than adding another nested set of options. |

## 5. Proposed next increments, in order

**None of these are implemented by this review.** Effort is relative: S = contained,
M = cross-component, L = data/index or substantial lifecycle work. These are not time quotes.

| Order | Proposed increment | Relative effort | Dependencies / exit condition |
|---|---|---|---|
| 1 | Signed 0.3.4 draft plus clean install/upgrade checklist | M | R2; keep unpublished; one immutable source commit |
| 2 | Recovery/backups design and a restore rehearsal | M–L | R1; test failure paths before convenience automation |
| 3 | Five-user task study, readability/keyboard/zoom review | S–M | R7; record task success, errors and assistance needed |
| 4 | About-this-search/coverage summary and precise no-results guidance | M | R10; reuse manifests, capabilities and existing filter chips |
| 5 | Private saved searches and a research log, with draft recovery | M | R9; versioned local storage and safe transfer semantics |
| 6 | Place-only suggestions from installed, indexed editions | M | Bounded API/query design; no external geocoder dependency required |
| 7 | Stable archival reference design and compatibility mapping | M–L | R6; settle identity contract before more sharing integrations |
| 8 | Whole-search grouping spike, then measured implementation | L | R5; group paging and underlying-source preservation proven first |
| 9 | Nearest/name/edition ordering spike | M–L | R8; full-result order, stable cursors and host budget established |
| 10 | Business classification search | M–L | R11; historical code semantics and index footprint verified |
| Parallel planning | Publication/operations checklist and shipped-component update policy | M | R3/R4; prerequisites to public real-data hosting, not extra desktop search fields |

Small polish candidates to evaluate during the task study: **Edit search** near results;
consistent button labels and target sizes; a clear-query action before a search; matched
text/reason highlighting; visible unsaved/saved note status; concise source badges with
expanded provenance; import-phase elapsed time and space estimates; structured diagnostic
failure codes. Preserve the current exclusion of private data from diagnostics. The new
report gives lifecycle/error-presence clues, not the raw failure reason, so some support
cases will still require a targeted follow-up.

### Explicitly defer

- A new database/language/UI framework solely to pursue hypothetical performance.
- Automatic identity merging, invented family links, or “moved here” narratives.
- Unrestricted full-result exports in the hosted UI without resource/access decisions.
- Unattended updates before recovery and compatibility checks.
- A public real-record service before its publication and operational gates.
- More map animation or exact-looking house pins using postcode centroids.
- AI/semantic search before explicit search, source fidelity and research workflows are
  understandable. Gramps has semantic features, but that does not establish a useful
  need or reliable interpretation for these particular directory records.

## 6. Validation plan for the next decision

### Usability: small, concrete tasks

Recruit a mix of a genealogy researcher, a house-history researcher, a first-time user,
an older/low-vision user, and a keyboard/screen-reader user. This is an initial sample,
not a statistically representative study. Use invented scenarios:

1. Find a named listing in a district, then include additional given names.
2. Find who is listed on a street without knowing a name; explain all applied filters.
3. Change a postcode query into a two-mile nearby search without entering the code again.
4. Compare a property in two available editions, save evidence and explain what it
   does and does not prove.
5. Quit/reopen, recover a draft, export/import a research backup and attach diagnostics.

Observe first successful action, task completion, reformulations, mistaken assumptions,
help required, and whether source/sample/coordinate limitations are understood. Do not
log original records or personal research notes as product telemetry. The proposed draft
recovery task tests a future feature; until built, it documents the current gap.

### Hosting capacity: measure the actual service shape

Use the existing validated load tool on the candidate host. Include cold and warm exact,
broad-prefix, all-edition, address-only, premises, nearby and concurrent page-export cases;
interleave them rather than reporting only hot exact keys. Include empty/invalid queries,
slow clients, repeated pagination, and overload/recovery. Measure p50/p95/p99, timeouts,
rejections, CPU, resident memory, DB waits/connections, storage/WAL/temp growth and bytes.
Keep fixture values private and reports aggregate-only. Set service objectives after
observing the host and intended workload; no new public request-rate promise is justified
by this review. [Existing load tool](../../tools/search-load/README.md).

### Desktop: test user's work, not just a successful launch

Cover clean browser downloads on each supported Mac architecture, the Linux baseline,
installation/move, first launch offline, known source import, cancel/restart, low disk,
crash/recovery, duplicate source selection, upgrade with existing notes/catalogues,
backup restoration and subsequent rollback. Exercise a wrong/corrupt source without
running its binaries. Keep source media outside public CI and retain invented regression
fixtures. Record compatibility limits instead of labelling all platforms “portable”.

## 7. Recommendation for the next morning

Review **R1–R4** as release/hosting decisions, then choose a small usability and research
workflow increment from **R7/R9/R10**. Whole-search grouping and sorting deserve explicit
spikes with acceptance tests, not another optimistic UI-only promise. Preserve the
source-fidelity, safe local persistence and bounded query behavior already established.

No new feature, index, database migration, dependency change, release tag, signing run,
public issue or public deployment was initiated as part of this review. Only this report
and its aggregate/screenshot evidence are new review artifacts.
