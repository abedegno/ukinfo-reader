# Product review implementation

Tracks the recommendations in [the October 2026 review](research/product-review-2026-10-19.md).
Release publication remains a separate explicit decision; all releases stay drafts.

| Recommendation                                            | Progress                                                                                                                                                                                                                                                                                                                                                      |
| --------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| R1 Recovery, backup, log retention                        | Implemented: PostgreSQL-owned stale-lock recovery, ownership lease, verified cold backup/separate restore, archive switching and bounded logs. Native invented-catalogue round trip passed; full private-catalogue restore remains a separate rehearsal.                                                                                                      |
| R2 Signed release and customer upgrade checks             | 0.4.2 three-platform native gates and both Developer ID/notarized/stapled Mac candidates pass. Draft assets verified; clean customer hardware/download and full-catalogue recovery checks remain. See release evidence below.                                                                                                                                 |
| R3 Shipped-component inventory and update policy          | Implemented runtime/build-manifest inventories, Python build constraints, full npm audit, weekly npm/Go advisory checks and SECURITY.md policy.                                                                                                                                                                                                               |
| R4 Hosted protection, monitoring and deployment rehearsal | Implemented separate health pool/admission, optional private aggregate metrics and nginx TLS/quotas recipe; saturated-pool integration passes. Target-host load/restore rehearsal and public deployment remain external acceptance work                                                                                                                       |
| R5 Whole-search grouping                                  | Implemented indexed search-wide mode, group/source pagination and source-page exports, preserving occurrence mode and warning exclusions. All four complete imports indexed (189,064,266 occurrences); exhaustive bounded original-field group/source traversals pass; common national groups still time out on the small backend. See measured limits below. |
| R6 Archival references                                    | Implemented source-component fingerprint plus original entry lookup, independent of ingestion positions/keys; guarded fallbacks and research JSON v2 with v1 compatibility backups. Synthetic reordered-import resolution passes.                                                                                                                             |
| R7 Accessibility and responsive usability                 | Implemented first pass: larger text and targets, wrapping actions, Edit search and clear controls. Existing 18 search browser checks pass; keyboard/screen-reader user study remains.                                                                                                                                                                         |
| R8 Full-result ordering and workload coverage             | Implemented name, nearest and publication ordering with catalogue-bound keyset cursors. Complete synthetic pagination tests pass; full-corpus local mixed-workload measurement recorded below; actual selected-host acceptance remains.                                                                                                                       |
| R9 Saved searches, research log, recoverable drafts       | Implemented: local versioned query recipes, opt-in capped history including empty searches/coverage, transfer preview and autosaved drafts with concurrent-write protection. Browser reload/conflict flows and native persistence unit checks pass; packaged-app backup/restore also preserves saved queries.                                                 |
| R10 Search coverage/explanation                           | Implemented About this search: imported scope, samples, address availability, matching/coordinate rules, no-result and publication-date limitations.                                                                                                                                                                                                          |
| R11 Business classifications                              | Implemented resolved source industry/SIC exact/prefix filters, classification-only search and native index setup. Ambiguous values stay unknown; all 6,152,296 businesses indexed; bounded source-field validation and district pagination checked.                                                                                                           |
| R12 Distribution/support documentation                    | README desktop/web/CLI routes, platform matrix, portable/backup semantics and security/release docs implemented; invented-data screenshots refreshed.                                                                                                                                                                                                         |

[Full-corpus build, correctness and mixed-load evidence](research/review-validation.md) records
the local environment and remaining limits. Research indexes add **45.84 GiB**; the
combined development database measures **86.40 GiB**, before recovery/build headroom.

Real-user studies, clean customer hardware checks and publication/legal decisions need
external participation; automated checks do not stand in for them. Implementation and
local verification can proceed while those remain outstanding.

## Release evidence

`desktop-v0.4.2` is the immutable source `db66e23a4c8abc818f07a0485835550a876f9b0b`.
[Three-platform packages](https://github.com/abedegno/ukinfo-reader/actions/runs/39182930254),
[both signed Mac candidates](https://github.com/abedegno/ukinfo-reader/actions/runs/39182930388),
[API/browser regression](https://github.com/abedegno/ukinfo-reader/actions/runs/39182930298),
[Python tests](https://github.com/abedegno/ukinfo-reader/actions/runs/39182930239) and
[component advisory checks](https://github.com/abedegno/ukinfo-reader/actions/runs/39182930286) pass.
The [draft attachment](https://github.com/abedegno/ukinfo-reader/actions/runs/39183586365)
checks all nine uploaded asset sizes/digests and retains draft status. Linux remains unsigned.
The [0.4.2 draft](https://github.com/abedegno/ukinfo-reader/releases)
is unpublished; earlier 0.4.0/0.4.1 drafts are superseded.

The measured API/search UI is unchanged from `e56720f`; the later patch versions address
Apple ticket connection failures and perform the full notarization gate immediately
before creating the archive. They retain the Apple Accepted, ticket, strict signature
and Gatekeeper requirements; failed checks cannot produce signed release metadata.
