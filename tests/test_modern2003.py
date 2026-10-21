"""Invented fixtures for the independently verified 2003 format differences."""

import hashlib
import io
import json
import struct
import tempfile
import unittest
from contextlib import contextmanager
from pathlib import Path
from unittest.mock import patch

from modern_fixtures import fixed_table, place, residence
from test_modern import leaf_fixture

from ukinfo_reader import Database, FormatError
from ukinfo_reader.editions import Edition
from ukinfo_reader.modern import ModernLeafIndex
from ukinfo_reader.modern2003 import decode_business_2003, decode_residence_2003
from ukinfo_reader.validation import validate


@contextmanager
def old_source(first_prefix=b"ZZ991"):
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        first = residence(4, [1, 999], building=1, subbuilding=1, phone=10000123, area=1234)
        second = bytearray(residence(3, [2], phone=4567))
        second[4:7] = (2).to_bytes(3, "little")

        def postcode(prefix, suffix, street):
            return (
                prefix.to_bytes(3, "little")
                + suffix
                + bytes(6)
                + street.to_bytes(3, "little")
                + bytes(4)
            )

        def prefix(code, locality):
            return code.ljust(5, b"\0") + b"0000" + locality.to_bytes(2, "little") + bytes(4)

        business = bytes([255, 1]) + (1).to_bytes(3, "little") + (1).to_bytes(2, "little")
        for value, width in [
            (1, 3),
            (11, 3),
            (27, 3),
            (1, 2),
            (1, 3),
            (1, 2),
            (10000123, 3),
            (1234, 2),
            (1234, 3),
            (7654321, 3),
        ]:
            business += value.to_bytes(width, "little")
        files = {
            "uk.dat": first + second,
            "ukspos.idx": leaf_fixture([(bytes(4), 1)], page_size=1024),
            "d1.dat": fixed_table(
                56,
                [
                    place("FIXTURE COUNTY", "C"),
                    place("FIXTURE TOWN", "T", 1),
                    place("FIXTURE VILLAGE", "V", 1, 2),
                ],
            ),
            "d2.dat": fixed_table(24, [b"EXAMPLE"]),
            "d3.dat": fixed_table(21, [b"ADA", b"BEA"]),
            "d4.dat": fixed_table(10, [b"42"]),
            "d6.dat": fixed_table(18, [postcode(1, b"AA", 1), postcode(2, bytes(2), 0)]),
            "d6pa.dat": fixed_table(15, [prefix(first_prefix, 3), prefix(b"ZZ99", 0xFFFF)]),
            "d5.dat": b"\0FICTION HOUSE\0",
            "d7.dat": b"\0INVENTED ROAD\0",
            "d8.dat": fixed_table(10, [b"FLAT 1"]),
            "dbbus.dat": business,
            "dd.dat": b"FICTIONAL\0WORKS\0",
            "dbsic.dat": fixed_table(6, [b"12345"]),
        }
        for name, content in files.items():
            (root / name).write_bytes(content)
        edition = Edition(
            "se2003-synthetic",
            "Invented 2003 source",
            tuple((n, len(v), hashlib.sha256(v).hexdigest()) for n, v in files.items()),
            format="modern2003",
        )
        with patch("ukinfo_reader.editions.SUPPORTED_EDITIONS", (edition,)):
            yield root


