# Search form design

The form groups questions around **Who?** and **Where?**. Every field is optional;
a search needs a surname/business name, postcode or supported address location. A
first name, middle initial, house, building or flat alone is not a search anchor.

## Main controls

- **People / Businesses** changes the name fields and hides irrelevant person options.
- **Surname / First name** or **Business name** is the name search. Case is ignored;
  a trailing `*` matches a beginning. New UI searches use exact surnames and allow a
  first name to be followed by other given names: John finds John A, not Johnny.
- **Postcode** is entered once. **In this postcode** matches a district, sector or full
  postcode. **Nearby** uses that same value as the centre and reveals a distance.
  It is disabled when a postcode reference is unavailable; ordinary postcode matching
  stays available. Nearby uses approximate GB postcode positions, not exact houses.
- **Street / Town or city** stay visible. They can be used without a name, combined
  with a postcode, or combined with names. These are separate indexed source fields;
  the UI does not imply a free-text place parser or autocomplete that the backend lacks.

**Name search options** reveals explicit surname/business and first-name matching
controls and a middle initial. **More address filters** reveals house, flat, building,
locality and county. Hidden active refinements open when restoring a saved search;
result chips show the actual submitted constraints and allow their removal. The form
submits on Search rather than making a database request on every keystroke.

## Compatibility

API parameters are unchanged. Existing links without `first_match` retain their previous
inheritance from `match`; the UI displays the corresponding explicit first-name mode.
New searches set `first_match` independently when a first name is supplied. A postcode
mode switch keeps the typed postcode but changes which API parameter receives it:
`postcode` or `near` (plus `radius_m`). A normal mode switch never sends both.

Older links can deliberately contain both `postcode` and `near`. Those constraints are
preserved, explained beside the postcode and represented by separate result chips.
Remove the Postcode chip to drop the extra restriction. This avoids silently broadening
a saved search and does not introduce a second postcode input. URL restoration, paging,
filter removal and browser history continue to use the same API semantics.

## Research and decisions

Reviewed October 2026. These sources informed the design; this is an implementation
based on their guidance, not a claim of user-tested usability for this archive.

- [192.com](https://www.192.com/) presents name and location as its main people-search
  questions. We use that hierarchy, keeping structured address fields because our
  current service has no equivalent general location parser.
- [Nielsen Norman Group: progressive disclosure](https://www.nngroup.com/articles/progressive-disclosure/)
  recommends deferring less-used, complex controls. Matching rules and detailed address
  parts are refinements; the primary name, postcode, street and town fields remain visible.
- [GOV.UK text input](https://design-system.service.gov.uk/components/text-input/)
  recommends visible labels above inputs and discourages placeholders for examples or
  hints. The form uses persistent, associated hints and no disappearing example text.
- [GOV.UK radios](https://design-system.service.gov.uk/components/radios/) provides a
  pattern for mutually exclusive choices with conditional content. One postcode field
  and an explicit scope choice replace independent postcode and centre inputs.
- [GOV.UK details](https://design-system.service.gov.uk/components/details/) advises
  keeping commonly needed information visible. Street and town are not hidden in details.
- [GOV.UK addresses](https://design-system.service.gov.uk/patterns/addresses/) discusses
  separating address components when a service needs them individually. This is an
  archive filter form, not a form asking for the user's own address: browser autofill
  is off so a personal address does not populate a historical search.

`AA1 1AA` is our format-only illustration, explicitly marked as such. It is not a
recommended search, an installed-reference guarantee or an example supplied by GOV.UK.
The UI and documentation no longer use the previous WS postcode example. The helper
text remains visible after typing and is connected to its field with `aria-describedby`.

## Verification

Browser checks cover changing postcode scope, unavailable/activated geography, old
combined-filter links, first-name matching compatibility, address-only searches,
paging, filter removal, delayed initialization and 1240/768/390/320px layouts. Native
packaged-app tests continue to exercise onboarding, searching and address refinements.

## Explaining and revisiting results

**About this search** names the selected imported sources, sample/full status, entry
counts and address-index availability. It explains the actual name/postcode/location
rules, coordinate limitations and why zero matches do not establish absence. Edition
labels identify publications, not dates of residence. Research history records this
scope alongside the submitted criteria. **Edit search** returns keyboard focus to the
form; **Clear search fields** retains the selected edition and directory type. Text
and action sizes have been increased, and mobile record actions span the card width.
These checks improve the existing interface; they are not a complete WCAG audit or
substitute for the review's proposed user study.
