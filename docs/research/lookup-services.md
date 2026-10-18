# Directory search comparison and implementation order

Reviewed public forms/help on 17 October 2026. Paid results were not exercised.
These are documented product patterns, not a performance or data-coverage comparison.

| Service | Documented patterns | Source |
|---|---|---|
| 192.com | Building/street/locality/town/county fields, middle initials, co-occupant search | [Advanced people search](https://www.192.com/people/advanced/) |
| 192.com business | Business classifications, source/location refinement, distance ordering | [Business guide](https://www.192.com/help/tools-guides/findbusinesses/) |
| Yell | What/where, place and business suggestions, postcode areas/districts, spelling/synonym assistance | [Search guide](https://support.yell.com/support/solutions/articles/77000497072-using-yell-com) |
| Findmypast | Property-first address exploration, street/location search, variants and radius | [Address guide](https://www.findmypast.co.uk/help/articles/360009188397) |
| FamilySearch | Per-field exact matching and multi/single-character wildcards | [Historical records instructions](https://www.familysearch.org/en/help/helpcenter/article/search-instructions-for-historical-records) |
| FreeCEN | Optional phonetics, first-name variants, multiple counties, nearby places and coverage | [Search help](https://www.freecen.org.uk/help) |

## Agreed order

1. Address indexing and street/locality/town/county search, with a bounded pilot first.
2. Address pages with premises preserved, then co-listed occupants within the same edition.
3. Group identical listings across the whole search and compare available edition snapshots.
4. Refine location controls, name matching, ordering and result filters.
5. Business categories where verified source classifications exist.
6. Durable record links, saved research lists and optional map exploration.

## Current increment

The additive address index supports the first item, including existing catalogues.
See [address search](../address-search.md). It shares normalized location tuples and
sorted occurrence-ID postings rather than adding text columns to 189 million existing
rows. Displayed source fields alone supply the index; source blocks remain lossless.

The second item is now implemented through [address exploration](../address-exploration.md):
an independently checked premises key preserves house, building and flat distinctions,
with paged people/business listings and edition-scope comparison. It uses the existing
postcode projection and source blocks without another full-catalogue index. See the
[validation results and limits](premises-exploration.md).

A street/location match must not be presented as a household, a person match, or evidence
of residence across missing years. Edition dates remain cover-disc dates rather than
verified listing/event dates. Whole-search deduplication in item 3 remains separate
future work; current grouping is page-local and address pages retain source occurrences.

Guarded record/address links and reopening saved links are also implemented. These
references depend on matching dataset keys and ingestion order; globally durable source
identifiers, saved research lists and map exploration remain future work.

Item 4 now includes [house/building/flat and given-name refinements](../search-refinements.md),
middle initials, removable filter chips and explicit zero-result broadening actions.
Place autocomplete, additional ordering choices and whole-search grouping remain open.

Later query expansion should be explicit and explain its matches. Broad searches must
retain complete pagination. Result counts and sorts need full-corpus benchmarks before
being presented as cheap/exact operations. Map rendering can be optional and lazy-loaded
so that ordinary web and offline desktop searches incur no tile-provider dependency.
