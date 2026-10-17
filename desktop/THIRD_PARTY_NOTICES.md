# Dependency notices

This application's own source is MIT licensed; see the repository's LICENSE.

Electron includes Chromium, Node.js and other dependencies. Retain Electron's `LICENSE`
and `LICENSES.chromium.html` supplied with its distribution. Go module licenses are
available through the pinned `server/go.mod`/`go.sum` dependencies; pgx is MIT licensed
and gofrs/flock is BSD-3-Clause licensed. Build dependencies are recorded in package-lock.json.

PostgreSQL is distributed under the PostgreSQL License. Native binary distributions
also bundle libraries with their own notices. The operator-supplied PostgreSQL runtime
and all of its required notices must accompany any binary bundle you redistribute.
`tools/stage_desktop.py` copies the supplied directory, including any notice files;
it does not gather missing dependency notices or grant redistribution rights.

These development bundles are unsigned and are not published as project releases.
The MIT license does not apply to original UK-Info source databases or proprietary media.
