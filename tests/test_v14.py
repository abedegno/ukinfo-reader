"""Invented records for the v14 physical format; no source bytes."""

import unittest

from ukinfo_reader.codecs import FormatError
from ukinfo_reader.v14 import (
    decode_business_v14,
    decode_residence_v14,
    residential_length_v14,
    telephone,
)


def residence(
    kind=5, names=(0x1234567, 2), building=0x12345678, flat=4, phone=0x12345678, precision=8
):
    flags = len(names) | kind << 3 | 0xC0
    pc = 0x234567 | (0x80 << 16)
    data = (
        bytes([flags])
        + names[0].to_bytes(4, "little")
        + pc.to_bytes(3, "little")
        + (7).to_bytes(2, "little")
    )
    data += b"".join(n.to_bytes(4, "little") for n in names[1:])
    data += building.to_bytes(4, "little") + flat.to_bytes(2, "little")
    if kind in (2, 5):
        data += (3).to_bytes(3, "little")
    if 1 <= kind <= 6:
        data += phone.to_bytes(4, "little")
    return data + bytes([precision])


class V14Tests(unittest.TestCase):
    def test_residential_widths_and_postcode_mask(self):
        data = residence()
        r = decode_residence_v14(data)
        self.assertEqual(r.length, len(data))
        self.assertEqual(residential_length_v14(data), len(data))
        self.assertEqual(r.name_codes, (0x1234567, 2))
        self.assertEqual(r.postcode_id, 0x234567)
        self.assertEqual(r.house_id, 7)
        self.assertEqual(r.optional_flag80, 0x12345678)
        self.assertEqual(r.optional_kind25, 3)
        self.assertEqual(r.optional_kind16, 0x12345678)
        self.assertIsNone(r.optional_pc20)
        self.assertEqual(r.optional_pc80, 8)
        for n in range(len(data)):
            with self.assertRaises(FormatError):
                decode_residence_v14(data[:n])

    def test_business_full_width_industry_building_phone_and_fax(self):
        data = bytes([0xFF, 0xA1]) + (0x123456).to_bytes(3, "little") + (4).to_bytes(2, "little")
        values = [
            (1, 3),
            (11, 3),
            (0x12345678, 4),
            (2, 2),
            (0x23456789, 4),
            (3, 2),
            (0x34567890, 4),
            (0x45678901, 4),
            (9, 1),
        ]
        data += b"".join(v.to_bytes(w, "little") for v, w in values)
        r = decode_business_v14(data)
        self.assertEqual(r.length, len(data))
        self.assertEqual(r.word_codes, (1, 11))
        self.assertEqual(r.lob_reference, 0x12345678)
        self.assertEqual(r.building_reference, 0x23456789)
        self.assertEqual(r.subscriber, 0x34567890)
        self.assertEqual(r.fax_subscriber, 0x45678901)
        self.assertEqual(r.area, 0)
        self.assertEqual(r.fax_area, 0)
        self.assertEqual(r.precision, 9)
        for n in range(len(data)):
            with self.assertRaises(FormatError):
                decode_business_v14(data[:n])

    def test_telephone_prefix_is_not_mislabelled_as_an_area_code(self):
        # High twelve bits select a source prefix; the final six digits retain zeroes.
        self.assertEqual(
            telephone((1 << 20) | 123, ("01234",)), ("01234000123", "01234", "000123", "")
        )
        self.assertEqual(telephone(0, ("01234",)), ("", "", "", ""))
        self.assertEqual(
            telephone((2 << 20) | 123, ("01234",))[3], "telephone_prefix_reference_out_of_range"
        )
        self.assertEqual(
            telephone((1 << 20) | 123, ("01234",), extension=True)[3],
            "telephone_prefix_reference_out_of_range",
        )
        self.assertEqual(telephone((1 << 20) | 123, ("01234+",))[3], "invalid_phone_prefix")
        self.assertEqual(telephone((1 << 20) | 1048575, ("01234",))[2], "048575")

    def test_invalid_count_and_negative_offsets_fail_closed(self):
        for data, offset in ((b"\0" * 100, 0), (residence(), -1), (residence(), 1000)):
            with self.assertRaises(FormatError):
                decode_residence_v14(data, offset)


