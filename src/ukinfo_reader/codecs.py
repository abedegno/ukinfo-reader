"""Binary codecs recovered from the SE 4.0 reader. No Windows libraries are required."""

import struct


class FormatError(ValueError):
    """A file or compressed block violates the documented format."""


def varint(data, pos=0, mode=0):
    """Big-endian seven-bit groups; mode 0 marks the LAST byte with bit 7."""
    value = 0
    for _ in range(len(data) - pos):
        b = data[pos]
        pos += 1
        value = ((value << 7) | (b & 127)) & 0xFFFFFFFF
        if (mode == 0 and b & 128) or (mode == 2 and (pos == len(data) or data[pos] < 128)):
            return value, pos
    raise FormatError("Overlong integer")


def integers(data, pos=0):
    count, encoding, checksum = struct.unpack_from("<IHI", data, pos)
    pos += 10
    if count > 10_000_000 or encoding > 12:
        raise FormatError(f"Invalid integer array: {count=}, {encoding=}")
    values = []
    previous = 0
    if encoding == 12:
        n, pos = varint(data, pos)
        lookup = []
        for _ in range(n + 1):
            v, pos = varint(data, pos)
            lookup.append(v)
        for _ in range(count):
            v, pos = varint(data, pos)
            values.append(lookup[v] if v < 128 else v - 128)
    else:
        width = encoding % 3
        rle = 2 < encoding < 9
        while len(values) < count:
            if width == 0:
                token = data[pos]
                pos += 1
            elif width == 1:
                token = struct.unpack_from("<H", data, pos)[0]
                pos += 2
            else:
                token, pos = varint(data, pos)
            run = 0
            if rle:
                if width == 0 and token > 200:
                    run = token - 200
                elif width == 1 and token > 65000:
                    run = token - 65000
                elif width == 2 and token & 1:
                    run = token >> 1
            if run:
                if run > count - len(values):
                    raise FormatError("Repeat exceeds declared array length")
                values.extend([previous] * run)
            else:
                previous = (
                    ((token if token < 0x80000000 else token - 0x100000000) >> 1)
                    if rle and width == 2
                    else token
                )
                values.append(previous)
        if encoding < 6:
            total = 0
            for i, v in enumerate(values):
                total = (total + v) & 0xFFFFFFFF
                values[i] = total
    if len(values) != count or sum(values) & 0xFFFFFFFF != checksum:
        raise FormatError(f"Integer checksum/count mismatch ({encoding=}, {count=})")
    return values, pos


def dictionary(data, pos=0):
    count, pos = varint(data, pos)
    kind = data[pos]
    pos += 1
    if kind not in (4, 11):
        raise FormatError(f"Unexpected dictionary kind {kind}")
    strings = []
    previous = b""
    for _ in range(count):
        end = data.find(b"\0", pos)
        if end < 0:
            raise FormatError("Unterminated string")
        word = data[pos:end]
        if word and word[0] < 32:
            prefix = word[0]
            if prefix > len(previous):
                raise FormatError("Invalid shared prefix")
            word = previous[:prefix] + word[1:]
        strings.append(word.decode("cp1252"))
        previous = word
        pos = end + 1
    return strings, kind, pos


def postcode_bits(data, size=400):
    """Decode the original reader's sparse bitmap opcodes (0x4031fc)."""
    bitmap = bytearray((size + 7) // 8)
    pos = 0
    while True:
        opcode, pos = varint(data, pos)
        cursor = 0
        if opcode == 0:
            break
        if opcode > 4:
            bit = opcode - 5
            if bit >= size:
                raise FormatError("Postcode bit outside sector")
            bitmap[bit // 8] |= 1 << (bit % 8)
            continue
        while True:
            v, pos = varint(data, pos)
            if v == 0:
                break
            if opcode == 1:
                delta, pos = varint(data, pos)
                cursor += delta
                if cursor + v > len(bitmap):
                    raise FormatError("Bitmap run overrun")
                bitmap[cursor : cursor + v] = b"\xff" * v
            elif opcode == 2:
                cursor += v - 1
                if cursor >= size:
                    raise FormatError("Bitmap bit overrun")
                bitmap[cursor // 8] |= 1 << (cursor % 8)
            elif opcode == 3:
                cursor += v - 1
                end = data.find(b"\0", pos)
                if end < 0 or cursor + end - pos > len(bitmap):
                    raise FormatError("Bitmap literal overrun")
                bitmap[cursor : cursor + end - pos] = data[pos:end]
                pos = end + 1
            elif opcode == 4:
                cursor += v - 1
                if cursor >= len(bitmap):
                    raise FormatError("Bitmap byte overrun")
                bitmap[cursor] = data[pos]
                pos += 1
    if pos != len(data):
        raise FormatError("Trailing bitmap data")
    return [i for i in range(size) if bitmap[i // 8] & (1 << (i % 8))]
