# Security and supported builds

Report vulnerabilities through GitHub's **Security → Report a vulnerability** for
this repository, if enabled. If that option is unavailable, open an issue requesting
a private reporting channel without including exploit details, records or credentials.
Never attach an ISO, catalogue, archive backup, research/workspace JSON or signing key.
For ordinary faults, use the desktop's diagnostic export and inspect it before sharing.

This project is a preview. Security fixes target the current main branch and the next
preview build. There is no promise of backports or a response-time SLA. Historical disc
software is never run by the importer. Supported media are parsed as untrusted input.

## Component review and updates

Each new desktop build includes `runtime/components.json`: standalone decoder packages,
Go modules/toolchain, native supplier notices, pinned PostgreSQL/cabinet sources and
executable hashes. Packaging adds the actual Electron, Chromium and embedded Node
versions. Release `.build.json` manifests include the same inventory. This inventory
is not a guarantee that every statically linked dependency or OS library is enumerated.
Review supplier notices and advisories for those components too.

Before every release, and at least weekly while maintaining a preview:

1. Review Electron/Chromium/Node security releases, PostgreSQL minor releases, Go
   security releases, Python/security advisories and bundled decoder/native suppliers.
2. Audit **all** npm dependencies; Electron is declared under devDependencies but its
   binary is shipped. `npm audit --omit=dev` alone is not a desktop security review.
   Review Go reachable vulnerability results and Python runtime advisories as well.
3. Record affected builds, impact, upgrade/mitigation decision and any accepted
   advisory in the release notes. A clean package audit is not proof of a clean binary.
4. Change explicit pins in a reviewed PR. Build all supported platforms, rerun native
   import/search/backup/restore gates, sign/notarize macOS and prepare a draft release.
5. Back up before an archive/database upgrade. PostgreSQL major changes require an
   explicit migration plan; an app update must not silently rewrite a live catalogue.

Updates are manual; there is no unattended updater. Releases remain drafts until
explicitly approved. Release publication and public hosting of historical directory
records also require their own operational and source-data rights decisions.

See [Electron's security guidance](https://www.electronjs.org/docs/latest/tutorial/security),
[PostgreSQL security](https://www.postgresql.org/support/security/),
[Go vulnerability management](https://go.dev/doc/security/vuln/) and
[desktop release gates](docs/desktop-releases.md).
