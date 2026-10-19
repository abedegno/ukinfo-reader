# Desktop builds and releases

The **Desktop packages** GitHub Actions workflow builds native Apple Silicon macOS,
Intel macOS and Linux x86-64 packages. The matrix uses `macos-15`, `macos-15-intel` and
`ubuntu-22.04`; there is no cross-compiled PostgreSQL or Python runtime. Windows local
PostgreSQL supervision remains unsupported. The Python reader's Windows CI is separate.

## Gates and artifacts

For matching main-branch pushes, pull requests, manual dispatches and `desktop-v*` tags:

1. Download official PostgreSQL 18.6 source, verify its pinned SHA-256, build a minimal
   native runtime and `cube`, retain source notices, fix library relocation paths, and
   check executables after moving the installation prefix.
2. Build the Go service and standalone PyInstaller decoder; collect runtime notices.
   A runtime component inventory records Python packages from decoder analysis, Go
   modules and source versions; packaging adds actual Electron/Chromium/Node versions.
   External decoder libraries retain supplier notices from the build's Homebrew/Debian
   packages, recorded in a separate dependency manifest; missing notices fail staging.
3. Check native PostgreSQL lifecycle, desktop unit tests and the full npm dependency audit (including devDependencies).
4. Generate the native icon from the repository SVG, package Electron, and apply an
   ad-hoc integrity signature on macOS.
5. Launch the **packaged** app: complete the empty-catalogue wizard, import invented
   data, search/export, save a research note, close/relaunch, reopen its guarded source,
   and verify the owned PostgreSQL child shuts down. A second native gate performs a
   version-1 research upgrade, retaining the original bytes for rollback, followed by a
   verified cold backup/separate restore that checks notes, saved queries and the original archive.
6. Reject obvious source/user-data artifacts, create ZIP/tar.gz archives, SHA-256 files
   and per-platform build manifests. Upload non-PR artifacts for 3 days. Published release assets are retained separately.

Only a successful three-platform tag build creates/updates a **draft prerelease**.
The release job verifies all archive checksums, versions and commits before uploading.
It has narrowly scoped contents-write/actions-read permissions; build jobs are read-only.
Published releases are not overwritten on a rerun. Artifact checksums establish integrity,
not a signed publisher identity or bit-for-bit reproducibility of toolchains.

The separate **Tests** and **Search application** workflows retain Python correctness,
Go race/vet, API/PostgreSQL and browser regression coverage, including list validation,
conflicting writes, map precision and source-free fixtures.

## Cut a preview

1. Set `desktop/package.json` and its lockfile root version together.
2. Merge reviewed code and verify all CI workflows, including native packages.
3. Tag that commit `desktop-vVERSION` and push the tag. A mismatched package version fails.
4. For signed Mac downloads, run **Signed macOS candidates** for this tag. After it
   and the tag's Desktop packages run succeed, use **Attach signed desktop candidates
   to draft** with both run IDs. See [macOS signing](macos-signing.md).
5. Inspect the draft, checksums and manifests, then test downloading/opening the apps
   on clean machines. Keep releases as drafts until explicitly approved for publication;
   no branch push or signing workflow publishes automatically.

No auto-updater is installed. Replace the application bundle to update; keep the user
profile/data directory. Back up research JSON and the catalogue before moving data or
planning a schema migration. Physical PostgreSQL clusters are OS/architecture/version
specific and are not a portable interchange format.

## Native local build

Requires a C compiler/GNU make, Bison/Flex, Python, Go and Node. Linux also needs
`patchelf`, Xvfb and Electron system libraries (the workflow lists packages).

```sh
python tools/build_postgres_runtime.py --output private/native-pg
python -m pip install -e '.[desktop]'
python tools/stage_desktop.py --postgres private/native-pg
cd desktop
npm ci
npx playwright install chromium
node build-icon.cjs
npm run pack
UKINFO_PG_BIN="$PWD/../build/desktop/pg/bin" \
  UKINFO_TEST_APP='/absolute/path/to/packaged/app/executable' npm run test:import
cd ..
python tools/package_desktop_release.py
```

The runtime builder will not replace an existing output directory. It checks for a
5 GiB free-space reserve before building, uses temporary source/build files and removes
them afterwards. Runtime builds deliberately omit optional ICU, SSL, compression,
readline, LDAP and other external libraries: the supervisor uses private loopback SCRAM
connections, and the Go client implements PostgreSQL transport. `BUILD.json` records
actual configure options and the source URL/hash. Source dependencies must be reviewed
when updating PostgreSQL; the script does not fetch an unpinned "latest" release.
CI caches the native runtime by architecture, runner and builder-script hash. All
restored runtimes still pass lifecycle and packaged-app checks before release.

## Signing and platform limits

Ordinary workflow artifacts are macOS ad-hoc signed previews. The separate signed
candidate workflow produces Developer ID signed, Apple-notarized and stapled Mac apps;
both architectures passed this path for
[0.4.2](https://github.com/abedegno/ukinfo-reader/actions/runs/39182930388). The manual draft-attachment workflow
verifies and replaces Mac assets after all builds succeed. Check each release's notes
and build manifests for its actual signing status. Credentials are held in GitHub
secrets. Linux archives remain unsigned and target the CI baseline, not every Linux
distribution. Test hardware/OS compatibility before broad distribution.

Primary references: [GitHub runner architectures](https://docs.github.com/en/actions/reference/runners/github-hosted-runners),
[PostgreSQL native build/relocation guidance](https://www.postgresql.org/docs/18/install-make.html),
and [Apple's opening guidance](https://support.apple.com/guide/mac-help/open-a-mac-app-from-an-unknown-developer-mh40616/mac).

## Disc extraction runtime

Staging builds `ukinfo-cabinet` from unshield 1.6.2's hash-pinned official source
using `tools/build_cabinet_runtime.py` and the narrow `tools/cabinet.c` wrapper.
CMake, a C compiler and zlib headers are required (Linux: `cmake zlib1g-dev`). The
helper statically links libunshield and its MD5 object; only system zlib/libc remain
dynamic dependencies. macOS adds an ad-hoc integrity signature. Full upstream MIT
and MD5 source notices and source URL/hash are retained under `cabinet-notices`.

The standalone decoder includes pinned pycdlib/py7zr readers and their wheel/native
supplier notices. The runtime is self-contained; end users need no 7-Zip, unshield,
Python or compiler installation. See [desktop setup](desktop-setup.md).

## Signed candidates

The manual **Signed macOS candidates** workflow is separate from ordinary preview
builds. It requires Apple credentials in the `desktop-signing` environment and
produces reviewed artifacts without publishing. See [macOS signing](macos-signing.md)
for secrets, entitlement scope, embedded-decoder signing and validation limits.

## Inventory and maintenance

Every archive contains `runtime/components.json`; release build manifests include it
for review without unpacking an executable. Staged executable hashes describe binaries
before final macOS signing; the archive SHA-256 describes the final signed/notarized
package. Signing can change Mach-O bytes, so these are distinct integrity checks.
The Python desktop build closure is constrained by `tools/desktop-constraints.txt`;
this pins versions but does not make OS toolchains/wheels bit-for-bit reproducible.
See [SECURITY.md](../SECURITY.md) for advisory review and update policy. Clean user
hardware, external-drive recovery and upgrade testing remain release acceptance work,
not claims established by a successful CI build.
