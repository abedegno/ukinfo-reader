# Dependency notices

This application's own source is MIT licensed; see the repository's LICENSE.

Electron includes Chromium, Node.js and other dependencies. Retain Electron's `LICENSE`
and `LICENSES.chromium.html` supplied with its distribution. Go module licenses are
available through the pinned `server/go.mod`/`go.sum` dependencies; pgx is MIT licensed
gofrs/flock is BSD-3-Clause licensed, and klauspost/compress includes BSD-3-Clause
and dependency notices in its module distribution. Build dependencies are recorded in package-lock.json.

PostgreSQL is distributed under the PostgreSQL License. Native binary distributions
also bundle libraries with their own notices. The operator-supplied PostgreSQL runtime
and all of its required notices must accompany any binary bundle you redistribute.
`tools/stage_desktop.py` copies the supplied directory, including any notice files;
it does not reconstruct missing notices for that native distribution or grant
redistribution rights. Staging also collects available Go-module, Go, CPython and
PyInstaller notices into `runtime/notices`; review bundled shared libraries separately.

These development preview bundles are not Developer ID signed or notarized.
Automated desktop preview packages use the source-built PostgreSQL runtime described
in docs/desktop-releases.md and include its COPYRIGHT, source notices and BUILD.json.
Leaflet 1.9.4 is BSD-2-Clause licensed; its complete license is embedded with the web
assets as leaflet-LICENSE.txt. Leaflet's published JS/CSS hashes are verified in CI.
The MIT license does not apply to original UK-Info source databases or proprietary media.

The standalone decoder bundles CPython and its standard library. Retain the Python
Software Foundation license and its included dependency notices. PyInstaller uses
GPL-2.0-or-later with a bootloader distribution exception; retain its COPYING.txt and
exception text. The project source license does not replace these notices. See
`pyproject.toml` for the pinned build tool and the build environment for exact Python
and library versions. A distributor must collect and review all applicable runtime
notices before shipping binaries; local development staging is not a release audit.

Optional ONS Postcode Directory reference data is not included in these binaries.
Great Britain data is used under the applicable Open Government Licence with OS,
Royal Mail and ONS attribution shown in the application after import. The original
UK-wide archive also contains Northern Ireland data under separate terms; the current
importer excludes that data and the Crown dependencies. See docs/geography.md.
