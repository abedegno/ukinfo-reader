"""Read the prefix-compressed leaf chains used on the PC Format 78 disc."""

import struct
from bisect import bisect_left, bisect_right

from .codecs import FormatError


class LeafIndex:
    """A leaf directory, retaining only page offsets, counts and first keys in memory."""

    def __init__(self, data, key_size):
        self.data = data
        self.key_size = key_size
        if len(data) < 1024 or struct.unpack_from("<H", data, 66)[0] != key_size:
            raise FormatError("Unexpected c-tree key size")
        if struct.unpack_from("<H", data, 42)[0] != 1024:
            raise FormatError("Unexpected c-tree page size")
        self.count = struct.unpack_from("<I", data, 24)[0]
        self.pages, self.starts, self.first_keys = [], [], []
        page = struct.unpack_from("<I", data, 72)[0]
        previous = 0
        seen = set()
        total = 0
        while page:
            if page in seen or page % 1024 or page < 1024 or page + 1024 > len(data):
                raise FormatError("Invalid c-tree leaf chain")
            seen.add(page)
            nxt, prev, count, used = struct.unpack_from("<IIHH", data, page)
            if prev != previous or data[page + 17] != 1 or not count or used > 1006:
                raise FormatError("Invalid c-tree leaf header")
            self.pages.append(page)
            self.starts.append(total)
            first = next(self.page_keys(page))[0]
            if self.first_keys and first < self.first_keys[-1]:
                raise FormatError("Unsorted c-tree leaf directory")
            self.first_keys.append(first)
            total += count
            previous, page = page, nxt
        if total != self.count:
            raise FormatError("c-tree key count mismatch")

    def page_keys(self, page):
        count, used = struct.unpack_from("<HH", self.data, page + 8)
        cursor, end = page + 18, page + 18 + used
        old = bytes(self.key_size)
        for _ in range(count):
            if cursor >= end:
                raise FormatError("Truncated c-tree key")
            shared = self.data[cursor]
            cursor += 1
            literal = self.key_size - shared
            if literal < 0 or cursor + literal > end:
                raise FormatError("Invalid c-tree prefix")
            key = old[:shared] + self.data[cursor : cursor + literal]
            cursor += literal
            old = key
            yield key[:-4], int.from_bytes(key[-4:], "big")
        if cursor != end:
            raise FormatError("Trailing c-tree leaf payload")

    def keys(self, prefix=None):
        start = 0 if prefix is None else max(0, bisect_left(self.first_keys, prefix) - 1)
        for page in self.pages[start:]:
            for key, pointer in self.page_keys(page):
                if prefix is not None:
                    if key[: len(prefix)] < prefix:
                        continue
                    if key[: len(prefix)] > prefix:
                        return
                yield key, pointer

    def ordinal(self, index):
        if not 0 <= index < self.count:
            raise FormatError("Building-name ordinal out of range")
        page = bisect_right(self.starts, index) - 1
        for i, (key, _) in enumerate(self.page_keys(self.pages[page])):
            if i == index - self.starts[page]:
                return key
        raise FormatError("Missing c-tree ordinal")