class V14IntegrationTests(unittest.TestCase):
    def test_dispatch_filters_joins_and_business_phone_export(self):
        import hashlib
        import tempfile
        from pathlib import Path
        from unittest.mock import patch

        from modern_fixtures import fixed_table, place
        from test_modern import leaf_fixture

        from ukinfo_reader import Database
        from ukinfo_reader.editions import Edition
        from ukinfo_reader.validation import validate

        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            person = bytearray(
                residence(names=(1, 2), building=1, flat=1, phone=(1 << 20) | 123, precision=10)
            )
            person[5:8] = (0xC00001).to_bytes(3, "little")
            person[8:10] = (1).to_bytes(2, "little")
            business = bytes([0x5F, 0x20]) + (1).to_bytes(3, "little") + (1).to_bytes(2, "little")
            business += b"".join(
                v.to_bytes(w, "little")
                for v, w in [
                    (1, 3),
                    (1, 4),
                    (1, 2),
                    (1, 4),
                    (1, 2),
                    ((1 << 20) | 456, 4),
                    ((1 << 20) | 789, 4),
                ]
            )
            files = {
                "uk.dat": bytes(person),
                "ukspos.idx": leaf_fixture([(bytes(4), 1)]),
                "d1.dat": fixed_table(
                    56, [place("FIXTURE COUNTY", "C"), place("FIXTURE TOWN", "T", 1)]
                ),
                "d2.dat": fixed_table(61, [b"EXAMPLE"]),
                "d3.dat": fixed_table(35, [b"ADA", b"BEA", b"CECIL"]),
                "d4.dat": fixed_table(16, [b"42"]),
                "d5.dat": b"\0FICTION HOUSE\0",
                "d6new.dat": fixed_table(
                    22,
                    [
                        b"ZZ991AA"
                        + (2).to_bytes(2, "little")
                        + (1).to_bytes(3, "little")
                        + bytes(10)
                    ],
                ),
                "d7.dat": b"\0INVENTED ROAD\0",
                "d8.dat": fixed_table(29, [b"FLAT 1"]),
                "bus.dat": business,
                "dd.dat": b"FICTIONAL\0",
                "dbsic.dat": fixed_table(5, [b"1234"]),
                "lobdesc.dat": b"\0INVENTED INDUSTRY\0",
                "telephone-prefixes.dat": b"01234\0",
            }
            for name, data in files.items():
                (root / name).write_bytes(data)
            edition = Edition(
                "v14-synthetic",
                "Invented v14",
                tuple((n, len(v), hashlib.sha256(v).hexdigest()) for n, v in files.items()),
                format="v14",
            )
            with patch("ukinfo_reader.editions.SUPPORTED_EDITIONS", (edition,)):
                with Database(root) as db:
                    with patch(
                        "ukinfo_reader.modern_reader.subscriber_text",
                        side_effect=AssertionError("legacy telephone formatter used for v14"),
                    ):
                        rows = list(db.records())
                    self.assertEqual(len(rows), 4)
                    phone = next(r for r in rows if r["entry_type"] == "telephone")
                    self.assertEqual(phone["phone"], "01234000123")
                    self.assertEqual(phone["phone_area_code"], 0)
                    self.assertEqual(phone["postcode_flags"], 0xC0)
                    self.assertTrue(phone["source_co1"])
                    self.assertEqual(phone["street"], "INVENTED ROAD")
                    self.assertEqual(phone["town"], "FIXTURE TOWN")
                    business = next(r for r in rows if r["entry_type"] == "business")
                    self.assertEqual(business["organisation"], "FICTIONAL")
                    self.assertEqual(business["industry"], "INVENTED INDUSTRY")
                    self.assertEqual(business["phone"], "01234000456")
                    self.assertEqual(business["fax"], "01234000789")
                    self.assertEqual(
                        len(list(db.search(surname="EXAMPLE", postcode="ZZ99 1AA"))), 3
                    )
                    self.assertEqual(
                        len(list(db.search(surname="EXAMPLE", postcode="ZZ99 2AA"))), 0
                    )
                    self.assertEqual(len(list(db.search(organisation="FICTIONAL"))), 1)
                import io
                import json

                from ukinfo_reader.modern_inspect import inspect
                from ukinfo_reader.search_export import emit

                with Database(root) as export_db:
                    output = io.StringIO()
                    emit(export_db, root, output, limit=4)
                    frames = [json.loads(line) for line in output.getvalue().splitlines()]
                    self.assertEqual(frames[0]["edition"], "v14-synthetic")
                    self.assertEqual(frames[-1]["count"], 4)
                    self.assertEqual(frames[3]["record"]["phone"], "01234000123")
                self.assertEqual(inspect(root)["totals"]["entries"], 4)
                report = validate(root)
                self.assertEqual(report["totals"]["entries"], 4)
                (root / "telephone-prefixes.dat").write_bytes(b"\xff1234\0")
                with self.assertRaises(FormatError):
                    Database(root)


