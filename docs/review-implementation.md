# Product review implementation

Tracks the recommendations in [the October 2026 review](research/product-review-2026-10-19.md).
Release publication remains a separate explicit decision; all releases stay drafts.

| Recommendation | Progress |
| --- | --- |
| R1 Recovery, backup, log retention | Implemented: PostgreSQL-owned stale-lock recovery, ownership lease, verified cold backup/separate restore, archive switching and bounded logs. Native invented-catalogue round trip passed; full private-catalogue restore remains a separate rehearsal. |
| R2 Signed release and customer upgrade checks | Pending final implementation build |
| R3 Shipped-component inventory and update policy | Pending |
| R4 Hosted protection, monitoring and deployment rehearsal | Pending; no public deployment authorised by this work |
| R5 Whole-search grouping | Pending |
| R6 Archival references | Pending |
| R7 Accessibility and responsive usability | Pending |
| R8 Full-result ordering and workload coverage | Pending |
| R9 Saved searches, research log, recoverable drafts | Pending |
| R10 Search coverage/explanation | Pending |
| R11 Business classifications | Pending |
| R12 Distribution/support documentation | Pending |

Real-user studies, clean customer hardware checks and publication/legal decisions need
external participation; automated checks do not stand in for them. Implementation and
local verification can proceed while those remain outstanding.
