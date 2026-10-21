"""Residential and business entries from the verified 2004 and 2005 SE sources."""

import mmap
import struct
from array import array
from bisect import bisect_right
from contextlib import ExitStack
from functools import lru_cache
from pathlib import Path

from .codecs import FormatError
from .editions import identify_edition
from .modern import ModernFixedTable, ModernLeafIndex, decode_residence, residential_length


def word(data):
    """Decode only the declared text field, never adjacent record metadata."""
    return data.split(b"\0", 1)[0].decode("cp1252", errors="replace")


def location(table, source_id):
    """Resolve the stored locality, its post town and that town's county."""
    if not source_id:
        return dict(
            place_id=0, place="", place_type="", locality="", town="", county="", place_warning=""
        )
    raw = table.raw(source_id)
    name, kind = word(raw[:30]), chr(raw[30])
    county_id, town_id = struct.unpack_from("<II", raw, 36)
    locality = town = county = ""
    warnings = []
    if b"\0" not in raw[:30]:
        warnings.append("unterminated_place_text")
    if kind == "C":
        county = name
    elif kind == "T":
        town = name
        if county_id:
            county = word(table.raw(county_id)[:30])
            county = {0x25B8: "North Humberside", 0x6617: "Essex", 0x30CB: "South Yorkshire"}.get(
                source_id, county
            )
    elif kind in ("V", "v"):
        locality = name
        if name == "London Apprentice":
            locality, town, county_id = "", "London", 0
        elif name == "Bromley Cross":
            town, county_id = "Bolton", 0
        else:
            if town_id:
                parent = table.raw(town_id)
                town = word(parent[:30])
                county_id = struct.unpack_from("<I", parent, 36)[0]
                if b"\0" not in parent[:30]:
                    warnings.append("unterminated_place_text")
            if county_id:
                county = word(table.raw(county_id)[:30])
            if 0x25B9 <= source_id <= 0x25CD:
                county = "North Humberside"
            elif 0x30CC <= source_id <= 0x30E6:
                county = "South Yorkshire"
            elif 0x6618 <= source_id <= 0x661A:
                county = "Essex"
    else:
        raise FormatError("Unknown modern place type")
    return dict(
        place_id=source_id,
        place=name,
        place_type=kind,
        locality=locality,
        town=town,
        county=county,
        place_warning=";".join(sorted(set(warnings))),
    )


def subscriber_text(value, legacy=False):
    """Restore leading subscriber zeroes encoded above the ordinary numeric range."""
    if value in (None, 0):
        return ""
    if legacy:
        value = abs(value)
        return "0" + str(value - 10000000) if 10000000 < value < 11000000 else str(value)
    if value == -1:
        return "0000000"
    if value > 10000000:
        zeroes = (value - 9000001) // 1000000
        return "0" * zeroes + str(value - zeroes * 1000000 - 9000000)
    return str(value)


def entry_codes(record, deleted=False):
    """Preserve electoral and telephone entries as distinct source occurrences.

    Deletions suppress telephone entries only. A combined kind 4/5 record retains
    its electoral names even when its associated telephone entry is deleted.
    """
    if record.kind not in (0, 3, 4, 5):
        raise FormatError("Unverified residential source kind")
    if record.kind != 3:
        for ordinal, code in enumerate(record.name_codes):
            yield "electoral", ordinal, code
    elif not deleted:
        for ordinal, code in enumerate(record.name_codes):
            yield "telephone", ordinal, code
    if record.kind in (4, 5) and not deleted:
        code = record.optional_kind25 if record.kind == 5 else record.name_codes[0]
        yield "telephone", len(record.name_codes), code


