# UK-Info Archive desktop preview

A local archive search app with a guided first-launch importer, people/business and
address searches, edition comparison, research lists with notes/citations, and optional
approximate postcode maps. **Manage archive** shows edition/storage status, helps
finish interrupted setup and removes chosen imports while preserving research notes.

## Choose a download

- `darwin-arm64.zip`: Apple Silicon macOS.
- `darwin-x64.zip`: Intel macOS.
- `linux-x64.tar.gz`: Linux x86-64, built/tested on Ubuntu 22.04.

Extract the whole download. On macOS move **UK-Info Archive.app** to Applications.
On Linux keep the extracted directory together and run **UK-Info Archive**. Run as
your ordinary user. Native Windows local mode is not included in this preview.

These preview packages are **not Developer ID signed/notarized** on macOS and are
unsigned on Linux. macOS may block a downloaded preview under Gatekeeper. Check the
published checksum and source, then use Apple's normal Open Anyway flow if you choose
to run it; do not disable system protections globally. The macOS build has an ad-hoc
integrity signature, which is not a trusted publisher identity.

No original UK-Info data, catalogue, ONSPD reference or map tiles are included. The app
requires no separately installed Python, Go or PostgreSQL. On first launch, choose a
supported **ISO/7z**, an **extracted database folder**, or the invented demo. The app
prepares and verifies required database files without running the original installer.
Choose DVD 2 for PC Format 175 (2005). See [desktop setup](https://github.com/abedegno/ukinfo-reader/blob/main/docs/desktop-setup.md). Complete imports need several gigabytes of free space; the
importer preserves a 5 GiB local reserve. Existing catalogues are not deleted or upgraded
in place by installing this app.

Research lists are saved in the app's data directory and survive restarts. Export JSON
backups before moving machines or changing data directories. Postcode maps require an
optional ONSPD reference added through **Postcode setup**; online backgrounds are off until selected.

Each archive has a SHA-256 file and a build manifest recording its exact commit,
architecture and pinned runtime source. These are development preview packages, not
a claim of compatibility with every older OS or a tested public hosting capacity.
