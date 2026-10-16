import os
import struct
import unittest

from fixtures import synthetic_database

from ukinfo_reader import Database, FormatError
from ukinfo_reader.codecs import dictionary, integers, postcode_bits, varint


class CodecTests(unittest.TestCase):
    def test_integer_families(self):
        # Independent encodings of the same absolute sequence.
        fixtures = [
            (0, b"\x02\x03\x00\x04"),
            (1, b"\x02\0\x03\0\0\0\x04\0"),
            (2, bytes.fromhex("82 83 80 84")),
            (3, bytes.fromhex("02 03 00 04")),
            (4, bytes.fromhex("02 00 03 00 00 00 04 00")),
            (5, bytes.fromhex("84 86 80 88")),
            (6, bytes.fromhex("02 05 c9 09")),
            (7, bytes.fromhex("02 00 05 00 e9 fd 09 00")),
            (8, bytes.fromhex("84 8a 83 92")),
            (9, bytes.fromhex("02 05 05 09")),
            (10, bytes.fromhex("02 00 05 00 05 00 09 00")),
            (11, bytes.fromhex("82 85 85 89")),
            (12, bytes.fromhex("81 82 85 80 81 81 01 89")),
        ]
        for encoding, payload in fixtures:
            with self.subTest(encoding=encoding):
                data = struct.pack("<IHI", 4, encoding, 21) + payload
                self.assertEqual(integers(data), ([2, 5, 5, 9], len(data)))

    def test_run_lengths_and_negative_delta(self):
        data = struct.pack("<IHI", 4, 3, 12) + bytes.fromhex("01 ca 03")
        self.assertEqual(integers(data)[0], [1, 2, 3, 6])

    def test_signed_delta_and_checksum(self):
        # The shipped writer emits overlong negative integers. The DLL wraps at 32 bits.
        token = b"\0" * 4 + b"\x7f" * 19 + b"\xf6"  # wraps to -10; arithmetic shift yields -5
        self.assertEqual(varint(token)[0], 0xFFFFFFF6)
        data = struct.pack("<IHI", 3, 5, 30) + bytes.fromhex("94") + token + bytes.fromhex("94")
        # Deltas 10,-5,10 -> absolute values 10,5,15.
        self.assertEqual(integers(data)[0], [10, 5, 15])
        broken = bytearray(data)
        broken[6] ^= 1
        with self.assertRaises(FormatError):
            integers(broken)

    def test_repeat_cannot_overrun_declared_length(self):
        data = struct.pack("<IHI", 1, 8, 0) + bytes.fromhex("7f 7f 7f 7f ff")
        with self.assertRaisesRegex(FormatError, "Repeat exceeds"):
            integers(data)

    def test_prefix_strings(self):
        data = b"\x83\x04ALBERT TERRACE\0\x06GARDENS\0BROAD STREET\0"
        words, kind, pos = dictionary(data)
        self.assertEqual(words, ["ALBERT TERRACE", "ALBERTGARDENS", "BROAD STREET"])
        self.assertEqual(pos, len(data))
        with self.assertRaises(FormatError):
            dictionary(b"\x81\x04\x07X\0")

    def test_bitmap(self):
        self.assertEqual(postcode_bits(bytes.fromhex("85 82 82 82 80 80")), [0, 1, 2])
        # Literal blocks use relative starts, independently of their lengths.
        self.assertEqual(postcode_bits(bytes.fromhex("83 81 01 02 00 82 04 00 80 80")), [0, 10])


