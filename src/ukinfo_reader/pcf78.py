"""Independent decoder for the UK-Info Disk SE supplied with PC Format 78."""

import mmap
import re
import struct
from contextlib import ExitStack
from functools import lru_cache
from pathlib import Path

from .codecs import FormatError
from .ctree import LeafIndex
from .editions import identify_edition

# Width, source ID base, offset of text inside a fixed record.
DICTIONARIES = {
    "place": ("d1.dat", 44, 1, 4),
    "surname": ("d2.dat", 19, 0, 0),
    "firstName": ("d3.dat", 16, 0, 0),
    "houseno": ("d4.dat", 10, 1, 0),
    "postcode": ("d6.dat", 12, 0, 3),
    "street": ("d7.dat", 32, 0, 0),
}
POSTCODE = re.compile(r"(?:[A-Z]{1,2}[0-9][0-9A-Z]?|GIR)[0-9][A-Z]{2}\Z")
INVALID_SURNAME = bytes.fromhex("002c22")
# One index key on the verified source points into an unrelated record. Never repair by guessing.
BROKEN_INDEX_POINTER = 150994689
# The place-navigation table labels this range as 19899, but its records decode to 19898.
AMBIGUOUS_PLACE = 19898


def shifted(data, base=128):
    value = 0
    for byte in data:
        value = value * base + byte - 48
    return value


def text(data):
    try:
        return data.split(b"\0", 1)[0].decode("cp1252")
    except UnicodeError as exc:
        raise FormatError("Invalid Windows-1252 dictionary text") from exc


