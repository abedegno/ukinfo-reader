# Desktop diagnostics

Choose **Help → Export diagnostics…** in the native app, or **Manage archive → Get
help → Export diagnostics…**. Choose where to save the JSON file, review it, and drag
it onto a GitHub issue. Nothing is uploaded automatically. If startup fails or the
archive server stops, the error dialog also offers an export before quitting.

The report contains:

- App version, packaged/development status, packaged build commit and whether the
  build contained modified tracked source files.
- Operating system/kernel version, CPU architecture and Electron/Chromium/Node versions.
- Bundled PostgreSQL and cabinet-reader versions, when their metadata is available.
- Whether custom runtime/data locations are configured, without their values.
- App lifecycle and database health; approximate free space on the archive volume.
- Setup state and sample limit; known edition IDs, sample/full status, entry counts,
  storage usage and address-index readiness for an app-owned catalogue.
- Whether a postcode reference is installed and a catalogue operation/error is present.

It does **not** include records, addresses, search queries/history, research lists,
notes, source filenames or paths, database URLs, hostnames, usernames, credentials,
control tokens, raw errors, stack traces or logs. Unknown edition IDs are reported as
`other`; custom manifest titles and source hashes are excluded. An external database's
catalogue inventory is not inspected. Health is checked with a short timeout; failed
checks appear as unavailable/unreachable rather than copying error messages.

The collector builds a fixed allowlist of fields rather than redacting a copy of logs
or application state. It does not read research files or log contents. Export works
without a healthy server, so some fields may be unavailable. There is no telemetry,
background collection or automatic issue submission. Canceling the save dialog creates
no report. The native main process writes through a temporary file with owner-only
permissions, then renames it to the chosen destination. The renderer cannot supply a
filename or access the server's control token.

## Reporting a problem

Use the [desktop bug report](https://github.com/abedegno/ukinfo-reader/issues/new?template=desktop.yml).
Attach the diagnostics JSON and describe the steps, expected result and actual result.
Use invented names and addresses when explaining searches. Crop or redact screenshots
before attaching them. Do not attach your app data folder, research backups, raw logs,
original discs or database files. Diagnostics are a support summary, not a backup or a
complete crash dump; a maintainer may ask for a specific additional observation.

## Maintainers

The JSON uses `schema: ukinfo-desktop-diagnostics`, `version: 1`. Missing optional
metadata is `null` or an explicit status. Do not assume the absence of a healthy server
means an empty catalogue. The generated timestamp is export time, not failure time.
Native tests exercise export from a working imported catalogue; unit checks inject
private markers into metadata, setup history and failures and verify their exclusion.