class ResidenceStream:
    postcode_table = "d6new"
    decode = staticmethod(decode_residence)
    length = staticmethod(residential_length)

    def __init__(self, mapped, suffix):
        self.name = "update" if suffix else "base"
        self.data = mapped["ukupd.dat" if suffix else "uk.dat"]
        self.tables = {
            field: ModernFixedTable(mapped[name + suffix + ".dat"])
            for field, name in [
                ("surname", "d2"),
                ("name", "d3"),
                ("house", "d4"),
                ("postcode", self.postcode_table),
                ("subbuilding", "d8"),
            ]
        }
        self.buildings = mapped["d5" + suffix + ".dat"]
        self.starts, self.surnames = array("I"), array("I")
        for key, source_id in ModernLeafIndex(mapped["ukspos" + suffix + ".idx"]).keys():
            start = int.from_bytes(key, "big")
            if (
                len(key) != 4
                or start >= len(self.data)
                or not 1 <= source_id <= self.tables["surname"].count
                or (self.starts and start <= self.starts[-1])
            ):
                raise FormatError("Invalid modern surname range")
            self.starts.append(start)
            self.surnames.append(source_id)
        if not self.starts:
            raise FormatError("Missing modern surname ranges")
        self.deleted = set()
        self.fix_offsets, self.fix_values = array("I"), array("I")
        if not suffix and "hfix.idx" in mapped:
            for key, value in ModernLeafIndex(mapped["hfix.idx"]).keys():
                offset = int.from_bytes(key, "big")
                if len(key) != 4 or (self.fix_offsets and offset <= self.fix_offsets[-1]):
                    raise FormatError("Invalid building correction index")
                self.fix_offsets.append(offset)
                self.fix_values.append(value)
        if not suffix and "delr.idx" in mapped:
            for key, _ in ModernLeafIndex(mapped["delr.idx"]).keys():
                offset = int.from_bytes(key, "little")
                if len(key) != 4 or offset >= len(self.data) or offset in self.deleted:
                    raise FormatError("Invalid telephone deletion index")
                self.deleted.add(offset)

    def building_reference(self, offset, record):
        reference = record.optional_flag80
        if reference and self.fix_offsets:
            i = bisect_right(self.fix_offsets, offset) - 1
            if i >= 0 and self.fix_offsets[i] == offset:
                return self.fix_values[i]
        return reference


