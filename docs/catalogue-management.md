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

Use **Create backup…** under Manage archive or the native **Archive** menu. Save
research notes first. The app stops its database and holds the same ownership lock
used by database startup while copying. It copies database files, credentials, research
lists, setup history and supported workspace files into a new private folder, hashes
every file, rereads the copy to verify hashes, then restarts the original archive.
Original media, caches and logs are excluded. At least 5 GiB free is retained.

**Restore backup…** verifies compatibility, paths and every file's checksum while
creating a separate folder. It restarts into that restored archive. Your prior archive
and the backup remain unchanged. **Open archive…** switches between existing archive
folders and remembers your choice. The default profile location retains a small private
`archive-location.json` pointer. When using `UKINFO_DATA_DIR`, that directory holds the
pointer, so the environment-selected root remains the profile selector across restarts.
If a selected drive is missing, startup offers Open archive, Restore backup or diagnostics.

Backups are physical PostgreSQL 18 copies: use the same operating system, CPU architecture
and compatible app schema. They are not a cross-platform migration format. Checksums detect
accidental corruption; they do not authenticate a backup's author. Only restore backups
you trust. Archives using external tablespaces/symlinks need a separately managed logical
backup. External database connections are not copied or switched by the desktop controls.

Backups contain directory records, research notes and database credentials. Keep them
private; **never attach a backup to a public issue**. A diagnostics export is not a backup.
Cancellation removes only the in-progress destination. A forced shutdown may leave a
hidden `.partial` folder; it is not a completed backup. The original archive is retained.
Use native Cancel copy and let the app restart instead of force-quitting during a copy.

For manual backups, quit and wait for PostgreSQL to stop before copying the entire archive
folder. Never copy a live cluster. Research-only JSON export remains available in Research
lists. Both server and PostgreSQL logs retain at most an 8 MiB current file and three rotated
files; old log contents are not copied into backups or diagnostic exports.

PostgreSQL checks stale PID files using `pg_ctl status` and its own startup lock/shared-memory
checks. The app does not delete PID files or adopt another running server. Live or uncertain
ownership still fails with existing files preserved.

## Report a problem

Use **Get help → Export diagnostics…** to save a support summary for an issue. The
report excludes records, research notes, logs, credentials and paths. It is also
available in the native **Help** menu and startup error dialogs. See
[desktop diagnostics](diagnostics.md).
