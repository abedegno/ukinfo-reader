"""Explicit record layout for the supplied March 2008 UK-Info v14 source.

Telephone prefixes are extracted from the user's original decoder as factual
source metadata; neither the Windows decoder nor its code is executed here.
"""

from dataclasses import dataclass
from functools import lru_cache

from .codecs import FormatError
from .modern import ModernFixedTable, PhysicalBusiness, PhysicalResidence
from .modern_business import BusinessStream
from .modern_reader import ModernDatabase, ResidenceStream, location, word


def residential_length_v14(data, offset=0):
    """Check boundaries without allocating a decoded record during filtering."""
    if offset < 0 or offset + 10 > len(data):
        raise FormatError("Truncated v14 residential record")
    flags, pc_flags = data[offset], data[offset + 7]
    count, kind = flags & 7, (flags >> 3) & 7
    if not 1 <= count <= 5:
        raise FormatError("Invalid v14 residential name count")
    length = (
        6
        + 4 * count
        + (4 if flags & 128 else 0)
        + (2 if flags & 64 else 0)
        + (3 if kind in (2, 5) else 0)
        + (4 if 1 <= kind <= 6 else 0)
        + (1 if pc_flags & 128 else 0)
    )
    if offset + length > len(data):
        raise FormatError("Truncated v14 residential record")
    return length


def decode_residence_v14(data, offset=0):
    residential_length_v14(data, offset)
    flags, pc_flags = data[offset], data[offset + 7]
    count, kind = flags & 7, (flags >> 3) & 7
    cursor = offset + 10

    def take(width, present=True):
        nonlocal cursor
        if not present:
            return None
        if cursor + width > len(data):
            raise FormatError("Truncated v14 residential record")
        value = int.from_bytes(data[cursor : cursor + width], "little")
        cursor += width
        return value

    names = [int.from_bytes(data[offset + 1 : offset + 5], "little") & 0x7FFFFFF]
    names.extend(take(4) for _ in range(count - 1))
    building = take(4, flags & 128)
    flat = take(2, flags & 64)
    contact = take(3, kind in (2, 5))
    phone = take(4, 1 <= kind <= 6)
    precision = take(1, pc_flags & 128)
    return PhysicalResidence(
        offset,
        cursor - offset,
        kind,
        flags,
        tuple(names),
        int.from_bytes(data[offset + 5 : offset + 8], "little") & 0x3FFFFF,
        pc_flags & 0xC0,
        int.from_bytes(data[offset + 8 : offset + 10], "little"),
        building,
        flat,
        contact,
        phone,
        None,
        precision,
    )


@dataclass(frozen=True)
class PhysicalBusinessV14(PhysicalBusiness):
    telephone_extensions: int


def decode_business_v14(data, offset=0):
    if offset < 0 or offset + 10 > len(data):
        raise FormatError("Truncated v14 business record")
    flags, control = data[offset : offset + 2]
    count = (control & 15) + 1
    cursor = offset + 7

    def take(width, present=True):
        nonlocal cursor
        if not present:
            return None
        if cursor + width > len(data):
            raise FormatError("Truncated v14 business record")
        value = int.from_bytes(data[cursor : cursor + width], "little")
        cursor += width
        return value

    codes = tuple(take(3) for _ in range(count))
    lob, sic = take(4, flags & 1), take(2, flags & 2)
    building, flat = take(4, flags & 4), take(2, flags & 8)
    phone, fax = take(4, flags & 16), take(4, flags & 64)
    precision = take(1, control & 128) or 0
    return PhysicalBusinessV14(
        offset,
        cursor - offset,
        flags,
        (control >> 4) & 3,
        codes,
        int.from_bytes(data[offset + 2 : offset + 5], "little") & 0x1FFFFF,
        int.from_bytes(data[offset + 5 : offset + 7], "little")
        | (0x10000 if data[offset + 4] & 128 else 0),
        lob,
        sic,
        building,
        flat,
        phone,
        0,
        0,
        fax,
        precision,
        (data[offset + 4] >> 5) & 3,
    )


def telephone(code, prefixes, extension=False):
    """Return display, source prefix, six-digit suffix and a source warning.

    Prefixes can include subscriber digits, so they are not asserted to be area
    codes. The original formatter takes the final six decimal suffix digits.
    """
    if not code:
        return "", "", "", ""
    index = (code >> 20) - 1 + (4096 if extension else 0)
    if not 0 <= index < len(prefixes):
        return "", "", "", "telephone_prefix_reference_out_of_range"
    prefix = prefixes[index]
    suffix = f"{code & 0xFFFFF:06d}"[-6:]
    if not prefix.isascii() or not prefix.isdigit():
        return "", prefix, suffix, "invalid_phone_prefix"
    return prefix + suffix, prefix, suffix, ""


class ResidenceStreamV14(ResidenceStream):
    decode = staticmethod(decode_residence_v14)
    postcode_offset = 5
    postcode_mask = 0x3FFFFF

    length = staticmethod(residential_length_v14)


class BusinessStreamV14(BusinessStream):
    decode = staticmethod(decode_business_v14)

    def __init__(self, mapped):
        self.name = "base"
        self.data, self.words = mapped["bus.dat"], mapped["dd.dat"]
        self.industry, self.sic = mapped["lobdesc.dat"], ModernFixedTable(mapped["dbsic.dat"])
        self.word = lru_cache(maxsize=32768)(lambda offset: self.text(self.words, offset))
        self.industry_text = lru_cache(maxsize=8192)(self._industry_text)
        self.deleted = set()

    def _industry_text(self, reference):
        if not reference:
            return "", reference, ""
        if not 0 <= reference < len(self.industry) or self.industry[reference - 1] != 0:
            return None, None, "invalid_industry_reference"
        return self.text(self.industry, reference), reference, ""


