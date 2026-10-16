"""Small invented residential sources for both modern source profiles."""

import hashlib
import struct
import tempfile
from contextlib import contextmanager
from pathlib import Path
from unittest.mock import patch

from test_modern import leaf_fixture

from ukinfo_reader.editions import Edition


def fixed_table(width, rows):
    data = bytearray(4096)
    struct.pack_into("<I", data, 8, 256 + width * len(rows) - 1)
    struct.pack_into("<I", data, 20, len(rows) + 1)
    struct.pack_into("<I", data, 24, len(rows))
    struct.pack_into("<H", data, 44, width)
    data[160:172] = b"FirstRecPos\0"
    struct.pack_into("<I", data, 172, 256)
    for i, row in enumerate(rows):
        assert len(row) <= width
        data[256 + i * width : 256 + (i + 1) * width] = row.ljust(width, b"\0")
    return data


def residence(
    kind, names, building=None, subbuilding=None, phone=0, area=0, contact=0, precision=0
):
    flags = (128 if building is not None else 0) | (64 if subbuilding is not None else 0)
    pc_flags = (32 if area else 0) | (128 if precision else 0)
    data = bytearray([flags | (kind << 3) | len(names)])
    data.extend(names[0].to_bytes(3, "little"))
    data.extend((1 | (pc_flags << 16)).to_bytes(3, "little"))
    data.extend((1).to_bytes(2, "little"))
    for name in names[1:]:
        data.extend(name.to_bytes(3, "little"))
    for value, width, present in [
        (building, 3, building is not None),
        (subbuilding, 2, subbuilding is not None),
        (contact, 3, kind in (2, 5)),
        (phone, 3, 1 <= kind <= 6),
        (area, 2, bool(area)),
        (precision, 1, bool(precision)),
    ]:
        if present:
            data.extend(value.to_bytes(width, "little"))
    return bytes(data)


def place(name, kind, county=0, town=0):
    return (
        name.encode().ljust(30, b"\0")
        + kind.encode()
        + b"C\1\0\0\0"
        + struct.pack("<II", county, town)
        + bytes(12)
    )


@contextmanager
def modern_source(updated=True, undefined_name=False, business=False):
    with tempfile.TemporaryDirectory() as temporary:
        root = Path(temporary)
        files = {}
        files["d1.dat"] = fixed_table(
            56,
            [
                place("FIXTURE COUNTY", "C"),
                place("FIXTURE TOWN", "T", 1),
                place("FIXTURE VILLAGE", "V", 1, 2),
            ],
        )
        files["d7.dat"] = b"\0FICTION ROAD\0"
        offsets = []
        for suffix in ["", "upd"] if updated else [""]:
            old = not updated
            surname_width, name_width, house_width, sub_width = (
                (24, 21, 10, 10) if old else (54, 44, 20, 50) if suffix else (34, 32, 12, 30)
            )
            names = (
                [b"UPDATED ADA", b"UPDATED BERT", b"UPDATED CECIL"]
                if suffix
                else [b"ADA", b"BERT", b"CECIL"]
            )
            if undefined_name and not suffix:
                names[0] = b"\x90DA"
            files["d2" + suffix + ".dat"] = fixed_table(
                surname_width, [b"EXAMPLE".ljust(surname_width - 4, b"\0") + bytes(4)]
            )
            tail = 1 if old else 4
            files["d3" + suffix + ".dat"] = fixed_table(
                name_width, [n.ljust(name_width - tail, b"\0") + bytes(tail) for n in names]
            )
            files["d4" + suffix + ".dat"] = fixed_table(house_width, [b"20" if suffix else b"10"])
            files["d8" + suffix + ".dat"] = fixed_table(sub_width, [b"FLAT 1"])
            files["d5" + suffix + ".dat"] = b"\0EXAMPLE HOUSE\0FIXED HOUSE\0"
            pc = b"ZZ991AA" + struct.pack("<H", 3) + (1).to_bytes(3, "little") + bytes(10)
            files["d6new" + suffix + ".dat"] = fixed_table(22, [pc])
            if suffix:
                records = [residence(3, [1], building=1, phone=987654, area=1234)]
                prefix = residence(0, [1])
            else:
                records = [
                    residence(0, [0x800001, 2], building=3, subbuilding=1),
                    residence(3, [3], phone=10000123, area=1234),
                    residence(4, [1], phone=1234567, area=1234),
                    residence(5, [1, 2], contact=3, phone=7654321, area=1234),
                    residence(0, [2], precision=11),
                ]
                prefix = residence(0, [1]) if old else b""
            data = bytearray(prefix)
            current_offsets = []
            for record in records:
                current_offsets.append(len(data))
                data.extend(record)
            if not suffix:
                offsets = current_offsets
            files["ukupd.dat" if suffix else "uk.dat"] = data
            files["ukspos" + suffix + ".idx"] = leaf_fixture([(len(prefix).to_bytes(4, "big"), 1)])
        if updated:
            files["delr.idx"] = leaf_fixture(
                sorted(
                    [(offsets[1].to_bytes(4, "little"), 1), (offsets[3].to_bytes(4, "little"), 2)]
                )
            )
        else:
            files["hfix.idx"] = leaf_fixture(
                [(offsets[0].to_bytes(4, "big"), 15), ((4096).to_bytes(4, "big"), 15)]
            )
        if business:
            assert updated
            for suffix in ("", "upd"):
                files["dd" + suffix + ".dat"] = b"FICTIONAL\0WORKS\0"
                files["lobdesc" + suffix + ".dat"] = b"\0INVENTED INDUSTRY\0"
                files["dbsic" + suffix + ".dat"] = fixed_table(5, [b"1234"])
                # All optional fields, including both fax values and precision.
                raw = bytes([255, 0x91]) + (1).to_bytes(3, "little") + (1).to_bytes(2, "little")
                for value, width in [
                    (1, 3),
                    (11, 3),
                    (1, 3),
                    (1, 2),
                    (1, 3),
                    (1, 2),
                    (10000123, 3),
                    (1234, 2),
                    (1234, 3),
                    (7654321, 3),
                    (10, 1),
                ]:
                    raw += value.to_bytes(width, "little")
                files["bus" + suffix + ".dat"] = raw if suffix else raw + raw
            files["delb.idx"] = leaf_fixture([(bytes(4), 1)])
        for name, content in files.items():
            (root / name).write_bytes(content)
        edition = Edition(
            "se2005-synthetic" if updated else "se2004-synthetic",
            "Invented source",
            tuple(
                (name, len(content), hashlib.sha256(content).hexdigest())
                for name, content in files.items()
            ),
            format="modern",
        )
        with patch("ukinfo_reader.editions.SUPPORTED_EDITIONS", (edition,)):
            yield root, offsets
