"""Invented PC Format 78 dictionaries and records, encoded independently of the reader."""

import hashlib
import struct
import tempfile
from contextlib import contextmanager
from pathlib import Path
from unittest.mock import patch

from ukinfo_reader.editions import Edition


def fixed(records, width):
    start = ((160 + width - 1) // width) * width
    end = start + len(records) * width
    data = bytearray(((end + 4095) // 4096) * 4096)
    struct.pack_into("<I", data, 8, end - 1)
    struct.pack_into("<H", data, 12, start - 32)
    struct.pack_into("<I", data, 20, len(records))
    struct.pack_into("<H", data, 44, width)
    for i, record in enumerate(records):
        data[start + i * width : start + (i + 1) * width] = record.ljust(width, b"\0")
    return bytes(data)


def index(keys, size, per_page=3):
    pages = [keys[i : i + per_page] for i in range(0, len(keys), per_page)]
    data = bytearray((len(pages) + 2) * 1024)
    struct.pack_into("<I", data, 24, len(keys))
    struct.pack_into("<H", data, 42, 1024)
    struct.pack_into("<H", data, 66, size)
    struct.pack_into("<I", data, 72, 2048)
    for i, group in enumerate(pages):
        page = (i + 2) * 1024
        payload = b""
        previous = bytes(size)
        for key, pointer in group:
            full = key + pointer.to_bytes(4, "big")
            shared = 0
            while shared < size and full[shared] == previous[shared]:
                shared += 1
            payload += bytes([shared]) + full[shared:]
            previous = full
        struct.pack_into(
            "<IIHH",
            data,
            page,
            page + 1024 if i + 1 < len(pages) else 0,
            page - 1024 if i else 0,
            len(group),
            len(payload),
        )
        data[page + 17] = 1
        data[page + 18 : page + 18 + len(payload)] = payload
    return bytes(data)


def name_code(value):
    return bytes((48 + value // 16384, 48 + (value // 128) % 128, 48 + value % 128))


def main_record(flags, names=(0, 1), surname=b"000"):
    optional = b""
    if flags & 4:
        optional += (1234).to_bytes(2, "little") + (567890).to_bytes(3, "little")
    if flags & 2:
        optional += b"01"  # house dictionary ID 2
    if flags & 1:
        optional += (2).to_bytes(3, "little")  # building ordinal 2, regardless of its pointer
    length = 11 + len(optional) + 3 * (len(names) - 1)
    head = bytearray(b"02" + surname + names[0].to_bytes(3, "little") + bytes(3))
    head[7] |= (length % 32) << 3
    head[8] = flags << 5
    return bytes(head) + optional + b"".join(n.to_bytes(3, "little") for n in names[1:])


def write_fixture(directory):
    directory = Path(directory)
    places = []
    for county, town, name, kind in [
        (0, 0, b"FIXTURE COUNTY", b"C"),
        (1, 0, b"TEST TOWN", b"TC"),
        (1, 2, b"FIXTURE VILLAGE", b"VC"),
        (0, 0, b"OTHER TOWN", b"TO"),
        (0, 4, b"OTHER VILLAGE", b"VO"),
    ]:
        places.append(
            struct.pack("<HH", county, town)
            + name.ljust(30, b"\0")
            + kind.ljust(2, b"\0")
            + bytes(8)
        )
    files = {
        "d1.dat": fixed(places, 44),
        "d2.dat": fixed([b"EXAMPLE", b"LAB"], 19),
        "d3.dat": fixed([b"ADA", b"BERT", b"CECIL"], 16),
        "d4.dat": fixed([b"10", b"20A"], 10),
        "d5.idx": index(
            [(b"FIXTURE HALL".ljust(30, b"\0"), 99), (b"TEST LAB".ljust(30, b"\0"), 2)], 34
        ),
        "d6.dat": fixed([b"000ZZ1 1AA00"], 12),
        "d7.dat": fixed([b"TEST ROAD"], 32),
    }
    data = b"*** R.A. ***"
    keys = []
    for record in [main_record(i) for i in range(8)] + [
        main_record(7, names=(0, 1, 2, 0, 1, 2, 0)),
        main_record(0, names=(0,), surname=bytes.fromhex("002c22")),
    ]:
        keys.append((record[2:5] + record[:2], len(data)))
        data += record
    data += main_record(0, surname=b"001")  # deliberately absent from live index
    files["Ukdemo.dat"] = data
    files["Uk1.idx"] = index(sorted(keys), 9)
    for name, content in files.items():
        (directory / name).write_bytes(content)
    return Edition(
        "synthetic-pcf78",
        "Invented early-format database",
        tuple(
            (name, len(content), hashlib.sha256(content).hexdigest())
            for name, content in files.items()
        ),
        format="pcf78",
    )


@contextmanager
def synthetic_pcf78():
    with tempfile.TemporaryDirectory() as directory:
        edition = write_fixture(directory)
        with patch("ukinfo_reader.editions.SUPPORTED_EDITIONS", (edition,)):
            yield Path(directory)