class V14PrefixExtractionTests(unittest.TestCase):
    def test_extract_numeric_metadata_from_invented_pe_sections(self):
        import struct

        from ukinfo_reader.v14_media import read_prefixes

        data = bytearray(2048)
        data[:2] = b"MZ"
        struct.pack_into("<I", data, 0x3C, 0x80)
        data[0x80:0x84] = b"PE\0\0"
        struct.pack_into("<H", data, 0x86, 1)
        struct.pack_into("<H", data, 0x94, 0xE0)
        struct.pack_into("<H", data, 0x98, 0x10B)
        struct.pack_into("<I", data, 0x98 + 28, 0x10000000)
        section = 0x98 + 0xE0
        struct.pack_into("<IIII", data, section + 8, 1024, 0x1000, 1024, 0x400)
        struct.pack_into("<II", data, 0x400, 0x10001100, 0x10001110)
        data[0x500:0x506] = b"01234\0"
        data[0x510:0x516] = b"05678\0"
        self.assertEqual(read_prefixes(data, 0x1000, 2), b"01234\0" + b"05678\0")
        struct.pack_into("<I", data, 0x400, 0x20000000)
        with self.assertRaises(FormatError):
            read_prefixes(data, 0x1000, 2)
        with self.assertRaises(FormatError):
            read_prefixes(data[:100], 0x1000, 2)


class V14LocalityTests(unittest.TestCase):
    def test_shifted_source_ids_do_not_inherit_old_county_overrides(self):
        from types import SimpleNamespace

        from modern_fixtures import place

        from ukinfo_reader.v14 import V14Database

        # A source ID in the old override range now refers to another county.
        rows = {
            1: place("FIXTURE COUNTY", "C"),
            2: place("FIXTURE TOWN", "T", 1),
            3: place("ALTERNATE SOURCE COUNTY", "C"),
            0x25B9: place("INVENTED VILLAGE", "V", 3, 2),
        }
        db = object.__new__(V14Database)
        db.places = SimpleNamespace(raw=rows.__getitem__)
        result = db._location(0x25B9)
        self.assertEqual(result["county"], "FIXTURE COUNTY")
        self.assertEqual(result["town"], "FIXTURE TOWN")
        self.assertEqual(result["locality"], "INVENTED VILLAGE")
        self.assertIn("legacy_locality_override_ignored", result["place_warning"])
        self.assertEqual(result["county_source_id"], 3)
        self.assertEqual(result["resolved_county_source_id"], 1)
        self.assertEqual(result["town_source_id"], 2)

    def test_original_v14_island_locality_role_is_retained(self):
        from types import SimpleNamespace

        from modern_fixtures import place

        from ukinfo_reader.v14 import V14Database

        rows = {1: place("FIXTURE COUNTY", "C"), 0x4D83: place("INVENTED ISLAND", "T", 1)}
        db = object.__new__(V14Database)
        db.places = SimpleNamespace(raw=rows.__getitem__)
        row = db._location(0x4D83)
        self.assertEqual(row["locality"], "INVENTED ISLAND")
        self.assertEqual(row["town"], "")
        self.assertEqual(row["county"], "FIXTURE COUNTY")
        self.assertNotIn("legacy_locality_override_ignored", row["place_warning"])
