"""The distinct 2003 SE layout: 17-bit house IDs and no precision byte.

Source profiles are registered only after whole-source validation. The 2003 media
has no line-of-business description dictionary; its references remain explicit.
"""

import re
from functools import lru_cache

from .modern import ModernFixedTable, _decode_business, _decode_residence, _residential_length
from .modern_business import BusinessStream
from .modern_reader import ModernDatabase, ResidenceStream, word


def residential_length_2003(data, offset=0):
    return _residential_length(data, offset, precision=False)


def decode_residence_2003(data, offset=0):
    return _decode_residence(
        data, offset, precision=False, house_extension=True, building_extension=False
    )


def decode_business_2003(data, offset=0):
    return _decode_business(data, offset, precision=False, building_extension=False)


class ResidenceStream2003(ResidenceStream):
    postcode_table = "d6"
    decode = staticmethod(decode_residence_2003)
    length = staticmethod(residential_length_2003)

    def __init__(self, mapped, suffix):
        super().__init__(mapped, suffix)
        self.prefixes = ModernFixedTable(mapped["d6pa.dat"])


class BusinessStream2003(BusinessStream):
    decode = staticmethod(decode_business_2003)
    allowed_kinds = (0,)
    industry_available = False

    def __init__(self, mapped):
        self.name = "base"
        self.data = mapped["dbbus.dat"]
        self.words = mapped["dd.dat"]
        self.sic = ModernFixedTable(mapped["dbsic.dat"])
        self.word = lru_cache(maxsize=32768)(lambda offset: self.text(self.words, offset))
        self.deleted = set()

    def industry_text(self, reference):
        if not reference:
            return "", reference, ""
        return None, None, "industry_dictionary_missing"


class Modern2003Database(ModernDatabase):
    residence_stream = ResidenceStream2003
    allow_unknown_names = True
    fields = ModernDatabase.fields + [
        dict(name=name) for name in ("postcode_x_raw", "postcode_y_raw", "postcode_flags")
    ]

    @staticmethod
    def variant(edition_id):
        return 0x1FFFFF, True

    @staticmethod
    def open_business_streams(mapped):
        return [BusinessStream2003(mapped)]

    def _joined(self, stream_index, offset, surname_id, record):
        for row in super()._joined(stream_index, offset, surname_id, record):
            row["postcode_flags"] = record.postcode_flags
            yield row

    def _word(self, stream_index, field, source_id):
        if field == "postcode":
            return self._cached_address(stream_index, source_id)["postcode_raw"]
        return super()._word(stream_index, field, source_id)

    def _address(self, stream_index, postcode_id):
        if not postcode_id:
            return dict(
                postcode="",
                postcode_raw="",
                street="",
                postcode_x_raw=None,
                postcode_y_raw=None,
                **self._cached_location(0),
            )
        stream = self.streams[stream_index]
        raw = stream.tables["postcode"].raw(postcode_id)
        prefix_id = int.from_bytes(raw[:3], "little")
        prefix = stream.prefixes.raw(prefix_id)
        postcode = word(prefix[:5]) + word(raw[3:5])
        place_id = int.from_bytes(prefix[9:11], "little")
        street_offset = int.from_bytes(raw[11:14], "little")
        if place_id == 0xFFFF:
            place = dict(self._cached_location(0))
            place["place_id"] = place_id
            place["place_warning"] = "missing_source_locality"
        else:
            place = self._cached_location(place_id)
        full = re.fullmatch(r"(?:GIR0AA|[A-Z]{1,2}[0-9][A-Z0-9]?[0-9][A-Z]{2})", postcode)
        result = dict(
            postcode=postcode[:-3] + " " + postcode[-3:] if full else postcode,
            postcode_raw=postcode,
            postcode_x_raw=int.from_bytes(raw[5:8], "little"),
            postcode_y_raw=int.from_bytes(raw[8:11], "little"),
            street=self._string_at(self.streets, street_offset) if street_offset else "",
            **place,
        )
        if not full:
            result["place_warning"] = ";".join(
                filter(None, (result["place_warning"], "partial_postcode_source"))
            )
        return result