class Modern2003Tests(unittest.TestCase):
    def test_house_extension_does_not_consume_a_precision_byte_or_extend_building(self):
        data = bytearray(residence(0, [1], building=1))
        data[6] |= 0xC0
        data[7:9] = (2).to_bytes(2, "little")
        record = decode_residence_2003(data)
        self.assertEqual(record.length, len(data))
        self.assertEqual(record.house_id, 65538)
        self.assertEqual(record.optional_flag80, 1)
        self.assertIsNone(record.optional_pc80)
        with self.assertRaises(FormatError):
            decode_residence_2003(data[:-1])

    def test_business_control_high_bits_do_not_add_bytes_or_extend_building(self):
        # Two words, a building offset and no precision byte in this release.
        data = (
            bytes([4, 0xC1])
            + (1).to_bytes(3, "little")
            + (1).to_bytes(2, "little")
            + (1).to_bytes(3, "little")
            + (11).to_bytes(3, "little")
            + (9).to_bytes(3, "little")
        )
        record = decode_business_2003(data)
        self.assertEqual(record.length, len(data))
        self.assertEqual(record.building_reference, 9)
        self.assertEqual(record.precision, 0)
        with self.assertRaises(FormatError):
            decode_business_2003(data[:-1])

    def test_one_kib_leaf_pages_keep_page_chain_checks(self):
        entries = [(b"ABCD", 1), (b"ABCE", 2), (b"ABCF", 3)]
        for compression in (4, 12):
            raw = leaf_fixture(entries, compression, split=1, page_size=1024)
            self.assertEqual(list(ModernLeafIndex(raw).keys()), entries)
            struct.pack_into("<I", raw, 2048, 2048)
            with self.assertRaises(FormatError):
                list(ModernLeafIndex(raw).keys())

    def test_special_full_postcode_is_spaced_without_inventing_partial_precision(self):
        with old_source(b"GIR0") as root, Database(root) as db:
            row = next(db.search(entry_type="electoral"))
            self.assertEqual(row["postcode"], "GIR 0AA")
            self.assertNotIn("partial_postcode_source", row["source_warning"])

    def test_spaced_source_prefixes_keep_full_precision_and_raw_text(self):
        for prefix, displayed, raw in (
            (b"Z1 1", "Z1 1AA", "Z1 1AA"),
            (b"ZZ1 1", "ZZ1 1AA", "ZZ1 1AA"),
            (b"GIR 0", "GIR 0AA", "GIR 0AA"),
        ):
            with self.subTest(prefix=prefix), old_source(prefix) as root, Database(root) as db:
                row = next(db.search(entry_type="electoral"))
                self.assertEqual(row["postcode"], displayed)
                self.assertEqual(row["postcode_raw"], raw)
                self.assertNotIn("partial_postcode_source", row["source_warning"])
                business = next(db.search(entry_type="business"))
                self.assertEqual(business["postcode"], displayed)
                self.assertNotIn("partial_postcode_source", business["source_warning"])

    def test_catalogue_protocol_preserves_both_entry_types_and_terminal_count(self):
        from ukinfo_reader.search_export import emit

        with old_source() as root, Database(root) as db:
            output = io.StringIO()
            emit(db, root, output, limit=5)
            frames = [json.loads(line) for line in output.getvalue().splitlines()]
            self.assertEqual(frames[0]["expected"], 5)
            self.assertEqual(frames[0]["edition"], "se2003-synthetic")
            records = [frame for frame in frames if frame["type"] == "record"]
            self.assertEqual(len({frame["source_id"] for frame in records}), 5)
            self.assertEqual(
                {frame["record"]["entry_type"] for frame in records},
                {"business", "electoral", "telephone"},
            )
            self.assertEqual(frames[-1]["type"], "end")
            self.assertEqual(frames[-1]["count"], 5)

    def test_prefix_join_partial_postcode_source_anomalies_and_businesses(self):
        with old_source() as root, Database(root) as db:
            rows = list(db.records())
            self.assertEqual(len(rows), 5)
            electoral = [row for row in rows if row["entry_type"] == "electoral"]
            self.assertEqual(electoral[0]["postcode"], "ZZ99 1AA")
            self.assertEqual(electoral[0]["postcode_x_raw"], 0)
            self.assertEqual(electoral[0]["postcode_flags"], 0x20)
            self.assertEqual(
                (electoral[0]["street"], electoral[0]["town"], electoral[0]["county"]),
                ("INVENTED ROAD", "FIXTURE TOWN", "FIXTURE COUNTY"),
            )
            self.assertEqual(electoral[1]["firstName"], "")
            self.assertEqual(electoral[1]["name_code"], 999)
            self.assertIn("invalid_name_reference", electoral[1]["source_warning"])
            partial = [row for row in rows if row["postcode_raw"] == "ZZ99"][0]
            self.assertEqual(partial["postcode"], "ZZ99")
            self.assertEqual(partial["place_id"], 65535)
            self.assertIn("missing_source_locality", partial["source_warning"])
            self.assertEqual(len(list(db.search(postcode="ZZ99 1AA"))), 4)
            businesses = list(db.search(organisation="FICTIONAL WORKS"))
            self.assertEqual(len(businesses), 1)
            self.assertEqual(businesses[0]["phone"], "01234 0123")
            self.assertEqual(businesses[0]["sic_code"], "12345")
            self.assertIsNone(businesses[0]["industry"])
            self.assertEqual(businesses[0]["industry_reference_raw"], 27)
            self.assertIn("industry_dictionary_missing", businesses[0]["source_warning"])
            report = validate(root)
            self.assertEqual(report["totals"]["entries"], 5)
            self.assertEqual(
                report["streams"]["base"]["source_warnings"]["invalid_name_reference"], 1
            )