class ModernDatabase:
    """Read joined residential and available business entries."""

    format = "modern"
    residence_stream = ResidenceStream
    allow_unknown_names = False
    fields = [
        dict(name=name)
        for name in (
            "entry_id",
            "source_stream",
            "record_offset",
            "entry_type",
            "kind",
            "surname",
            "firstName",
            "surname_id",
            "name_code",
            "name_flags",
            "postcode",
            "postcode_raw",
            "postcode_precision",
            "houseno",
            "building",
            "building_raw",
            "building_reference",
            "building_reference_raw",
            "subbuilding",
            "street",
            "place_id",
            "place",
            "place_type",
            "locality",
            "town",
            "county",
            "phone_area_code",
            "phone_subscriber",
            "phone",
            "source_warning",
            "organisation",
            "business_word_codes",
            "industry",
            "industry_reference",
            "industry_reference_raw",
            "sic_code",
            "sic_id",
            "fax_area_code",
            "fax_subscriber",
            "fax",
        )
    ]

    def __init__(self, directory):
        self._resources = ExitStack()
        self._cached_word = lru_cache(maxsize=16384)(self._word)
        self._cached_address = lru_cache(maxsize=8192)(self._address)
        self._cached_location = lru_cache(maxsize=32768)(self._location)
        try:
            self.edition = identify_edition(directory)
            self.name_mask, self.legacy_phone = self.variant(self.edition.id)
            self.mapped = {}
            for name, _, _ in self.edition.files:
                stream = self._resources.enter_context((Path(directory) / name).open("rb"))
                self.mapped[name] = self._resources.enter_context(
                    mmap.mmap(stream.fileno(), 0, access=mmap.ACCESS_READ)
                )
            self.places = ModernFixedTable(self.mapped["d1.dat"])
            self.streets = self.mapped["d7.dat"]
            self.streams = [self.residence_stream(self.mapped, "")]
            if "ukupd.dat" in self.mapped:
                self.streams.append(self.residence_stream(self.mapped, "upd"))
            self.business_streams = self.open_business_streams(self.mapped)
        except BaseException:
            self.close()
            raise

    @staticmethod
    def variant(edition_id):
        return (0x3FFFFF, True) if edition_id.startswith("se2004") else (0x1FFFFF, False)

    @staticmethod
    def open_business_streams(mapped):
        from .modern_business import BusinessStream

        return (
            [BusinessStream(mapped, ""), BusinessStream(mapped, "upd")]
            if "bus.dat" in mapped
            else []
        )

    def _word(self, stream_index, field, source_id):
        if not source_id:
            return ""
        table = self.streams[stream_index].tables[field]
        raw = table.raw(source_id)
        if field == "surname":
            raw = raw[:-4]
        elif field == "name":
            raw = raw[:-1] if table.width == 21 else raw[:-4]
        elif field == "postcode":
            raw = raw[:7]
        return word(raw)

    def _location(self, source_id):
        return location(self.places, source_id)

    @staticmethod
    def _string_at(data, offset):
        if not 0 <= offset < len(data):
            raise FormatError("Modern string reference out of range")
        end = data.find(b"\0", offset)
        if end < 0:
            raise FormatError("Unterminated modern dictionary string")
        return word(data[offset:end])

    def _address(self, stream_index, postcode_id):
        if not postcode_id:
            return dict(postcode="", postcode_raw="", street="", **self._cached_location(0))
        raw = self.streams[stream_index].tables["postcode"].raw(postcode_id)
        pc = word(raw[:7])
        place = int.from_bytes(raw[7:9], "little")
        street = int.from_bytes(raw[9:12], "little")
        return dict(
            postcode=pc[:-3] + " " + pc[-3:] if len(pc) > 3 else pc,
            postcode_raw=pc,
            street=self._string_at(self.streets, street) if street else "",
            **self._cached_location(place),
        )

    def _joined(self, stream_index, offset, surname_id, record):
        stream = self.streams[stream_index]
        address = dict(self._cached_address(stream_index, record.postcode_id))
        warnings = [address.pop("place_warning")]
        building_id = stream.building_reference(offset, record)
        building_raw = building = ""
        if building_id not in (None, 0, 0xFFFFFF):
            if not 0 <= building_id < len(stream.buildings):
                building = building_raw = None
                warnings.append("building_reference_out_of_range")
            else:
                building_raw = self._string_at(stream.buildings, building_id)
                building = building_raw
            if building_raw is not None and stream.buildings[building_id - 1] != 0:
                building = None
                warnings.append("building_reference_inside_string")
        precision = record.optional_pc80 or 0
        if precision not in (0, 10):
            warnings.append("partial_postcode_source")
            # Apply the source's precision rule to the full stored code. The SE
            # application's unrelated initial/three-character display cap is omitted.
            pc = address["postcode_raw"]
            remove = precision - 10 if precision > 10 else 10 - precision
            pc = pc[: max(0, len(pc) - remove)]
            address["postcode"] = pc
            address["street"] = ""
            if precision < 10:
                for field in ("place", "locality", "town", "county"):
                    address[field] = ""
        common = dict(
            source_stream=stream.name,
            record_offset=offset,
            kind=record.kind,
            surname=self._cached_word(stream_index, "surname", surname_id),
            surname_id=surname_id,
            **address,
            postcode_precision=precision,
            houseno=self._cached_word(stream_index, "house", record.house_id),
            building=building,
            building_raw=building_raw,
            building_reference=building_id,
            building_reference_raw=record.optional_flag80,
            subbuilding=self._cached_word(stream_index, "subbuilding", record.optional_flag40 or 0),
        )
        for entry_type, ordinal, code in entry_codes(record, offset in stream.deleted):
            row_warnings = list(warnings)
            name_id = code & self.name_mask
            if name_id > stream.tables["name"].count:
                if code != 0xFFFFFF and not self.allow_unknown_names:
                    raise FormatError("Invalid modern telephone name reference")
                first = ""
                row_warnings.append(
                    "unknown_telephone_name" if code == 0xFFFFFF else "invalid_name_reference"
                )
            else:
                first = self._cached_word(stream_index, "name", name_id)
            if any(
                "\ufffd" in value for value in [first, *common.values()] if isinstance(value, str)
            ):
                row_warnings.append("invalid_dictionary_text")
            area = (record.optional_pc20 or 0) if entry_type == "telephone" else 0
            subscriber = (
                subscriber_text(record.optional_kind16, legacy=self.legacy_phone)
                if entry_type == "telephone"
                else ""
            )
            yield dict(
                dict.fromkeys((f["name"] for f in self.fields), ""),
                **common,
                entry_id=f"{stream.name}:{offset}:{ordinal}",
                entry_type=entry_type,
                firstName=first,
                name_code=code,
                name_flags=code & ~self.name_mask,
                phone_area_code=area,
                phone_subscriber=subscriber,
                phone=f"0{area} {subscriber}" if area and subscriber else "",
                source_warning=";".join(sorted(set(filter(None, row_warnings)))),
            )

    def records(self):
        yield from self.search()

    def search(
        self,
        surname=None,
        first_name=None,
        postcode=None,
        sector=None,
        organisation=None,
        entry_type=None,
    ):
        """Search source occurrences; organisation filters select business entries."""
        if entry_type not in (None, "business", "electoral", "telephone"):
            raise ValueError("Unknown entry type")
        if organisation is None and entry_type != "business":
            for row in self._search_residential(surname, first_name, postcode, sector):
                if entry_type is None or row["entry_type"] == entry_type:
                    yield row
        if surname is None and first_name is None and entry_type in (None, "business"):
            yield from self.search_business(organisation, postcode, sector)

    def search_business(self, organisation=None, postcode=None, sector=None):
        """Exact business-name and postcode filters, in base then update order."""
        from .modern_business import joined_business

        for index, stream in enumerate(self.business_streams):
            for record in stream.physical_records():
                if record.offset in stream.deleted:
                    continue
                if (
                    organisation is not None
                    and stream.organisation(record).casefold() != organisation.casefold()
                ):
                    continue
                if postcode or sector:
                    pc = (
                        self._cached_word(index, "postcode", record.postcode_id)
                        .replace(" ", "")
                        .casefold()
                    )
                    if postcode and pc != postcode.replace(" ", "").casefold():
                        continue
                    if sector and pc[:-2] != sector.replace(" ", "").casefold():
                        continue
                row = joined_business(self, index, stream, record)
                visible = row["postcode"].replace(" ", "").casefold()
                if postcode and visible != postcode.replace(" ", "").casefold():
                    continue
                if sector and not visible.startswith(sector.replace(" ", "").casefold()):
                    continue
                yield row

    def _search_residential(self, surname=None, first_name=None, postcode=None, sector=None):
        """Exact case-insensitive filters; surnames restrict the physical ranges scanned."""
        target_pc = postcode.replace(" ", "").casefold() if postcode else None
        target_sector = sector.replace(" ", "").casefold() if sector else None
        for stream_index, stream in enumerate(self.streams):
            ids = None
            if surname is not None:
                table = stream.tables["surname"]
                ids = {
                    i
                    for i in range(1, table.count + 1)
                    if word(table.raw(i)[:-4]).casefold() == surname.casefold()
                }
            for group, (start, surname_id) in enumerate(zip(stream.starts, stream.surnames)):
                if ids is not None and surname_id not in ids:
                    continue
                end = (
                    stream.starts[group + 1] if group + 1 < len(stream.starts) else len(stream.data)
                )
                offset = start
                while offset < end:
                    length = stream.length(stream.data, offset)
                    if offset + length > end:
                        raise FormatError("Surname group ends inside residential record")
                    pc_id = (
                        int.from_bytes(stream.data[offset + 4 : offset + 7], "little") & 0x1FFFFF
                    )
                    if target_pc or target_sector:
                        pc = (
                            self._cached_word(stream_index, "postcode", pc_id)
                            .replace(" ", "")
                            .casefold()
                        )
                        if (target_pc and pc != target_pc) or (
                            target_sector and pc[:-2] != target_sector
                        ):
                            offset += length
                            continue
                    record = stream.decode(stream.data, offset)
                    for row in self._joined(stream_index, offset, surname_id, record):
                        visible_pc = row["postcode"].replace(" ", "").casefold()
                        if target_pc and visible_pc != target_pc:
                            continue
                        if target_sector and (
                            not visible_pc.startswith(target_sector)
                            or len(visible_pc) < len(target_sector)
                        ):
                            continue
                        if (
                            first_name is None
                            or row["firstName"].casefold() == first_name.casefold()
                        ):
                            yield row
                    offset += length

    def close(self):
        self._cached_word.cache_clear()
        self._cached_address.cache_clear()
        self._cached_location.cache_clear()
        self._resources.close()

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        self.close()