class DatabaseTests(unittest.TestCase):
    def test_synthetic_full_validation_and_records(self):
        from ukinfo_reader.validation import validate

        with synthetic_database() as directory:
            report = validate(directory)
            self.assertEqual(
                report["totals"], dict(postcodes=1, localities=1, addresses=2, entries=3)
            )
            with Database(directory) as db:
                rows = list(db.records(0))
                self.assertEqual([r["firstName"] for r in rows], ["ADA", "BERT", ""])
                self.assertEqual([r["surname"] for r in rows], ["EXAMPLE", "EXAMPLE", "LAB"])
                self.assertEqual([r["houseno"] for r in rows], [10, 10, 20])
                self.assertEqual(rows[0]["household_id"], rows[1]["household_id"])
                self.assertNotEqual(rows[0]["household_id"], rows[2]["household_id"])
                self.assertEqual(rows[0]["postcode"], "ZZ1 1AA")
                self.assertEqual(rows[0]["street_1"], "SIDE LANE")
                self.assertEqual(rows[0]["street_2"], "TEST ROAD")
            db.close()

    def test_unknown_edition_and_fingerprint_failure(self):
        from ukinfo_reader.editions import UnsupportedEditionError, identify_edition

        with synthetic_database() as directory:
            edition = identify_edition(directory)
            path = directory / "ukid.dat"
            data = bytearray(path.read_bytes())
            data[0] ^= 1
            path.write_bytes(data)
            with self.assertRaises(UnsupportedEditionError):
                edition.verify(directory)
            path.write_bytes(b"wrong edition")
            with self.assertRaises(UnsupportedEditionError):
                Database(directory)

    def test_earlier_edition_is_not_mistaken_for_supported_data(self):
        import tempfile
        from pathlib import Path

        from ukinfo_reader.editions import UnsupportedEditionError, identify_edition

        with tempfile.TemporaryDirectory() as directory:
            for name in ("Uk1.idx", "Ukdemo.dat"):
                (Path(directory) / name).write_bytes(b"synthetic placeholder")
            with self.assertRaisesRegex(UnsupportedEditionError, "earlier format is unsupported"):
                identify_edition(directory)

    def test_cli_uses_only_synthetic_records(self):
        import csv
        import io
        import json
        from contextlib import redirect_stderr, redirect_stdout
        from unittest.mock import patch

        from ukinfo_reader.cli import main

        with synthetic_database() as directory:

            def run(*args):
                out = io.StringIO()
                err = io.StringIO()
                with (
                    patch("sys.argv", ["ukinfo-read", "--data", str(directory), *args]),
                    redirect_stdout(out),
                    redirect_stderr(err),
                ):
                    main()
                return out.getvalue()

            rows = [
                json.loads(line)
                for line in run("--surname", "example", "--first-name", "ada").splitlines()
            ]
            self.assertEqual(len(rows), 1)
            self.assertEqual(rows[0]["firstName"], "ADA")
            rows = list(csv.DictReader(io.StringIO(run("--postcode", "zz11aa", "--format", "csv"))))
            self.assertEqual(len(rows), 3)
            self.assertEqual(len(run("--all", "--limit", "1").splitlines()), 1)
            self.assertEqual(json.loads(run("--identify"))["edition"], "synthetic-test")
            for args in [
                ("--segment", "-1"),
                ("--field", "missing"),
                ("--segment", "0", "--field", "missing"),
            ]:
                with self.assertRaises(SystemExit):
                    run(*args)


@unittest.skipUnless(
    os.environ.get("UKINFO_TEST_DATA"), "set UKINFO_TEST_DATA for source-disc regression tests"
)
class DiscTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.db = Database(os.environ["UKINFO_TEST_DATA"])

    @classmethod
    def tearDownClass(cls):
        cls.db.close()

    def test_regression_sectors_and_joins(self):
        for segment in (0, 100, 951, 953, 9564):
            with self.subTest(segment=segment):
                stats = self.db.validate_segment(segment)
                self.assertGreater(stats["entries"], 0)
                row = next(self.db.records(segment))
                self.assertTrue(row["postcode"].startswith(self.db.sector(segment)))
                self.assertLess(max(self.db.column(segment, "houseno")), 100000)
        self.assertEqual(self.db.sector(100), "AL3 6")
        self.assertEqual(self.db.sector(9564), "ZE3 9")


if __name__ == "__main__":
    unittest.main()