class V14Database(ModernDatabase):
    residence_stream = ResidenceStreamV14
    fields = ModernDatabase.fields + [
        dict(name=n)
        for n in (
            "phone_prefix",
            "phone_code_raw",
            "fax_prefix",
            "fax_code_raw",
            "county_source_id",
            "postcode_flags",
            "source_co1",
            "resolved_county_source_id",
            "town_source_id",
        )
    ]

    def __init__(self, directory):
        super().__init__(directory)
        try:
            raw = self.mapped["telephone-prefixes.dat"][:]
            if not raw.endswith(b"\0"):
                raise FormatError("Unterminated v14 telephone prefix dictionary")
            try:
                self.telephone_prefixes = tuple(raw[:-1].decode("ascii").split("\0"))
            except UnicodeError as exc:
                raise FormatError("Non-ASCII v14 telephone prefix dictionary") from exc
            if not self.telephone_prefixes or any(
                not 1 <= len(p) <= 16 for p in self.telephone_prefixes
            ):
                raise FormatError("Invalid v14 telephone prefix dictionary")
        except BaseException:
            self._resources.close()
            raise

    @staticmethod
    def variant(edition_id):
        return 0x3FFFFF, False

    @staticmethod
    def open_business_streams(mapped):
        return [BusinessStreamV14(mapped)]

    def _telephone_fields(self, code, fax=False, extension=False):
        display, prefix, subscriber, warning = telephone(code, self.telephone_prefixes, extension)
        field = "fax" if fax else "phone"
        return {
            field: display,
            field + "_prefix": prefix,
            field + "_subscriber": subscriber,
            field + "_area_code": 0,
            field + "_code_raw": code or 0,
        }, warning

    def _location(self, source_id):
        if not source_id:
            return dict(
                place_id=0,
                place="",
                place_type="",
                locality="",
                town="",
                county="",
                county_source_id=0,
                resolved_county_source_id=0,
                town_source_id=0,
                place_warning="",
            )
        raw = self.places.raw(source_id)
        name, kind = word(raw[:30]), chr(raw[30])
        county_id = int.from_bytes(raw[36:40], "little")
        raw_county_id = county_id
        town_id = int.from_bytes(raw[40:44], "little")
        locality = town = county = ""
        warnings = []
        if b"\0" not in raw[:30]:
            warnings.append("unterminated_place_text")
        if kind == "C":
            county = name
        elif kind == "T":
            # The exact v14 formatter identifies these Orkney/Shetland entries
            # as localities although their stored type byte is T.
            island_locality = any(
                lo <= source_id <= hi
                for lo, hi in (
                    (0x4D83, 0x4D8E),
                    (0x4D96, 0x4D9D),
                    (0x4DAB, 0x4DB0),
                    (0x5541, 0x5556),
                    (0x555B, 0x5577),
                )
            )
            if island_locality:
                locality = name
            else:
                town = name
        elif kind in ("V", "v"):
            locality = name
            if town_id:
                parent = self.places.raw(town_id)
                if chr(parent[30]) != "T":
                    raise FormatError("Invalid v14 locality town parent")
                town = word(parent[:30])
                county_id = int.from_bytes(parent[36:40], "little")
                if b"\0" not in parent[:30]:
                    warnings.append("unterminated_place_text")
        else:
            raise FormatError("Unknown v14 place type")
        if kind != "C" and county_id:
            parent = self.places.raw(county_id)
            if chr(parent[30]) != "C":
                raise FormatError("Invalid v14 locality county parent")
            county = word(parent[:30])
        # The original v14 formatter retains stale ID/name patches from older
        # editions. Preserve source relationships and make the difference visible.
        legacy = location(self.places, source_id)
        if kind == "T" and locality:
            legacy.update(locality=name, town="")
        if (locality, town, county) != tuple(legacy[f] for f in ("locality", "town", "county")):
            warnings.append("legacy_locality_override_ignored")
        return dict(
            place_id=source_id,
            place=name,
            place_type=kind,
            locality=locality,
            town=town,
            county=county,
            county_source_id=raw_county_id,
            resolved_county_source_id=county_id,
            town_source_id=town_id,
            place_warning=";".join(sorted(set(warnings))),
        )

    def _joined(self, stream_index, offset, surname_id, record):
        for row in super()._joined(stream_index, offset, surname_id, record):
            row["postcode_flags"] = record.postcode_flags
            row["source_co1"] = bool(record.postcode_flags & 0x40)
            yield row

    def residential_phone(self, record):
        return self._telephone_fields(record.optional_kind16 if record else None)

    def validate_residential_record(self, record):
        if record.kind in (3, 4, 5):
            _, warning = self._telephone_fields(record.optional_kind16)
            return (warning,) if warning else ()
        return ()

    def business_phones(self, record):
        result, warnings = {}, []
        for fax, code, extension in (
            (False, record.subscriber, record.telephone_extensions & 1),
            (True, record.fax_subscriber, record.telephone_extensions & 2),
        ):
            values, warning = self._telephone_fields(code, fax, extension)
            result.update(values)
            warnings.append(warning)
        return result, warnings
