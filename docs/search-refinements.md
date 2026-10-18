# Refine names and individual premises

The web interface and portable app share these controls. House/building/flat filters
work on an existing catalogue without an index build or another data copy.

## Houses, buildings and flats

Open **Search by house, street, town or county**. Add:

- **House number / suffix**, for example `12` or `12A`.
- **Building name**, for example `Rose House`.
- **Flat / subbuilding**, for example `Flat 2`.

Combine them with at least one indexed constraint: surname/business name, street,
locality, town, county, postcode or nearby area. A building name or first name alone
cannot start a national search. Postcode districts and sectors are accepted, although
narrower locations reduce the source records that need checking.

All supplied fields use AND. Case and repeated whitespace are ignored; exact house
`12` does not match `12A`. A trailing `*` requests a prefix (`12*`, `Rose*`, `Flat*`).
House and flat prefixes need one character; other address prefixes need two. Embedded
wildcards, `?` and `%` are unsupported. No spelling or abbreviation variants are guessed.

Only displayed source fields are checked. Missing fields, raw references and unresolved
hidden substrings do not provide matches. Numeric and text integer house numbers match;
the older format's house number `0` is blank. A flat filter needs that recorded flat
value, and does not include records missing the flat. A house filter without a flat
filter includes all otherwise matching listings at that number; it is not a household
identity assertion. Source warnings remain visible on matching records.

House/building/flat controls remain usable with name/postcode constraints even when
street/locality/town/county indexing is unavailable. Those broader location controls
still require the [optional address index](address-search.md) in each selected edition.

## Given names and middle initials

The **First-name matching** selector overrides the main name matching selector for
`first_name` only:

| Mode | Behaviour |
| --- | --- |
| Use name matching above | Existing exact/prefix behaviour applies to supplied name fields. |
| Exact given names | The entire recorded given-name field must match. `John` excludes `John A`. |
| Given name (+ others) | Match `John`, `John A` and `John Arthur`, but exclude `Johnny`. Multiple supplied given names must also occur at the start in that order. |
| Starts with | Literal prefix of the recorded given-name field, including longer names. |

A trailing `*` explicitly requests a prefix even when another mode is selected.
**Middle initial** checks the first letter of the **second** recorded given-name token;
periods separate tokens for this check. `A` matches `John A`, `John A.` and `John Arthur`.
It excludes missing middle names and does not inspect later given-name tokens.

Older editions can contain initials instead of full names. `John` does not imply `J`,
and initials do not expand to guessed names. Middle initials and given names require a
surname or location constraint. These controls apply to people, not businesses.

## Refine, broaden and save

Above results, each active filter has a remove button. Removing a filter starts at the
first page; browser Back restores the previous query and page. **Clear filters** retains
the selected edition and directory type. If no indexed constraint remains, the app asks
for one instead of issuing a national premises scan.

Zero-result searches offer applicable actions, such as removing a flat, house, building,
middle initial or given-name filter, trying a name prefix, or searching all editions.
These are explicit new searches; the app does not silently relax the requested filters.
URLs, paging, reload and JSONL/CSV page exports retain the filters. The original JSONL
export is emitted on the server to preserve large source integers.

## API and execution

`/api/search` and `/api/export` accept the additional parameters:

| Parameter | Values |
| --- | --- |
| `house`, `building`, `flat` | Exact displayed text or trailing prefix; maximum 256 UTF-8 bytes. |
| `first_match` | Omitted, `exact`, `given`, or `prefix`. |
| `middle_initial` | One Unicode letter, with an optional final period. |

```text
/api/search?dataset=all&postcode=WS13&house=12&flat=Flat%202
/api/search?dataset=all&surname=Smith&first_name=John&first_match=given&middle_initial=A
/api/search?dataset=all&kind=business&postcode=WS13&building=Rose*
```

Invalid filters return 400. Changed filters invalidate a cursor. The existing edition
sample-exclusion rule applies to all-edition searches.

Indexed name/postcode/location predicates select candidate occurrences first. Additional
premises, given-name-boundary and middle-initial checks read batches of at most 100
lossless source records. Scanning continues through nonmatching batches until a full
page plus lookahead is known, or candidates are exhausted. A continuation cursor points
to the last returned match, so later matches in that candidate batch are retained.

These source-refined requests share the four-request address admission limit and a
five-second total deadline. Individual ordinary SQL statements retain the serving
role's timeout; address-index statements use their existing bounded transaction. A
failure/timeout returns 503, never a silently incomplete successful page. Narrow the
indexed constraint when a broad sparse refinement exceeds the deadline. Plain indexed
name matching retains its existing two-second request deadline.

Full-postcode searches now use postcode-leading ordering, as district/sector searches
already did. Since the exact postcode is constant, this retains the same occurrence
order and avoids a poor name-order plan for selective postcode/given-name combinations.

[Validation and representative measurements](research/search-refinements.md).
