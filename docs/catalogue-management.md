# Manage a desktop archive

Open **Manage archive** in the desktop header. Management is available only in the
native app with an app-owned local PostgreSQL catalogue, through its protected
local control listener. It is disabled for `UKINFO_DESKTOP_DATABASE_URL` connections. Hosted search has no
management or deletion endpoint.

## Editions and storage

Each card shows the verified disc edition, sample/complete status, imported entry
count, import time, dedicated storage, and whether street/town/county search is ready.
Disc dates identify distributions, not dates of residence or a census reference date.

- **Dedicated storage** includes an edition's integer search projection, lossless
  compressed source blocks and their indexes. It does not include its original media.
- **Shared search dictionaries & addresses** serves multiple editions. This storage
  remains for reuse when an edition is removed; no expensive global pruning runs.
- **Postcode reference** reports the versioned location tables/indexes separately.
- **Database files** includes PostgreSQL's catalogue files. It excludes original discs,
  temporary extraction, app binaries, backups and PostgreSQL's WAL outside the database.
- **Free on archive volume** comes from the native filesystem. Temporary media
  preparation may use a different volume; it checks its own free space separately.

## Upgrade a sample

Choose **Add complete edition**. If the app still knows the original source it can
recheck it; otherwise choose that edition's ISO/7z or extracted folder. Review the
edition and **Complete edition** selection before importing. There is no automatic
full import. The existing sample stays searchable throughout.

Once the complete version is imported, All editions search already prefers it.
The management panel identifies redundant samples so you can remove them explicitly.
Removing samples is not required, and the app never silently removes them.

## Continue interrupted setup

The app keeps a small private `setup-state.json` with the last original source path,
edition, selected sample limit and setup status. The renderer sees a source label, not
an unrestricted path API. This file is local app data and is excluded from packages.

After an interruption, **Recheck previous source…** is available in the setup wizard
and management panel when the intended import is absent. The original source is
identified and fingerprinted again. Imports restart **from the beginning**, with a
review before any new ingestion; there is no byte-offset resume of a database stream.
If the source was moved, choose it again. Malformed setup history is preserved and
reported; you can still choose another source.

If record ingestion committed but address indexing was interrupted, the edition
remains searchable. Choose **Finish address search** to rebuild the missing optional
index, without duplicating its source entries. Progress and cancellation are provided.

## Remove an edition

Choose **Remove edition…** and review the native confirmation. **Keep edition** is
the default. Removal drops only the chosen import's dedicated partitions and address
memberships in one transaction. It uses a reviewed immutable import identity; a stale
confirmation cannot delete a later reimport. Removal is excluded while source import or
address indexing holds the corresponding database locks. Busy readers can cause a
short lock timeout; refresh and retry later.

Original discs, other editions, postcode locations, shared dictionaries, research
lists, snapshots and notes are preserved. There is no undo of catalogue deletion;
reimport the original source to search it again. Dedicated files can be reclaimed
when removal commits; space in shared tables remains available for database reuse.

Saved source links are unavailable while the edition is absent. Reimporting the same
verified edition may make links with matching record/source identities available again.
Research snapshots remain readable throughout and are not evidence of an identity match.

## Back up or move

Export research lists as JSON. Keep original media and the ONSPD ZIP to rebuild the
search catalogue. **Show app data folder** opens the native file manager at the app's
fixed data directory; it does not let a page open an arbitrary path.

For a full local backup, quit the app and wait for PostgreSQL to stop, then copy the
**entire** app data directory to separate storage. Restore to a separate directory
using the same app version, operating system and CPU architecture; set
`UKINFO_DATA_DIR` to that restored directory before launching. Never copy or replace a
running PostgreSQL directory. Copying just research JSON does not back up the catalogue.

Automated tests use invented data to check partition ownership, stale confirmations,
index/import exclusion, other-edition preservation, new identities on reimport,
setup-history persistence, browser actions and native confirmation/note preservation.
