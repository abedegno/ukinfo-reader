"""Whole-source checks of residential joins and live entry selection."""

from collections import Counter

from . import __version__
from .codecs import FormatError
from .modern import decode_residence
from .modern_reader import entry_codes


def validate_database(db, directory, progress=None):
    hashes = db.edition.verify(directory)
    places = Counter()
    for source_id in range(1, db.places.count + 1):
        row = db._cached_location(source_id)
        places[row["place_type"]] += 1
    report = dict(
        edition=db.edition.id,
        reader_version=__version__,
        scope="residential-electoral-and-telephone",
        sha256=hashes,
        streams={},
        totals=dict(entries=0, electoral_entries=0, telephone_entries=0),
        place_types=dict(places),
        checks=[
            "source_sha256",
            "fixed_table_resources_and_bounds",
            "dictionary_text",
            "postcode_street_locality_joins",
            "physical_record_boundaries",
            "surname_range_boundaries",
            "residential_name_references",
            "building_and_subbuilding_references",
            "telephone_only_deletion_selection",
        ],
        limitations=[
            "Entries are source occurrences, not deduplicated people or households.",
            "Ambiguous building substrings are exposed as building_raw and flagged; building is null.",
            "Source precision limits are retained; postcode_raw is the stored dictionary value.",
        ],
    )
    completed = 0
    total_bytes = sum(len(stream.data) for stream in [*db.streams, *db.business_streams])
    for stream_index, stream in enumerate(db.streams):
        invalid_text = Counter()
        for field, table in stream.tables.items():
            if field == "postcode":
                for source_id in range(1, table.count + 1):
                    db._address(stream_index, source_id)
            else:
                for source_id in range(1, table.count + 1):
                    if "\ufffd" in db._word(stream_index, field, source_id):
                        invalid_text[field] += 1
        # Track all physical boundaries and remove each reference as it is visited.
        pending_groups = set(stream.starts)
        pending_deletions = set(stream.deleted)
        counts, flags, warnings = Counter(), Counter(), Counter()
        offset = 0
        while offset < len(stream.data):
            record = decode_residence(stream.data, offset)
            counts["physical_records"] += 1
            flags[str(record.kind)] += 1
            indexed = offset >= stream.starts[0]
            if offset in pending_groups:
                pending_groups.remove(offset)
            if offset in pending_deletions:
                pending_deletions.remove(offset)
                counts["deleted_telephone_records"] += 1
                if record.kind not in (3, 4, 5):
                    raise FormatError("Deletion targets a record without a telephone entry")
            for code in record.name_codes:
                if code & db.name_mask > stream.tables["name"].count:
                    raise FormatError("Primary residential name reference exceeds dictionary")
            if record.postcode_id > stream.tables["postcode"].count:
                raise FormatError("Residential postcode reference exceeds dictionary")
            if record.house_id > stream.tables["house"].count:
                raise FormatError("Residential house reference exceeds dictionary")
            if (record.optional_flag40 or 0) > stream.tables["subbuilding"].count:
                raise FormatError("Residential subbuilding reference exceeds dictionary")
            building_id = stream.building_reference(offset, record)
            if building_id not in (None, 0, 0xFFFFFF):
                if not 0 <= building_id < len(stream.buildings):
                    warnings["building_reference_out_of_range"] += 1
                elif stream.buildings[building_id - 1]:
                    warnings["building_reference_inside_string"] += 1
                else:
                    db._string_at(stream.buildings, building_id)
            if building_id != record.optional_flag80:
                counts["building_corrections_applied"] += 1
            if record.optional_pc80 not in (None, 0, 10):
                warnings["partial_postcode_source"] += 1
            if indexed:
                counts["indexed_physical_records"] += 1
                for entry_type, _, code in entry_codes(record, offset in stream.deleted):
                    name_id = code & db.name_mask
                    if name_id > stream.tables["name"].count:
                        if code != 0xFFFFFF:
                            raise FormatError(
                                f"Telephone name reference exceeds dictionary at {offset}"
                            )
                        warnings["unknown_telephone_name"] += 1
                    counts[entry_type + "_entries"] += 1
                    counts["entries"] += 1
            else:
                counts["unindexed_physical_records"] += 1
            offset += record.length
            if progress and counts["physical_records"] % 1000000 == 0:
                progress(
                    completed + offset, total_bytes, report["totals"]["entries"] + counts["entries"]
                )
        if pending_groups or pending_deletions:
            raise FormatError("Surname/deletion reference is not a physical record boundary")
        report["streams"][stream.name] = dict(
            counts=dict(counts),
            source_kinds=dict(flags),
            source_warnings=dict(warnings),
            surname_groups=len(stream.starts),
            unindexed_prefix_bytes=stream.starts[0],
            building_fix_keys=len(stream.fix_offsets),
        )
        if invalid_text:
            report["streams"][stream.name]["invalid_dictionary_text_rows"] = dict(invalid_text)
        for name in report["totals"]:
            report["totals"][name] += counts[name]
        completed += len(stream.data)
    if db.business_streams:
        from .modern_business import validate_business

        residential_entries = report["totals"]["entries"]
        business_progress = (
            (
                lambda done, total, entries: progress(
                    completed + done, total_bytes, residential_entries + entries
                )
            )
            if progress
            else None
        )
        report["business_streams"] = validate_business(db, business_progress)
        report["scope"] = "residential-and-business"
        business_total = sum(s["counts"]["entries"] for s in report["business_streams"].values())
        report["totals"]["business_entries"] = business_total
        report["totals"]["entries"] += business_total
        report["checks"].extend(
            [
                "business_record_boundaries",
                "business_word_and_industry_references",
                "business_address_phone_fax_joins",
                "business_deletion_selection",
            ]
        )
    if progress:
        progress(total_bytes, total_bytes, report["totals"]["entries"])
    return report
