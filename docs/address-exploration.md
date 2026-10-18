# Record links and address exploration

Open **View record** on a search result, then **Explore this address**. The address
view lists people and businesses recorded at the same premises. It starts with the
selected source edition; choose **All available editions** to compare available
snapshots. Each listing retains its edition, listing type, source reference and citation.

Use **View source record** for original fields, copying and export. **Back to address
listings** returns to the same address page and edition scope. Address listings have
next/previous controls and a first-page control for reopening a paged bookmark.
Source listings remain separate occurrences; electoral and telephone entries can
refer to the same person. Counts are not counts of distinct residents or households.

## Conservative premises matching

A listing needs:

- A syntactically complete **displayed** postcode, a recorded street, and a house
  identifier or building name.
- Interpretable displayed address fields, full source postcode precision, and no
  unresolved source warnings. The invented-demo warning is the only exception.

Matches require the same normalized house identifier, building, subbuilding/flat,
street components, locality components, town, county and full postcode. Case and
repeated whitespace are ignored. Numeric house numbers and equivalent text match;
0 is the older source's blank house-number value. House suffixes, flats and building
names remain distinct. Empty first street/locality components can be omitted when
comparing the remaining ordered components, allowing compatible edition layouts.
No abbreviation, spelling or historical-place replacement is guessed.

Raw or hidden full postcodes, unresolved building substrings and inferred household
relationships are never used. Conservative rules can miss genuine address matches,
including those with omitted counties, spelling variations or unrelated source
warnings. Use the broader street/town/county search to investigate those manually.

The selected record itself is included and labelled. Shared addresses do not establish
family relationships, simultaneous occupancy or continuous residence. Edition labels
refer to cover-disc snapshots, not verified move-in/move-out dates. A missing result
in an edition is not evidence that an occupant moved away.

“All available editions” excludes a sample when its complete edition is present.
Standalone samples remain eligible and are labelled as partial samples. Choosing the
selected record's edition for a sample explores only that sample's imported records.

## Save and reopen references

**Copy record link** saves a direct record reference. **Copy address link** preserves
the anchor record, edition scope and current address page. Search context is omitted
from copied links. Browser history restores record/address views alongside search
results; a standalone link can open without performing a name search first.

Use **Open saved link** in the header to open a reference saved from another session
or installation. Only its record/address parameters are used; the app resolves them
against the **current catalogue** and does not navigate to or fetch the pasted origin.
This is useful when the desktop app's local server address changes between sessions.
The referenced source edition/dataset must be available in the current catalogue.

Links contain the dataset key, catalogue occurrence ID and original source-entry ID.
Dataset keys and ingestion order must agree. A source-entry guard rejects an ID that
now refers to a different source record instead of silently showing another listing.
These are catalogue references, not globally version-independent archival identifiers.
Samples have distinct dataset keys; a link to an absent sample does not guess a record
in the full edition. Citations retain the human-readable source/edition information.

## Exports

**Export original record** works from search results, direct links and address listings.
**Export address page** exports the current bounded page as NDJSON. The server emits
original record JSON without browser number parsing, preserving large source integers.
Each line includes dataset, occurrence ID, source-entry ID and original record fields.
Save multiple pages separately when needed; there is no unbounded bulk-export endpoint.

## HTTP API

```text
GET /api/record?dataset=synthetic-demo&id=1&source_id=invented%3A0
GET /api/record?dataset=synthetic-demo&id=1&source_id=invented%3A0&format=jsonl
GET /api/address?dataset=synthetic-demo&id=1&source_id=invented%3A0&scope=edition
GET /api/address?dataset=synthetic-demo&id=1&source_id=invented%3A0&scope=all&limit=25
```

`/api/record` returns `item`, `address_available` and, when unavailable, an
`address_reason`. `/api/address` returns `anchor`, `scope`, `items`, `next_cursor`
and a matching/interpretation notice. Both endpoints support `format=jsonl` for
record/page export. Pass `next_cursor` as `cursor` for subsequent address pages.

- `dataset`: an imported dataset key; `all` is not an anchor dataset.
- `id`: positive 32-bit catalogue occurrence ID.
- `source_id`: optional original-entry guard; generated links always include it.
- `scope`: `edition` (default) or `all`.
- `limit`: 1–100, default 50; cursors are at most 2,048 bytes.

Invalid references/parameters return 400, absent sources 404, changed source-entry
identity 409, unsuitable premises 422, and unavailable/timed-out reads 503. Address
cursors bind the anchor, normalized premises, scope and selected edition set. Changing
one of these, or adding an edition, invalidates a prior address cursor.

Address results are ordered by edition date/key, then source occurrence ID, not by
name. Address detail reads share the four-request address admission limit and have a
five-second total deadline. Record links have a two-second total deadline. The API's
existing read-only PostgreSQL role and source tables are sufficient; no new grants,
full-catalogue copies or premises index build are required.

Candidate lookup uses the existing exact displayed-postcode dictionary and separate
person/business postcode-index branches. It reads bounded batches of source blocks,
checks full premises components, and continues until the page and lookahead are complete
or the request deadline is reached. It does not filter only the first postcode page.
A timeout is an error, not a silently truncated address result.

[Validation results and limits](research/premises-exploration.md) describe the invented
regressions and the small, independent full-catalogue postcode-reference comparison.
