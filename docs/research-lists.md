# Research lists

Open a record and choose **Save to list**, or use **Save address** in an address view.
Choose a named list (or create one) and add a note. **Research lists** in the header
opens your collection without requiring a search or account.

## What is saved

Each entry retains a readable name/address summary, edition, listing type, telephone,
source warnings, citation, guarded catalogue reference, note and save/update timestamps.
Address entries also retain their edition scope. They reopen at the first address page,
not an arbitrary previous page cursor. The source JSON itself is not duplicated.

Summaries are snapshots captured when saved. **Open source** checks the dataset,
occurrence ID and source-entry guard against the current catalogue. An absent edition
or changed reference produces an error; it does not silently open another listing.
Summaries and notes remain readable even when the source is unavailable. The existing
[catalogue-reference limitations](address-exploration.md#save-and-reopen-references) apply.

Select up to four entries for a side-by-side **Compare selected** table of saved fields
and notes. Selecting listings from different editions does not prove person identity,
family relationships or continuous residence. **Map selected** plots up to 100 entries
using their displayed postcodes; see [maps](maps.md).

Find entries by name, address, edition, citation or note. Lists render 50 entries at a
time with a **Show more** button. Notes are plain text and require **Save note**.
Unsaved drafts stay in the current window while filtering/switching lists; closing the
window can lose them. Reloading lists preserves those drafts for review. A conflicting
write from another tab is rejected rather than overwriting its saved notes.

An identical source reference/view/scope cannot be saved twice in one list. It can be
saved in different lists. Remove-entry and delete-list actions require an explicit
second confirmation. Deleting a list also deletes its saved notes.

## Storage and backups

- **Web:** browser local storage for this site's origin. Other browsers, ports and
  domains have separate collections. Clearing site data or using private browsing can
  lose lists. Nothing is uploaded to the hosted search service. Web saves require HTTPS
  or localhost and a browser with Web Locks support, so simultaneous tab writes are
  serialized; unavailable locking produces an error instead of risking lost notes.
- **Desktop:** `research-lists.json` in Electron's user-data directory, or the directory
  selected by `UKINFO_DATA_DIR`. It is independent of the temporary local server port
  and survives app restarts. A main-process-only bridge validates writes, serializes
  revisions and atomically replaces the file with private permissions. Invalid files
  are preserved and block writes until repaired/restored; they are never reset silently.
  Quitting with unsaved note drafts offers **Keep editing** before the local database
  is stopped; discarding the drafts requires choosing that action explicitly.

Use **Export all lists (JSON)** for a backup or transfer between web/desktop installations.
**Import a research backup** previews its list/entry counts, then **Add imported lists**
adds independent copies with fresh local IDs. Existing lists and notes are retained;
importing the same backup again creates additional copies. JSON backups contain saved
summaries and notes, so share them intentionally. Unsaved drafts are not exported.

**Export list (CSV)** provides readable entries, source citations, notes and dates for
the selected list. Spreadsheet formula-like values are escaped. CSV is for reading;
use JSON for restoring lists. Browser quota/storage failures are reported without
claiming a save succeeded.

## Format and limits

The portable JSON format is `ukinfo-research`, version 2 (version 1 is accepted), with a revision counter and
named lists. Both browser and native storage use the same validator. Limits:

- 2 MiB serialized UTF-8, at most 50 lists and 2,000 entries across all lists.
- List names: 100 characters; notes: 4,000 characters per entry.
- Bounded summary fields, positive 32-bit source occurrence IDs, source-entry guards,
  unique local IDs, record/address view type and edition/all scope.

Unknown fields are removed during validation. Invalid versions, oversized files,
malformed entries and duplicate IDs are rejected before mutation. Imported links are
stored as reference parameters, not arbitrary navigation URLs. Display uses text nodes,
not imported HTML. Native IPC accepts only the owning archive window's main frame.
The collection has no background sync, cloud account or multi-device merge service.

## Recoverable drafts and saved searches

Notes still require **Save note**. While editing a saved entry, a separate local draft
is written after a short pause. Wait for **Drafts saved for recovery** before relying
on recovery; storage failures are shown in the research status. Reloading restores
drafts without changing committed notes. **Discard draft** keeps the saved note.
When a saved note changed since a recovered draft began, compare both versions before
saving. Another tab's changed draft is preserved and a conflict is reported; copy
unsaved edits before reloading to resolve it. The desktop quit dialog offers keeping
drafts, discarding them, or continuing to edit. The note on a not-yet-saved new entry
is not a recoverable draft: save the entry before closing that dialog.

**Saved searches** stores query recipes independently of results. Rerunning starts
from the first page and uses the installed catalogue. Adding/removing editions can
change results. Page cursors and external URLs are never saved as recipe fields.
Optional local research history records the last 200 successful first-page searches,
including zero matches, with source coverage and page counts. It is off by default;
pausing retains existing history, while **Clear search history** removes it. Errors
and subsequent pages are not recorded as new research attempts.

Desktop lists use `research-lists.json`; recipes, history and drafts use a separate
versioned `workbench.json`. Both are private files, included in a full archive backup,
and excluded from diagnostics. Browser storage is specific to the site's origin.
Browser clearing/private browsing can lose it. Workspace export includes private
history and drafts; import previews and adds saved query recipes only, keeping notes,
drafts and logging settings untouched. Use full desktop backup/restore to preserve
the complete archive workspace. Version-1 files are upgraded when saved; their original bytes are retained as a compatibility backup. Version-2 exports include archival source IDs and optional research status/tags; older apps need a version-1 backup to roll back. List exports include committed notes only. Never attach either export
to public issues without first removing private content.

## Research labels and repeated imports

Each saved entry has optional **Research status** (Candidate, Follow up, Ruled out,
Reviewed) and up to ten tags of forty characters each. Open its research labels and
choose **Save labels**. These are private research judgements, not source facts or
identity confidence assigned by the app. List filtering searches saved labels too;
JSON and CSV exports retain them.

Research-backup import previews exact copies and references with different saved
content. **Skip exact copies** is selected by default. It compares list name, source
reference, saved summary, committed note and labels; timestamps alone do not create a
new version. Changed content is added separately and existing notes are never replaced.
Turn skipping off to deliberately keep separate copies. This is conservative duplicate
handling, not an automatic merge of research judgements or people.
