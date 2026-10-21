# Validation overview

The SE 4.0 disc passed a full validation of **248,690 blocks** across **9,565 postcode
sectors**:

| Stored items                            |      Count |
| --------------------------------------- | ---------: |
| Directory entries, including businesses | 46,355,850 |
| Addresses                               | 24,279,520 |
| Postcodes                               |  1,462,313 |

The SE 4.0 reader recovers full forenames, surnames, second initials, address components,
postcodes, telephone numbers and the original business fields. `household_id` groups entries
at the same stored address. It is a database address identifier; it does not prove kinship
or distinguish separate households that share an address.

For genealogy, treat this as a historical directory snapshot. The SE 4.0 files are dated
18 December 1998, but the underlying records' exact reference dates are unverified.
The data includes residential and business entries; the entry count is not a count of
unique people. No birth dates or family relationships are established by this reader.
Field meanings that remain uncertain are documented rather than inferred.

The SE 4.0 [full validation report](validation/se4-pcf94-1999-04.json) records hashes and checks.
Validation establishes internal consistency, including numeric checksums, record boundaries
and agreement between postcode relationships. Output has not been compared against a
running original Windows application.

The earlier disc has **19,018,491 usable indexed records** containing **27,983,545 entries**.
It recovers full forenames, surnames, house/building names, street, locality, town, county,
postcodes and stored telephone components. Its [validation report](validation/se-pcf78-1998-01.json)
records one excluded mismatched index pointer, 113 unindexed physical records, three unresolved
surname references, 13 indexed records with nonstandard postcode values and one inconsistent
place-navigation range. These source
issues are preserved or excluded as documented; their historical cause is unknown.

The 2004 and 2005 residential readers reconstruct the address hierarchy and separate
electoral names from telephone contacts. The 2005 deletion index removes telephone
occurrences while retaining electoral names in combined records. Full reports for
[2004](validation/se2004-pcp218-2004-07-residential.json) and
[2005](validation/se2005-pcf175-2005-06-complete.json) record every stream's
counts, fingerprints and source anomalies. All place joins were compared independently
with the original decoder routines.


The supplied [2003 SE source](formats/2003-se.md) has been validated in full:
59,886,841 entries, including 2,471,038 businesses. Its postcode-prefix joins and
17-bit house IDs have a distinct decoder. Original decoder comparisons use invented
inputs; source anomalies and the absent industry-description dictionary remain
explicit in the [report](validation/se2003-dpcs0104.json).


[Edition identities and provenance](editions.md) · [Binary format reference](../FORMAT.md)
