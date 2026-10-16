"""Read the UK-Info 4.0 column database without loading the Windows software."""

from __future__ import annotations

import mmap
import struct
from bisect import bisect_right
from contextlib import ExitStack
from functools import lru_cache
from pathlib import Path

from .codecs import FormatError, dictionary, integers, postcode_bits, varint
from .editions import identify_edition


class Database:
    """Memory-mapped, read-only access to a supported edition.

    Prefer ``with Database(directory) as db`` to close file handles deterministically.
    Use ``db.edition.verify(directory)`` to confirm exact source fingerprints.
    """

    def __init__(self, directory="extracted"):
        self._resources = ExitStack()
        try:
            directory = Path(directory)
            self.edition = identify_edition(directory)
            files = [
                self._resources.enter_context((directory / name).open("rb"))
                for name in ("ukid.idx", "ukid.dat")
            ]
            self.idx, self.dat = [
                self._resources.enter_context(mmap.mmap(f.fileno(), 0, access=mmap.ACCESS_READ))
                for f in files
            ]
            self._read_index()
            self._cached_column = lru_cache(maxsize=256)(self._decode_column)
        except (IndexError, struct.error, UnicodeError) as exc:
            self._resources.close()
            raise FormatError("Truncated or invalid index file") from exc
        except BaseException:
            self._resources.close()
            raise

    def _read_index(self):
        self.fields = []
        pos = 0
        while True:
            field_id = struct.unpack_from("<H", self.idx, pos)[0]
            pos += 2
            if field_id == 65535:
                break
            end = self.idx.find(b"\0", pos)
            if end < 0:
                raise FormatError("Unterminated field name")
            name = self.idx[pos:end].decode("ascii")
            pos = end + 1
            meta = struct.unpack_from("<6H2s4H", self.idx, pos)
            pos += 22
            if field_id != len(self.fields):
                raise FormatError("Nonconsecutive field IDs")
            self.fields.append(
                dict(
                    id=field_id,
                    name=name,
                    type=meta[0],
                    level=meta[1],
                    metadata=list(meta[:6]) + [meta[6].decode()] + list(meta[7:]),
                )
            )
        tag, length = struct.unpack_from("<HI", self.idx, pos)
        if not self.fields or length < 6 or pos + length > len(self.idx):
            raise FormatError("Invalid block directory header")
        if tag != 0:
            raise FormatError("Missing block directory")
        self.table = pos + 6
        self.segment_count, remainder = divmod(length - 6, 8 * len(self.fields))
        if remainder:
            raise FormatError("Invalid block directory size")
        self.common = {}
        self.sections = {}
        pos += length
        while True:
            tag = struct.unpack_from("<H", self.idx, pos)[0]
            if tag == 65535:
                break
            length = struct.unpack_from("<I", self.idx, pos + 2)[0]
            if tag in (6, 7):
                if tag == 6 and length != self.segment_count + 1:
                    raise FormatError("Segment count mismatch")
                pos += 6
                continue
            if tag != 1:
                raise FormatError(f"Unexpected index section {tag}")
            if length < 8 or pos + length > len(self.idx):
                raise FormatError("Invalid field index section length")
            field_id = struct.unpack_from("<H", self.idx, pos + 6)[0]
            subpos = pos + 8
            sections = {}
            while subpos < pos + length:
                subtag, sublength = struct.unpack_from("<HI", self.idx, subpos)
                if sublength < 6 or subpos + sublength > pos + length:
                    raise FormatError("Invalid section length")
                sections[subtag] = (subpos + 6, sublength - 6)
                subpos += sublength
            if subpos != pos + length:
                raise FormatError("Section overrun")
            self.sections[field_id] = sections
            if 12 in sections:
                start, size = sections[12]
                n = struct.unpack_from("<I", self.idx, start)[0]
                strings = self.idx[start + 4 : start + size].split(b"\0")
                if len(strings) - 1 != n:
                    raise FormatError("Common dictionary count mismatch")
                self.common[field_id] = [s.decode("cp1252") for s in strings[:-1]]
            pos += length
        self.names = {f["name"]: f["id"] for f in self.fields}

    def block(self, segment, field):
        if isinstance(field, str):
            field = self.names[field]
        if not 0 <= field < len(self.fields):
            raise IndexError(field)
        if not 0 <= segment < self.segment_count:
            raise IndexError(segment)
        off, size = struct.unpack_from(
            "<II", self.idx, self.table + (segment * len(self.fields) + field) * 8
        )
        if off + size > len(self.dat):
            raise FormatError("Block outside data file")
        return self.dat[off : off + size]

    def column(self, segment, field):
        """Return one decoded column. Treat cached lists as read-only."""
        if isinstance(field, str):
            field = self.names[field]
        try:
            return self._cached_column(segment, field)
        except (IndexError, struct.error, UnicodeError) as exc:
            raise FormatError(f"Invalid column {segment}:{field}") from exc

    def _decode_column(self, segment, field):
        data = self.block(segment, field)
        if not data:
            return []
        typ = self.fields[field]["type"]
        if typ in (1, 7, 11):
            words, kind, pos = dictionary(data)
            if pos == len(data):
                return words
            refs, end = integers(data, pos)
            if end != len(data):
                raise FormatError("Trailing column data")
            common = self.common.get(field, []) if kind == 11 else []
            combined = common + words
            if any(v >= len(combined) for v in refs):
                raise FormatError("Dictionary reference out of range")
            return [combined[v] for v in refs]
        if typ in (2, 5, 8):
            if (
                typ == 8
                and data[:2] == b"\x0a\0"
                and 16 <= struct.unpack_from("<I", data, 2)[0] <= len(data)
            ):
                length = struct.unpack_from("<I", data, 2)[0]
                a, end = integers(data, 6)
                b, end = integers(data, length)
                if end != len(data):
                    raise FormatError("Trailing composite data")
                return list(zip(a, b))
            values, end = integers(data)
            if end != len(data):
                raise FormatError("Trailing integer data")
            return values
        if typ == 4:
            return postcode_bits(data)
        if typ == 3:
            if data[0] & 128:
                n, pos = varint(data, mode=2)
                alphabet = data[pos : pos + 4]
                pos += 4
                if pos + n // 4 + 1 != len(data):
                    raise FormatError("Packed character length mismatch")
                return [chr(alphabet[(data[pos + i // 4] >> (2 * (i % 4))) & 3]) for i in range(n)]
            return list(data.decode("cp1252"))
        raise FormatError(f"Unsupported column type {typ}")

    def sector(self, segment):
        key = self.column(segment, "segment")[0]
        outward, inward = key[:-1], key[-1]
        if len(outward) >= 4 and outward[2] == "0" and outward[3].isdigit():
            outward = outward[:2] + outward[3:]
        return outward.replace(" ", "") + " " + inward

    def segment_columns(self, segment):
        return {f["name"]: self.column(segment, f["id"]) for f in self.fields}

    def validate_segment(self, segment):
        c = self.segment_columns(segment)
        p = len(c["postcode"])
        locality_count = len(c["localitiesToPostcodes"])
        a = len(c["houseno"])
        n = len(c["surname"])
        for f in self.fields:
            name = f["name"]
            if name in (
                "segment",
                "postcode",
                "postcodesToLocalities",
                "localitiesToAddresses",
                "addressesToNames",
                "addressesToPostcodes",
                "namesToPostcodes",
                "localitiesToPostcodes",
            ):
                continue
            expected = (p, locality_count * 2, a, n)[f["level"]]
            if len(c[name]) != expected:
                raise FormatError(f"{name}: {len(c[name])} values, expected {expected}")
        for field, parent, child in [
            ("postcodesToLocalities", p, locality_count),
            ("localitiesToAddresses", locality_count, a),
            ("addressesToNames", a, n),
        ]:
            v = c[field]
            if (
                len(v) != parent + 1
                or v[0] != 0
                or v[-1] != child
                or any(a > b for a, b in zip(v, v[1:]))
            ):
                raise FormatError(f"Invalid parent/child boundaries: {field}")
        for field, count in [("addressesToPostcodes", a), ("namesToPostcodes", n)]:
            v = c[field]
            if len(v) != count + 1 or v[-1] != p or any(i < 0 or i >= p for i in v[:-1]):
                raise FormatError(f"Invalid postcode references: {field}")
        for pc in range(p):
            for loc in range(c["postcodesToLocalities"][pc], c["postcodesToLocalities"][pc + 1]):
                if c["localitiesToPostcodes"][loc] != pc:
                    raise FormatError("Locality/postcode disagreement")
                for addr in range(
                    c["localitiesToAddresses"][loc], c["localitiesToAddresses"][loc + 1]
                ):
                    if c["addressesToPostcodes"][addr] != pc:
                        raise FormatError("Address/postcode disagreement")
                    start, end = c["addressesToNames"][addr : addr + 2]
                    if any(v != pc for v in c["namesToPostcodes"][start:end]):
                        raise FormatError("Name/postcode disagreement")
        return dict(postcodes=p, localities=locality_count, addresses=a, entries=n)

    def records(self, segment, indices=None):
        c = self.segment_columns(segment)
        alphabet = "ABDEFGHJLNPQRSTUWXYZ"
        sector = self.sector(segment)
        postcodes = [sector + alphabet[i // 20] + alphabet[i % 20] for i in c["postcode"]]
        localities = len(c["localitiesToPostcodes"])
        for i in range(len(c["surname"])) if indices is None else indices:
            addr = bisect_right(c["addressesToNames"], i) - 1
            loc = bisect_right(c["localitiesToAddresses"], addr) - 1
            pc = c["namesToPostcodes"][i]
            row = dict(segment_id=segment, entry_id=i, household_id=f"{segment}:{addr}")
            for f in self.fields:
                if f["name"] in ("county", "town"):
                    row[f["name"]] = c[f["name"]][pc]
                elif f["name"] in ("locality", "street"):
                    row[f["name"] + "_1"] = c[f["name"]][loc]
                    row[f["name"] + "_2"] = c[f["name"]][loc + localities]
                elif f["level"] == 2 and f["name"] in ("houseno", "building", "subbuilding"):
                    row[f["name"]] = c[f["name"]][addr]
                elif f["level"] == 3 and f["type"] not in (5, 8):
                    row[f["name"]] = c[f["name"]][i]
            row["postcode"] = postcodes[pc]
            yield row

    def close(self):
        """Release mapped files and decoded columns; safe to call more than once."""
        self._cached_column.cache_clear()
        self._resources.close()

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        self.close()
