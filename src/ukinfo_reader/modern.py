"""Physical structural decoders for the 2004 and 2005 SE sources.

These primitives expose physical source records. For joined entries and update
selection use modern_reader or Database. See docs/formats/modern-se.md.
"""

import struct
from dataclasses import dataclass

from .codecs import FormatError


class ModernLeafIndex:
    """Iterate the observed c-tree leaves with separate little-endian pointers.

    The logical key excludes the pointer. Compression uses a shared prefix and,
    optionally, a run of the file's fill byte at the end of the key.
    """

    def __init__(self, data):
        if len(data) < 128:
            raise FormatError("Truncated modern c-tree header")
        self.data = data
        self.page_size = struct.unpack_from("<H", data, 42)[0]
        self.key_size = struct.unpack_from("<H", data, 66)[0]
        self.count = struct.unpack_from("<I", data, 24)[0]
        self.first_page = struct.unpack_from("<I", data, 72)[0]
        self.compression = data[57]
        self.fill = bytes([data[60]])
        if (
            self.page_size not in (4096, 16384)
            or not 1 <= self.key_size <= 255
            or self.compression not in (4, 12)
            or data[64] != 1
        ):
            raise FormatError("Unrecognized modern c-tree layout")

    def keys(self):
        """Yield (logical key bytes, source pointer), validating the whole leaf chain."""
        data = self.data
        page, previous, total = self.first_page, 0, 0
        seen = set()
        last_key = None
        while page:
            if (
                page in seen
                or page < self.page_size
                or page % self.page_size
                or page + self.page_size > len(data)
            ):
                raise FormatError("Invalid modern c-tree leaf chain")
            seen.add(page)
            nxt, prev, count, used = struct.unpack_from("<IIHH", data, page)
            if prev != previous or data[page + 17] != 1 or not count:
                raise FormatError("Invalid modern c-tree leaf header")
            cursor, end = page + 18, page + 18 + used
            if end > page + self.page_size:
                raise FormatError("Modern c-tree payload exceeds page")
            old = bytes(self.key_size)
            for _ in range(count):
                header = 6 if self.compression & 8 else 5
                if cursor + header > end:
                    raise FormatError("Truncated modern c-tree key")
                pointer = struct.unpack_from("<I", data, cursor)[0]
                shared = data[cursor + 4]
                tail = data[cursor + 5] if header == 6 else 0
                cursor += header
                literal = self.key_size - shared - tail
                if literal < 0 or cursor + literal > end:
                    raise FormatError("Invalid modern c-tree compression")
                key = old[:shared] + data[cursor : cursor + literal] + self.fill * tail
                cursor += literal
                if last_key is not None and key < last_key:
                    raise FormatError("Unsorted modern c-tree keys")
                old = last_key = key
                total += 1
                yield key, pointer
            if cursor != end:
                raise FormatError("Trailing modern c-tree payload")
            previous, page = page, nxt
        if total != self.count:
            raise FormatError("Modern c-tree key count mismatch")


class ModernFixedTable:
    """Read fixed records using the embedded FirstRecPos resource and live count."""

    def __init__(self, data):
        if len(data) < 128:
            raise FormatError("Truncated modern fixed table")
        marker = data.find(b"FirstRecPos\0", 128, min(len(data), 4096))
        if marker < 0 or marker + 16 > len(data):
            raise FormatError("Missing modern fixed-table start resource")
        self.start = struct.unpack_from("<I", data, marker + 12)[0]
        self.width = struct.unpack_from("<H", data, 44)[0]
        self.count = struct.unpack_from("<I", data, 24)[0]
        self.data = data
        end = self.start + self.width * self.count
        if (
            not self.width
            or self.start < marker + 16
            or end > len(data)
            or struct.unpack_from("<I", data, 8)[0] != end - 1
        ):
            raise FormatError("Invalid modern fixed-table bounds")

    def raw(self, source_id):
        """Return a record for a one-based source ID; zero is not a table row."""
        if not 1 <= source_id <= self.count:
            raise FormatError("Modern fixed-table reference out of range")
        offset = self.start + (source_id - 1) * self.width
        return self.data[offset : offset + self.width]


