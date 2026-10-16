import csv
import io
import json
import os
import struct
import unittest
from contextlib import redirect_stderr, redirect_stdout
from itertools import islice
from unittest.mock import patch

from pcf78_fixtures import synthetic_pcf78

from ukinfo_reader import Database, FormatError
from ukinfo_reader.cli import main
from ukinfo_reader.ctree import LeafIndex
from ukinfo_reader.validation import validate


class EarlyDatabaseTests(unittest.TestCase):
    def test_all_flags_wrapped_length_joins_and_index_membership(self):
        with synthetic_pcf78() as directory:
            report = validate(directory)
            self.assertEqual(report["totals"]["physical_records"], 11)
            self.assertEqual(report["totals"]["indexed_records"], 10)
            self.assertEqual(report["totals"]["unindexed_records"], 1)
            self.assertEqual(report["totals"]["entries"], 24)
            with Database(directory) as db:
                rows = list(db.records())
                self.assertEqual(len(rows), 24)
                self.assertIsNone(rows[0]["surname"])
                self.assertEqual(rows[0]["source_warning"], "invalid_surname_reference")
                self.assertEqual(rows[-1]["building"], "TEST LAB")
                self.assertEqual(rows[-1]["houseno"], "20A")
                self.assertEqual(rows[-1]["phone"], "01234 567890")
                self.assertEqual(rows[-1]["locality"], "FIXTURE VILLAGE")
                self.assertEqual(rows[-1]["town"], "TEST TOWN")
                self.assertEqual(rows[-1]["county"], "FIXTURE COUNTY")
                self.assertEqual(rows[-1]["street"], "TEST ROAD")
                self.assertEqual(rows[-1]["postcode"], "ZZ1 1AA")
                self.assertEqual(rows[-1]["postcode_raw"], "ZZ1 1AA")
                self.assertEqual(
                    [r["firstName"] for r in rows[-7:]],
                    ["ADA", "BERT", "CECIL", "ADA", "BERT", "CECIL", "ADA"],
                )
                self.assertEqual(len(list(db.search(surname="example"))), 23)
                self.assertEqual(
                    len(
                        list(
                            db.search(
                                surname="example",
                                first_name="cecil",
                                postcode="zz11aa",
                                sector="ZZ1 1",
                            )
                        )
                    ),
                    2,
                )
                self.assertEqual(list(db.search(surname="lab")), [])
                self.assertEqual(list(db.search(sector="ZZ1 2")), [])
                self.assertEqual(db._location(5)["town"], "OTHER TOWN")
                self.assertEqual(db._location(5)["county"], "")
            db.close()

    def test_bad_links_prefixes_lengths_references_and_pointers(self):
        with synthetic_pcf78() as directory:
            path = directory / "Uk1.idx"
            data = bytearray(path.read_bytes())
            data[2048 + 18] = 10
            with self.assertRaises(FormatError):
                LeafIndex(data, 9)
            data = bytearray(path.read_bytes())
            struct.pack_into("<I", data, 2048, 2048)
            with self.assertRaises(FormatError):
                LeafIndex(data, 9)
            with Database(directory) as db:
                with self.assertRaises(FormatError):
                    db.decode_record(0)
                with self.assertRaises(FormatError):
                    db.decode_record(len(db.dat) - 2)
            path = directory / "Ukdemo.dat"
            original = path.read_bytes()
            for position, value in [(19, 0), (17, 255), (22, 255)]:
                data = bytearray(original)
                data[position] = value
                path.write_bytes(data)
                with Database(directory) as db:
                    with self.assertRaises(FormatError):
                        db.decode_record(12)
            path.write_bytes(original)
            with Database(directory) as db:
                with self.assertRaisesRegex(FormatError, "disagrees"):
                    # A changed key cannot silently join to an unrelated source record.
                    with patch.object(db.index, "keys", return_value=iter([(b"wrong", 12)])):
                        list(db.records())

    def test_cli_filters_schema_and_edition_specific_options(self):
        with synthetic_pcf78() as directory:

            def run(*args):
                output = io.StringIO()
                with (
                    patch("sys.argv", ["ukinfo-read", "--data", str(directory), *args]),
                    redirect_stdout(output),
                    redirect_stderr(io.StringIO()),
                ):
                    main()
                return output.getvalue()

            rows = [
                json.loads(line)
                for line in run(
                    "--surname", "example", "--first-name", "ada", "--limit", "2"
                ).splitlines()
            ]
            self.assertEqual(len(rows), 2)
            self.assertEqual(rows[0]["surname"], "EXAMPLE")
            exported = list(
                csv.DictReader(io.StringIO(run("--all", "--format", "csv", "--limit", "1")))
            )
            self.assertEqual(exported[0]["surname"], "")
            self.assertEqual(exported[0]["source_warning"], "invalid_surname_reference")
            self.assertEqual(json.loads(run("--schema"))["edition"], "synthetic-pcf78")
            self.assertEqual(json.loads(run("--identify"))["edition"], "synthetic-pcf78")
            with self.assertRaises(SystemExit):
                run("--segment", "0")


class SourceValueTests(unittest.TestCase):
    def test_nonstandard_postcode_is_preserved_and_flagged(self):
        with synthetic_pcf78() as directory:
            path = directory / "d6.dat"
            data = bytearray(path.read_bytes())
            start = struct.unpack_from("<H", data, 12)[0] + 32
            data[start + 3 : start + 10] = b",BAD\0\0\0"
            data[start + 10 : start + 12] = b"\x81\xfe"  # codes are not postcode text
            path.write_bytes(data)
            with Database(directory) as db:
                row = next(db.records())
                self.assertEqual(row["postcode"], ",BAD")
                self.assertEqual(row["postcode_raw"], ",BAD")
                self.assertEqual(
                    row["source_warning"], "invalid_surname_reference;nonstandard_postcode"
                )


@unittest.skipUnless(
    os.environ.get("UKINFO_TEST_DATA_78"),
    "set UKINFO_TEST_DATA_78 for early source-disc regression tests",
)
class EarlyDiscTests(unittest.TestCase):
    def test_source_index_anomalies_and_filtered_joins(self):
        with Database(os.environ["UKINFO_TEST_DATA_78"]) as db:
            self.assertEqual(db.index.count, 19018492)
            self.assertEqual(db.buildings.count, 307914)
            rows = list(islice(db.records(), 3))
            self.assertEqual(len(rows), 3)
            self.assertTrue(all(row["surname"] is None for row in rows))
            self.assertTrue(
                all("invalid_surname_reference" in row["source_warning"] for row in rows)
            )
            rows = list(islice(db.search(surname="Smith", first_name="John"), 20))
            self.assertEqual(len(rows), 20)
            self.assertTrue(
                all(
                    row["surname"].casefold() == "smith" and row["firstName"].casefold() == "john"
                    for row in rows
                )
            )
            self.assertTrue(
                all(db.decode_record(row["record_offset"])["length"] >= 11 for row in rows)
            )
