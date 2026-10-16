"""Business streams from V10.5 SE, with source dictionaries and update selection."""

from functools import lru_cache

from .codecs import FormatError
from .modern import ModernFixedTable, ModernLeafIndex, decode_business
from .modern_reader import subscriber_text, word


class BusinessStream:
    def __init__(self, mapped, suffix):
        self.name = "update" if suffix else "base"
        self.data = mapped["bus" + suffix + ".dat"]
        self.words = mapped["dd" + suffix + ".dat"]
        self.industry = mapped["lobdesc" + suffix + ".dat"]
        self.sic = ModernFixedTable(mapped["dbsic" + suffix + ".dat"])
        self.word = lru_cache(maxsize=32768)(lambda offset: self.text(self.words, offset))
        self.industry_text = lru_cache(maxsize=8192)(self._industry_text)
        self.deleted = set()
        if not suffix:
            for key, _ in ModernLeafIndex(mapped["delb.idx"]).keys():
                offset = int.from_bytes(key, "little")
                if len(key) != 4 or offset >= len(self.data) or offset in self.deleted:
                    raise FormatError("Invalid business deletion index")
                self.deleted.add(offset)

    def physical_records(self):
        offset = 0
        while offset < len(self.data):
            record = decode_business(self.data, offset)
            if record.kind not in (1, 2):
                raise FormatError("Unverified business source kind")
            yield record
            offset += record.length

    def text(self, data, offset):
        if not 0 <= offset < len(data) or (offset and data[offset - 1] != 0):
            raise FormatError("Invalid business dictionary string reference")
        end = data.find(b"\0", offset)
        if end < 0:
            raise FormatError("Unterminated business dictionary string")
        return word(data[offset:end])

    def organisation(self, record):
        return " ".join(self.word(code - 1) for code in record.word_codes)

    def _industry_text(self, reference):
        if not reference:
            return "", reference, ""
        # The source field has only 24 bits, but the base dictionary exceeds 16 MiB.
        # A match is usable only when precisely one string start has those low bits.
        candidates = [
            offset
            for offset in range(reference, len(self.industry), 1 << 24)
            if offset == 0 or self.industry[offset - 1] == 0
        ]
        if len(candidates) > 1:
            return None, None, "ambiguous_industry_reference"
        if not candidates:
            return None, None, "invalid_industry_reference"
        offset = candidates[0]
        warning = "industry_reference_wrap_recovered" if offset != reference else ""
        return self.text(self.industry, offset), offset, warning


def joined_business(db, stream_index, stream, record):
    """Reuse the verified address joins while preserving business-specific fields."""
    residence = db.streams[stream_index]
    address = dict(db._cached_address(stream_index, record.postcode_id))
    warnings = [address.pop("place_warning")]
    building = building_raw = ""
    reference = record.building_reference
    if reference not in (None, 0, 0xFFFFFF):
        if reference >= len(residence.buildings):
            building = building_raw = None
            warnings.append("building_reference_out_of_range")
        else:
            building_raw = db._string_at(residence.buildings, reference)
            building = building_raw
            if residence.buildings[reference - 1] != 0:
                building = None
                warnings.append("building_reference_inside_string")
    if record.precision not in (0, 10):
        warnings.append("partial_postcode_source")
        remove = abs(record.precision - 10)
        address["postcode"] = address["postcode_raw"][
            : max(0, len(address["postcode_raw"]) - remove)
        ]
        address["street"] = ""
        if record.precision < 10:
            for field in ("place", "locality", "town", "county"):
                address[field] = ""
    industry, industry_offset, industry_warning = stream.industry_text(record.lob_reference)
    warnings.append(industry_warning)
    subscriber = subscriber_text(record.subscriber, legacy=True)
    fax_subscriber = subscriber_text(record.fax_subscriber, legacy=True)
    row = dict.fromkeys((field["name"] for field in db.fields), "")
    row.update(
        address,
        entry_id=f"business:{stream.name}:{record.offset}",
        entry_type="business",
        source_stream=stream.name,
        record_offset=record.offset,
        kind=record.kind,
        organisation=stream.organisation(record),
        business_word_codes=list(record.word_codes),
        industry=industry,
        industry_reference=industry_offset,
        industry_reference_raw=record.lob_reference,
        sic_code=word(stream.sic.raw(record.sic_id)) if record.sic_id else "",
        sic_id=record.sic_id,
        postcode_precision=record.precision,
        houseno=db._cached_word(stream_index, "house", record.house_id),
        building=building,
        building_raw=building_raw,
        building_reference=reference,
        building_reference_raw=reference,
        subbuilding=db._cached_word(stream_index, "subbuilding", record.subbuilding_id or 0),
        phone_area_code=record.area or 0,
        phone_subscriber=subscriber,
        phone=f"0{record.area} {subscriber}" if record.area and subscriber else "",
        fax_area_code=record.fax_area or 0,
        fax_subscriber=fax_subscriber,
        fax=f"0{record.fax_area} {fax_subscriber}" if record.fax_area and fax_subscriber else "",
    )
    if any("\ufffd" in value for value in row.values() if isinstance(value, str)):
        warnings.append("invalid_dictionary_text")
    row["source_warning"] = ";".join(sorted(set(filter(None, warnings))))
    return row


def validate_business(db, progress=None):
    """Visit every physical business record and all its joins, including deletions."""
    from collections import Counter

    reports = {}
    completed = entries = 0
    total_bytes = sum(len(stream.data) for stream in db.business_streams)
    for index, stream in enumerate(db.business_streams):
        pending = set(stream.deleted)
        counts, kinds, warnings = Counter(), Counter(), Counter()
        for record in stream.physical_records():
            if record.kind not in (1, 2):
                raise FormatError("Unverified business source kind")
            row = joined_business(db, index, stream, record)
            kinds[str(record.kind)] += 1
            counts["physical_records"] += 1
            for warning in filter(None, row["source_warning"].split(";")):
                warnings[warning] += 1
            if record.offset in pending:
                pending.remove(record.offset)
                counts["deleted_records"] += 1
            else:
                counts["entries"] += 1
            if progress and counts["physical_records"] % 100000 == 0:
                progress(completed + record.offset, total_bytes, entries + counts["entries"])
        if pending:
            raise FormatError("Business deletion does not target a record boundary")
        completed += len(stream.data)
        entries += counts["entries"]
        reports[stream.name] = dict(
            counts=dict(counts), source_kinds=dict(kinds), source_warnings=dict(warnings)
        )
    return reports
