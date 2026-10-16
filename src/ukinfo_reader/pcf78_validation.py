"""Complete structural validation of PC Format 78, including known source defects."""

import struct
import time
from datetime import datetime, timezone

from . import __version__
from .codecs import FormatError
from .pcf78 import AMBIGUOUS_PLACE, BROKEN_INDEX_POINTER, POSTCODE, shifted, text


def validate_database(db, directory, progress=None):
    start = time.monotonic()
    hashes = db.edition.verify(directory)
    dictionary_counts = {}
    nonstandard_postcodes = set()
    place_navigation_offsets = 0
    navigation_mismatches = []
    for field, dictionary in db.dictionaries.items():
        dictionary_counts[field] = dictionary.count
        for index in range(dictionary.id_base, dictionary.id_base + dictionary.count):
            dictionary.word(index)
            if field == "postcode":
                raw = dictionary.raw(index)
                db._check("street", shifted(raw[:3]))
                if not POSTCODE.fullmatch(text(raw[3:10]).replace(" ", "")):
                    nonstandard_postcodes.add(index)
            if field == "place":
                db._location(index)
                raw = dictionary.raw(index)
                pointer = struct.unpack_from("<I", raw, 40)[0]
                if raw[34:35] != b"C" and pointer:
                    actual_place = db.decode_record(pointer)["place"]
                    if actual_place != index:
                        if (index, actual_place, pointer) != (19899, AMBIGUOUS_PLACE, 259849361):
                            raise FormatError("Unknown place navigation mismatch")
                        navigation_mismatches.append(
                            dict(
                                dictionary_place_id=index,
                                record_place_id=actual_place,
                                pointer=pointer,
                            )
                        )
                    else:
                        place_navigation_offsets += 1
    previous = None
    buildings = 0
    for key, _ in db.buildings.keys():
        text(key)
        if previous is not None and key < previous:
            raise FormatError("Unsorted building-name dictionary")
        previous = key
        buildings += 1
    if buildings != db.buildings.count:
        raise FormatError("Building-name count mismatch")
    marks = bytearray((len(db.dat) + 7) // 8)
    previous = None
    mismatches = []
    count = 0
    for count, (key, pointer) in enumerate(db.index.keys(), 1):
        if previous is not None and key < previous:
            raise FormatError("Unsorted main index")
        previous = key
        if not 12 <= pointer <= len(db.dat) - 11:
            raise FormatError("Index pointer out of range")
        if db.dat[pointer + 2 : pointer + 5] + db.dat[pointer : pointer + 2] != key:
            if pointer != BROKEN_INDEX_POINTER or key != bytes.fromhex("33557d7d44"):
                raise FormatError("Unknown index/record mismatch")
            mismatches.append(dict(index_ordinal=count - 1, pointer=pointer))
            continue
        byte, mask = pointer >> 3, 1 << (pointer & 7)
        if marks[byte] & mask:
            raise FormatError("Duplicate main index pointer")
        marks[byte] |= mask
    if count != db.index.count:
        raise FormatError("Main index count mismatch")
    offset = 12
    records = indexed = unindexed = entries = invalid_surnames = invalid_surname_entries = 0
    flags = [0] * 8
    nonstandard_records = nonstandard_entries = 0
    ambiguous_place_records = ambiguous_place_entries = 0
    while offset < len(db.dat):
        record = db.decode_record(offset)
        records += 1
        byte, mask = offset >> 3, 1 << (offset & 7)
        if marks[byte] & mask:
            marks[byte] ^= mask
            indexed += 1
            entries += len(record["names"])
            flags[record["flags"]] += 1
            if record["place"] == AMBIGUOUS_PLACE:
                ambiguous_place_records += 1
                ambiguous_place_entries += len(record["names"])
            if record["postcode"] in nonstandard_postcodes:
                nonstandard_records += 1
                nonstandard_entries += len(record["names"])
            if record["surname"] < 0:
                invalid_surnames += 1
                invalid_surname_entries += len(record["names"])
        else:
            unindexed += 1
        offset += record["length"]
        if progress and records % 1000000 == 0:
            progress(offset, len(db.dat), entries)
    if offset != len(db.dat) or any(marks):
        raise FormatError("Index pointer is not a physical record boundary")
    if progress:
        progress(offset, len(db.dat), entries)
    return dict(
        edition=db.edition.id,
        reader_version=__version__,
        validated_at=datetime.now(timezone.utc).isoformat(),
        elapsed_seconds=round(time.monotonic() - start, 2),
        sha256=hashes,
        totals=dict(
            physical_records=records,
            index_keys=db.index.count,
            indexed_records=indexed,
            unindexed_records=unindexed,
            entries=entries,
            building_names=buildings,
            place_navigation_offsets=place_navigation_offsets,
            dictionaries=dictionary_counts,
        ),
        indexed_records_by_flags=flags,
        source_issues=dict(
            mismatched_index_pointers=mismatches,
            place_navigation_mismatches=navigation_mismatches,
            ambiguous_place_records=ambiguous_place_records,
            ambiguous_place_entries=ambiguous_place_entries,
            invalid_surname_code="002c22",
            invalid_surname_records=invalid_surnames,
            invalid_surname_entries=invalid_surname_entries,
            nonstandard_postcode_dictionary_values=len(nonstandard_postcodes),
            nonstandard_postcode_records=nonstandard_records,
            nonstandard_postcode_entries=nonstandard_entries,
        ),
        checks=[
            "source_sha256",
            "fixed_dictionary_bounds_and_text",
            "postcode_street_references",
            "place_hierarchy_references",
            "place_navigation_offsets_agree_except_reported_source_defect",
            "leaf_chain_links_counts_and_payloads",
            "index_key_order",
            "index_record_key_agreement_except_reported_source_defect",
            "unique_index_pointers",
            "contiguous_physical_record_coverage",
            "index_pointers_are_record_boundaries",
            "all_record_dictionary_references",
        ],
        limitations=[
            "One mismatched index pointer is excluded from queries and exports.",
            "Unindexed physical records are excluded; their historical status is unknown.",
            "Invalid source surname references are emitted as null with the raw code.",
            "Nonstandard source postcodes are preserved and flagged.",
            "Ambiguous place references are retained as decoded and flagged.",
            "No comparison against a running original application.",
        ],
    )