class FixedDictionary:
    def __init__(self, data, width, id_base, text_offset):
        if len(data) < 128 or struct.unpack_from("<H", data, 44)[0] != width:
            raise FormatError("Unexpected fixed dictionary record size")
        header = struct.unpack_from("<H", data, 12)[0]
        self.start = ((header + 32 + width - 1) // width) * width
        self.count = struct.unpack_from("<I", data, 20)[0]
        self.data, self.width, self.id_base, self.text_offset = data, width, id_base, text_offset
        end = self.start + self.count * width
        if end > len(data) or struct.unpack_from("<I", data, 8)[0] != end - 1:
            raise FormatError("Invalid fixed dictionary bounds")

    def raw(self, index):
        index -= self.id_base
        if not 0 <= index < self.count:
            raise FormatError("Fixed dictionary reference out of range")
        pos = self.start + index * self.width
        return self.data[pos : pos + self.width]

    def word(self, index):
        data = self.raw(index)
        if self.text_offset == 3:
            return text(data[3:10])
        return text(data[4:34] if self.text_offset == 4 else data[self.text_offset :])


class PCF78Database:
    """Read indexed directory entries; source offsets identify records, not households."""

    format = "pcf78"
    fields = [
        dict(name=name)
        for name in (
            "record_offset",
            "entry_id",
            "surname",
            "surname_code",
            "firstName",
            "place_id",
            "place",
            "place_type",
            "locality",
            "town",
            "county",
            "postcode",
            "postcode_raw",
            "street",
            "houseno",
            "building",
            "phone_area_code",
            "phone_subscriber",
            "phone",
            "source_warning",
        )
    ]

    def __init__(self, directory):
        self._resources = ExitStack()
        self._cached_word = lru_cache(maxsize=8192)(self._word)
        try:
            directory = Path(directory)
            self.edition = identify_edition(directory)
            mapped = {}
            for name, _, _ in self.edition.files:
                stream = self._resources.enter_context((directory / name).open("rb"))
                mapped[name] = self._resources.enter_context(
                    mmap.mmap(stream.fileno(), 0, access=mmap.ACCESS_READ)
                )
            self.dat = mapped["Ukdemo.dat"]
            if self.dat[:12] != b"*** R.A. ***":
                raise FormatError("Missing PC Format 78 data signature")
            self.index = LeafIndex(mapped["Uk1.idx"], 9)
            self.buildings = LeafIndex(mapped["d5.idx"], 34)
            self.dictionaries = {
                field: FixedDictionary(mapped[name], width, base, offset)
                for field, (name, width, base, offset) in DICTIONARIES.items()
            }
        except (IndexError, struct.error, UnicodeError) as exc:
            self.close()
            raise FormatError("Truncated or invalid PC Format 78 database") from exc
        except BaseException:
            self.close()
            raise

    def _word(self, field, index):
        if field == "building":
            return text(self.buildings.ordinal(index - 1)) if index else ""
        return self.dictionaries[field].word(index)

    def _check(self, field, index):
        dictionary = self.dictionaries[field]
        if not dictionary.id_base <= index < dictionary.count + dictionary.id_base:
            raise FormatError(f"{field} reference out of range")

    def decode_record(self, offset):
        """Decode a physical record, including records absent from the live index.

        Returns source references; use records/search for indexed, joined entries.
        """
        if not 12 <= offset <= len(self.dat) - 11:
            raise FormatError("Main record offset out of range")
        b = self.dat[offset : offset + 11]
        length = b[7] >> 3
        if 0 < length < 11:
            length += 32
        flags = b[8] >> 5
        fixed = 11 + (5 if flags & 4 else 0) + (2 if flags & 2 else 0) + (3 if flags & 1 else 0)
        if length < fixed or (length - fixed) % 3 or offset + length > len(self.dat):
            raise FormatError(f"Invalid main record length at {offset}")
        place = shifted(b[:2], 155)
        place = 0 if place < 0 else place + (place < 14550)
        surname = shifted(b[2:5])
        if not 0 <= surname < self.dictionaries["surname"].count and b[2:5] != INVALID_SURNAME:
            raise FormatError(f"Unknown invalid surname code at {offset}")
        first = int.from_bytes(b[5:8], "little") & 0x7FFFF
        postcode = int.from_bytes(b[8:11], "big") & 0x1FFFFF
        self._check("place", place)
        self._check("postcode", postcode)
        cursor = offset + 11
        area = subscriber = house = building = 0
        if flags & 4:
            area = int.from_bytes(self.dat[cursor : cursor + 2], "little")
            subscriber = int.from_bytes(self.dat[cursor + 2 : cursor + 5], "little")
            cursor += 5
        if flags & 2:
            house = max(0, shifted(self.dat[cursor : cursor + 2])) + 1
            self._check("houseno", house)
            cursor += 2
        if flags & 1:
            building = int.from_bytes(self.dat[cursor : cursor + 3], "little")
            if building > self.buildings.count:
                raise FormatError("Building-name reference out of range")
            cursor += 3
        names = [first] + [
            int.from_bytes(self.dat[p : p + 3], "little") for p in range(cursor, offset + length, 3)
        ]
        for name in names:
            self._check("firstName", name)
        return dict(
            length=length,
            flags=flags,
            place=place,
            surname=surname,
            surname_code=b[2:5],
            postcode=postcode,
            names=names,
            houseno=house,
            building=building,
            area=area,
            subscriber=subscriber,
        )

    def _location(self, place):
        raw = self.dictionaries["place"].raw(place)
        county_id, town_id = struct.unpack_from("<HH", raw)
        kind = text(raw[34:36])
        name = self._cached_word("place", place)
        locality = town = county = ""
        if kind.startswith("V"):
            locality = name
            parent = self.dictionaries["place"].raw(town_id)
            town = self._cached_word("place", town_id)
            county_id = struct.unpack_from("<H", parent)[0] if parent[35:36] == b"C" else 0
        elif kind.startswith("T"):
            town = name
            if kind != "TC":
                county_id = 0
        elif kind == "C":
            county = name
            county_id = 0
        else:
            raise FormatError("Unknown place type")
        if county_id:
            county = self._cached_word("place", county_id)
        return dict(
            place_id=place, place=name, place_type=kind, locality=locality, town=town, county=county
        )

    def _joined(self, offset, record):
        pc = self.dictionaries["postcode"].raw(record["postcode"])
        street = shifted(pc[:3])
        postcode = text(pc[3:10]).replace(" ", "")
        valid_postcode = bool(POSTCODE.fullmatch(postcode))
        warnings = []
        if record["surname"] < 0:
            warnings.append("invalid_surname_reference")
        if not valid_postcode:
            warnings.append("nonstandard_postcode")
        if record["place"] == AMBIGUOUS_PLACE:
            warnings.append("place_navigation_mismatch")
        common = dict(
            record_offset=offset,
            surname=self._cached_word("surname", record["surname"])
            if record["surname"] >= 0
            else None,
            surname_code=record["surname_code"].hex(),
            **self._location(record["place"]),
            postcode=postcode[:-3] + " " + postcode[-3:] if valid_postcode else postcode,
            postcode_raw=text(pc[3:10]),
            street=self._cached_word("street", street),
            houseno=self._cached_word("houseno", record["houseno"]) if record["houseno"] else "",
            building=self._cached_word("building", record["building"]),
            phone_area_code=record["area"],
            phone_subscriber=record["subscriber"],
            phone=f"0{record['area']} {record['subscriber']}" if record["area"] else "",
            source_warning=";".join(warnings),
        )
        for ordinal, first in enumerate(record["names"]):
            yield dict(
                common,
                entry_id=f"{offset}:{ordinal}",
                firstName=self._cached_word("firstName", first),
            )

    def records(self):
        """Yield entries in source index order, excluding its one known broken pointer."""
        yield from self.search()

    def search(self, surname=None, first_name=None, postcode=None, sector=None):
        """Exact case-insensitive filters; surname uses the source leaf index."""
        prefixes = [None]
        if surname is not None:
            dictionary = self.dictionaries["surname"]
            ids = [
                i
                for i in range(dictionary.count)
                if dictionary.word(i).casefold() == surname.casefold()
            ]
            prefixes = [bytes([48 + i // 16384, 48 + (i // 128) % 128, 48 + i % 128]) for i in ids]
        target_pc = postcode.replace(" ", "").casefold() if postcode else None
        target_sector = sector.replace(" ", "").casefold() if sector else None
        for prefix in prefixes:
            for key, offset in self.index.keys(prefix):
                if self.dat[offset + 2 : offset + 5] + self.dat[offset : offset + 2] != key:
                    if offset == BROKEN_INDEX_POINTER and key == bytes.fromhex("33557d7d44"):
                        continue
                    raise FormatError("Index key disagrees with main record")
                record = self.decode_record(offset)
                if target_pc or target_sector:
                    pc = (
                        self._cached_word("postcode", record["postcode"])
                        .replace(" ", "")
                        .casefold()
                    )
                    if target_pc and pc != target_pc:
                        continue
                    if target_sector and pc[:-2] != target_sector:
                        continue
                if first_name and not any(
                    self._cached_word("firstName", i).casefold() == first_name.casefold()
                    for i in record["names"]
                ):
                    continue
                for row in self._joined(offset, record):
                    pc = row["postcode"].replace(" ", "").casefold()
                    if target_pc and pc != target_pc:
                        continue
                    if target_sector and pc[:-2] != target_sector:
                        continue
                    if first_name and row["firstName"].casefold() != first_name.casefold():
                        continue
                    yield row

    def close(self):
        self._cached_word.cache_clear()
        self._resources.close()

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        self.close()
