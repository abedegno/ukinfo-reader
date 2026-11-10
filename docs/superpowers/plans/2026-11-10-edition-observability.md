# Edition Observability Implementation Plan

> Implement inline using executing-plans; user authorised proceeding autonomously.

**Goal:** Observe individual edition candidate calls without exporting values.
**Architecture:** Bounded helpers shared by trace and aggregate metric hooks.
**Spec:** ../specs/2026-11-10-edition-observability.md

- [ ] Add failing privacy/wire/unsampled/concurrency metric tests.
- [ ] Implement bounded labels/error classification and complete-row timers;
      instrument positions and pair-drain call sites without SQL changes.
- [ ] Update/test both Collector privacy gates and histogram conversion;
      add edition latency/error dashboard charts and docs.
- [ ] Verify Go tests/vet/race, exact SQL diff and synthetic Collector processing.
- [ ] Independent review; fix findings, update homelab, commit/push draft work.

Keep deployed base, no diagnostic SQL logging, no rollout of experimental paths,
no archive records/queries/hashes/credentials in git, no local Docker images.
