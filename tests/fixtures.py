"""Generate a tiny, entirely invented database. No source-disc content is copied."""

import hashlib
import struct
import tempfile
from contextlib import contextmanager
from pathlib import Path
from unittest.mock import patch

from ukinfo_reader.editions import Edition

FIELDS = [
    ("segment", 7, 0),
    ("postcode", 4, 0),
    ("postcodesToLocalities", 5, 1),
    ("county", 1, 0),
    ("town", 1, 0),
    ("locality", 11, 1),
    ("street", 11, 1),
    ("localitiesToAddresses", 5, 2),
    ("houseno", 2, 2),
    ("building", 1, 2),
    ("subbuilding", 1, 2),
    ("addressesToNames", 5, 3),
    ("phone", 1, 3),
    ("surname", 1, 3),
    ("firstName", 1, 3),
    ("initial2", 3, 3),
    ("residency", 2, 3),
    ("gender", 3, 3),
    ("organisation", 1, 3),
    ("business", 1, 3),
    ("sic", 1, 3),
    ("sicHeading", 1, 3),
    ("sicDescription", 1, 3),
    ("addressesToPostcodes", 8, 2),
    ("namesToPostcodes", 8, 3),
    ("localitiesToPostcodes", 8, 1),
]


def vint(value):
    groups = [value & 127]
    value >>= 7
    while value:
        groups.append(value & 127)
        value >>= 7
    groups.reverse()
    groups[-1] |= 128
    return bytes(groups)


def numbers(values):
    return struct.pack("<IHI", len(values), 11, sum(values) & 0xFFFFFFFF) + b"".join(
        map(vint, values)
    )


def strings(values, common=()):
    local = list(dict.fromkeys(v for v in values if v not in common))
    combined = list(common) + local
    return (
        vint(len(local))
        + bytes([11 if common else 4])
        + b"".join(v.encode("ascii") + b"\0" for v in local)
        + numbers([combined.index(v) for v in values])
    )


def section(tag, payload):
    return struct.pack("<HI", tag, len(payload) + 6) + payload


def write_fixture(directory):
    values = {
        "segment": ["ZZ01 1"],
        "postcode": None,
        "postcodesToLocalities": [0, 1],
        "county": ["FIXTURE COUNTY"],
        "town": ["TEST TOWN"],
        "locality": ["", "FIXTURE VILLAGE"],
        "street": ["SIDE LANE", "TEST ROAD"],
        "localitiesToAddresses": [0, 2],
        "houseno": [10, 20],
        "building": ["", "FIXTURE LAB"],
        "subbuilding": ["", ""],
        "addressesToNames": [0, 2, 3],
        "phone": ["", "", ""],
        "surname": ["EXAMPLE", "EXAMPLE", "LAB"],
        "firstName": ["ADA", "BERT", ""],
        "initial2": [" ", "X", " "],
        "residency": [0, 1, 0],
        "gender": ["F", "M", "U"],
        "organisation": ["", "", "FIXTURE LAB"],
        "business": ["", "", "TESTING"],
        "sic": ["", "", ""],
        "sicHeading": ["", "", ""],
        "sicDescription": ["", "", ""],
        "addressesToPostcodes": [0, 0, 1],
        "namesToPostcodes": [0, 0, 0, 1],
        "localitiesToPostcodes": [0],
    }
    schema = b""
    data = b""
    table = b""
    for field, (name, kind, level) in enumerate(FIELDS):
        schema += (
            struct.pack("<H", field)
            + name.encode()
            + b"\0"
            + struct.pack("<6H2s4H", kind, level, 0, 0, 1, 0, b"01", 0, 0, 0, 65535)
        )
        if kind in (2, 5, 8):
            block = numbers(values[name])
        elif kind == 3:
            block = "".join(values[name]).encode()
        elif kind == 4:
            block = bytes.fromhex("85 80")  # only suffix AA exists
        else:
            block = strings(values[name], common=["EXAMPLE"] if name == "surname" else [])
        table += struct.pack("<II", len(data), len(block))
        data += block
    common = section(12, struct.pack("<I", 1) + b"EXAMPLE\0")
    index = (
        schema
        + b"\xff\xff"
        + section(0, table)
        + struct.pack("<HIHI", 6, 2, 7, 0)
        + section(1, struct.pack("<H", 13) + common)
        + b"\xff\xff"
    )
    directory = Path(directory)
    for name, content in [("ukid.idx", index), ("ukid.dat", data)]:
        (directory / name).write_bytes(content)
    return Edition(
        "synthetic-test",
        "Invented test database",
        tuple(
            (name, len(content), hashlib.sha256(content).hexdigest())
            for name, content in [("ukid.idx", index), ("ukid.dat", data)]
        ),
    )


@contextmanager
def synthetic_database():
    with tempfile.TemporaryDirectory() as directory:
        edition = write_fixture(directory)
        with patch("ukinfo_reader.editions.SUPPORTED_EDITIONS", (edition,)):
            yield Path(directory)
