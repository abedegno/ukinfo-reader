# Desktop setup and disc import

The first launch opens **Set up your archive**. Start with **Try invented demo**,
choose a supported ISO/7z, or choose a folder containing extracted database files.
The application never runs the original installer. No directory data is bundled.

## Add an edition

1. Choose **Add edition → Choose ISO or 7z…**. For PC Format 175 (2005), choose
   **DVD 2**; DVD 1 does not contain UK-Info. Wait for the browser download to finish;
   `.crdownload` files are not accepted.
2. The app finds the database, extracts the required files and verifies every
   required SHA-256 against the [supported edition profiles](editions.md).
   Preparation shows progress and can be canceled. The 2004 and 2005 editions need
   their InstallShield cabinet tables; these are extracted by a bundled helper.
3. Review the identified edition. Choose a first-10,000 sample or the complete
   edition, then **Import edition**. A small 2005 sample normally has no businesses.
4. Choose **Start searching**. A sample is explicitly labelled; add the complete
   edition separately when ready.

ISO files are read directly. A 7z may contain one ISO or one complete database
folder. Ambiguous collections, incomplete files, encrypted archives and unsupported
editions are rejected. Select one disc at a time. The original media is unchanged.
Installers stored in other container formats are not supported by direct import;
use **Choose source folder…** after extracting the required files yourself.

### Temporary storage

For a 7z containing an ISO, allow space for its uncompressed ISO plus required
database files and cabinets. A 2005 cover DVD needs roughly 6 GiB of temporary
headroom **in addition to the 5 GiB reserve**; the catalogue also needs room to grow.
The exact check uses the archive sizes and edition profiles. Disk space is checked
while writing, and a database import also checks the catalogue volume.

The temporary ISO and cabinets are removed after preparation. Verified database
files are kept for sample/full imports until another source is selected or the app
quits. Cancellation removes the current temporary extraction. A forced OS/process
termination can leave an `ukinfo-disc-*` directory in the OS temporary directory;
close the app before removing such abandoned directories. No original media is removed.

## Add postcode locations

Open **Postcode setup** in the desktop header. This is optional: ordinary directory
search needs no postcode reference.

1. Follow **official ONSPD ZIP** to download the exact release shown in the app
   (currently **August 2026**, `2026-08`). Keep the ZIP intact.
2. Choose that ZIP and click **Import postcode locations**. The app checks its pinned
   SHA-256, reads CSV data directly inside the ZIP and prepares the spatial index.
   No CSV extraction or separate Python/PostgreSQL installation is needed.
3. The installed reference/count is shown after completion. Nearby controls become
   available immediately. Postcode maps work offline; street-map tiles remain opt-in.

The reference covers Great Britain, including current and retired postcodes. Northern
Ireland and Crown dependencies are excluded. Coordinates identify approximate postcode
points, not individual buildings. The app shows ONS, OS and Royal Mail attribution.

Progress covers verification, loading and indexing. You can close the panel and keep
searching, or cancel. The old active reference remains available until the transaction
commits. A failed/canceled transaction does not publish a partial reference. If a
cancellation arrives after commit, check the installed status before retrying. Importing
an already installed release leaves the existing reference unchanged.

The release, official URL and SHA-256 are pinned together in the Go backend; the UI
cannot override them. Supporting a later release requires a verified code update.
The reference is stored in the local catalogue, so subsequent launches do not need the
ZIP. Keep a separate copy if you need to rebuild the catalogue.

## Implementation and verification

The renderer receives opaque native-picker selection IDs. File paths and the local
server's random import capability stay in Electron main. The postcode import runs
through the server that already owns PostgreSQL, with the same import exclusion,
progress and cancellation channel as directory ingestion. Hosted web users have no
native file-import controls.

ISO reading uses pycdlib; 7z uses py7zr with bounded output writers and fixed output
filenames. The cabinet helper statically links pinned MIT-licensed unshield and saves
one numeric cabinet entry to a canonical profile path. Each output has a size bound
and every final database file is fingerprinted before it can be decoded.

Automated tests use invented ISO/7z contents for canonical names, nested archives,
size/hash failures, disk headroom and cleanup. Browser tests cover postcode setup and
reference activation. Native packaged-app tests reject a wrong ZIP and retain existing
search/list behavior. Optional local end-to-end checks accept `UKINFO_TEST_ONSPD`
(the pinned ZIP) and `UKINFO_TEST_MEDIA` (a supported ISO/7z) when running
`npm --prefix desktop run test:import`; the latter imports a 10,000-entry sample.

## Manage, recover or back up

Open **Manage archive** for edition storage/status, sample-to-full actions, confirmed
removal and interrupted-setup recovery. See [catalogue management](catalogue-management.md).
For release maintainers, [macOS signing](macos-signing.md) describes the optional
credential-backed candidate workflow; ordinary previews remain ad-hoc signed.