def residential_length(data, offset=0):
    """Return the physical record length; reject invalid counts and truncation."""
    if offset < 0 or offset + 9 > len(data):
        raise FormatError("Truncated modern residential record")
    flags, pc_flags = data[offset], data[offset + 6]
    count, kind = flags & 7, (flags >> 3) & 7
    if not 1 <= count <= 5:
        raise FormatError("Invalid modern residential name count")
    length = (
        6
        + count * 3
        + (3 if flags & 128 else 0)
        + (2 if flags & 64 else 0)
        + (3 if kind in (2, 5) else 0)
        + (3 if 1 <= kind <= 6 else 0)
        + (2 if pc_flags & 32 else 0)
        + (1 if pc_flags & 128 else 0)
    )
    if offset + length > len(data):
        raise FormatError("Truncated modern residential record")
    return length


@dataclass(frozen=True)
class PhysicalResidence:
    """Raw source values. Optional fields are named by their storage condition.

    No surname is stored here: the ukspos index supplies the surname ID for each
    range of physical records. Name codes retain their source flag bits.
    """

    offset: int
    length: int
    kind: int
    flags: int
    name_codes: tuple[int, ...]
    postcode_id: int
    postcode_flags: int
    house_id: int
    optional_flag80: int | None
    optional_flag40: int | None
    optional_kind25: int | None
    optional_kind16: int | None
    optional_pc20: int | None
    optional_pc80: int | None


def decode_residence(data, offset=0):
    """Decode a physical record without applying dictionary joins or deletion rules."""
    length = residential_length(data, offset)
    flags, pc_flags = data[offset], data[offset + 6]
    count, kind = flags & 7, (flags >> 3) & 7
    names = [int.from_bytes(data[offset + 1 : offset + 4], "little")]
    cursor = offset + 9

    def take(width, present=True):
        nonlocal cursor
        if not present:
            return None
        value = int.from_bytes(data[cursor : cursor + width], "little")
        cursor += width
        return value

    names.extend(take(3) for _ in range(count - 1))
    a = take(3, flags & 128)
    # An additional high bit comes from the postcode flag byte, not an extra byte.
    if a is not None and pc_flags & 64:
        a |= 1 << 24
    b = take(2, flags & 64)
    c = take(3, kind in (2, 5))
    d = take(3, 1 <= kind <= 6)
    e = take(2, pc_flags & 32)
    f = take(1, pc_flags & 128)
    return PhysicalResidence(
        offset,
        length,
        kind,
        flags,
        tuple(names),
        int.from_bytes(data[offset + 4 : offset + 7], "little") & 0x1FFFFF,
        pc_flags & 0xE0,
        int.from_bytes(data[offset + 7 : offset + 9], "little"),
        a,
        b,
        c,
        d,
        e,
        f,
    )


@dataclass(frozen=True)
class PhysicalBusiness:
    """Business source values; word codes are one-based byte offsets in dd.dat."""

    offset: int
    length: int
    flags: int
    kind: int
    word_codes: tuple[int, ...]
    postcode_id: int
    house_id: int
    lob_reference: int | None
    sic_id: int | None
    building_reference: int | None
    subbuilding_id: int | None
    subscriber: int | None
    area: int | None
    fax_area: int | None
    fax_subscriber: int | None
    precision: int


def decode_business(data, offset=0):
    """Decode the V10.5 business stream, checking every optional field's bounds."""
    if offset < 0 or offset + 10 > len(data):
        raise FormatError("Truncated modern business record")
    flags, control = data[offset : offset + 2]
    count = (control & 15) + 1
    cursor = offset + 7

    def take(width, present=True):
        nonlocal cursor
        if not present:
            return None
        if cursor + width > len(data):
            raise FormatError("Truncated modern business record")
        value = int.from_bytes(data[cursor : cursor + width], "little")
        cursor += width
        return value

    codes = tuple(take(3) for _ in range(count))
    lob = take(3, flags & 1)
    sic = take(2, flags & 2)
    building = take(3, flags & 4)
    if building is not None and control & 64:
        building |= 1 << 24
    flat = take(2, flags & 8)
    phone = take(3, flags & 16)
    area = take(2, flags & 32)
    fax_area = take(3, flags & 64)
    fax = take(3, flags & 128)
    precision = take(1, control & 128) or 0
    return PhysicalBusiness(
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
        area,
        fax_area,
        fax,
        precision,
    )
