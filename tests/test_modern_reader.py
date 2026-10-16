import csv
import io
import json
import unittest
from contextlib import redirect_stderr, redirect_stdout
from dataclasses import replace
from unittest.mock import patch

from modern_fixtures import modern_source, place

from ukinfo_reader import Database, FormatError
from ukinfo_reader.cli import main
from ukinfo_reader.modern import decode_residence
from ukinfo_reader.modern_reader import location, subscriber_text
from ukinfo_reader.validation import validate


class ModernReaderTests(unittest.TestCase):
    def test_combined_records_deletion_keeps_electoral_names_and_update_dictionaries(self):
        with modern_source() as (root, offsets), Database(root) as db:
            rows = list(db.records())
            electoral = [r for r in rows if r["entry_type"] == "electoral"]
            telephone = [r for r in rows if r["entry_type"] == "telephone"]
            self.assertEqual(len(electoral), 6)
            self.assertEqual(len(telephone), 2)
            self.assertEqual(
                {r["record_offset"] for r in electoral}, {offsets[i] for i in (0, 2, 3, 4)}
            )
            self.assertEqual([r["phone"] for r in telephone], ["01234 1234567", "01234 987654"])
            self.assertEqual(telephone[1]["firstName"], "UPDATED ADA")
            self.assertEqual(telephone[1]["houseno"], "20")
            self.assertEqual(telephone[1]["source_stream"], "update")
            self.assertEqual(len({r["entry_id"] for r in rows}), len(rows))
            self.assertIn("building_reference_inside_string", electoral[0]["source_warning"])
            self.assertIsNone(electoral[0]["building"])
            self.assertEqual(electoral[0]["building_raw"], "AMPLE HOUSE")
            report = validate(root)
            self.assertEqual(report["totals"]["entries"], len(rows))
            self.assertEqual(report["streams"]["base"]["counts"]["deleted_telephone_records"], 2)
            self.assertEqual(report["streams"]["update"]["counts"]["unindexed_physical_records"], 1)

    def test_address_components_precision_filters_and_2004_building_fixes(self):
        with modern_source(updated=False) as (root, _), Database(root) as db:
            rows = list(db.search(surname="example", first_name="ada", postcode="zz99 1aa"))
            self.assertTrue(rows)
            row = rows[0]
            self.assertEqual(
                (row["locality"], row["town"], row["county"]),
                ("FIXTURE VILLAGE", "FIXTURE TOWN", "FIXTURE COUNTY"),
            )
            self.assertEqual(row["street"], "FICTION ROAD")
            self.assertEqual(row["building"], "FIXED HOUSE")
            self.assertEqual(row["subbuilding"], "FLAT 1")
            self.assertEqual(row["houseno"], "10")
            self.assertEqual(len(list(db.search(surname="absent"))), 0)
            self.assertTrue(list(db.search(surname="example", sector="ZZ99 1")))
            partial = [r for r in db.records() if r["postcode_precision"]]
            self.assertEqual(partial[0]["postcode"], "ZZ991A")
            self.assertEqual(partial[0]["street"], "")
            self.assertNotIn(partial[0]["entry_id"], {r["entry_id"] for r in rows})
            phone = list(db.search(first_name="cecil"))
            self.assertIn("01234 0123", {r["phone"] for r in phone})
            report = validate(root)
            self.assertEqual(report["streams"]["base"]["counts"]["building_corrections_applied"], 1)

    def test_cli_csv_json_schema_and_segment_rejection(self):
        with modern_source() as (root, _):
            for fmt in ("csv", "jsonl"):
                output = io.StringIO()
                with (
                    patch(
                        "sys.argv",
                        [
                            "ukinfo-read",
                            "--data",
                            str(root),
                            "--surname",
                            "EXAMPLE",
                            "--limit",
                            "2",
                            "--format",
                            fmt,
                        ],
                    ),
                    redirect_stdout(output),
                ):
                    self.assertIsNone(main())
                rows = (
                    list(csv.DictReader(io.StringIO(output.getvalue())))
                    if fmt == "csv"
                    else [json.loads(x) for x in output.getvalue().splitlines()]
                )
                self.assertEqual(len(rows), 2)
            output = io.StringIO()
            with (
                patch("sys.argv", ["ukinfo-read", "--data", str(root), "--schema"]),
                redirect_stdout(output),
            ):
                self.assertIsNone(main())
            self.assertIn(
                "entry_type", [f["name"] for f in json.loads(output.getvalue())["fields"]]
            )
            with (
                patch("sys.argv", ["ukinfo-read", "--data", str(root), "--segment", "0"]),
                redirect_stderr(io.StringIO()),
            ):
                with self.assertRaises(SystemExit) as caught:
                    main()
                self.assertEqual(caught.exception.code, 2)

    def test_subscriber_zero_encoding(self):
        self.assertEqual(subscriber_text(10000123), "0123")
        self.assertEqual(subscriber_text(11000123), "00123")
        self.assertEqual(subscriber_text(9999999), "9999999")
        self.assertEqual(subscriber_text(0xFFFFFF), "0000000777215")
        self.assertEqual(subscriber_text(-1), "0000000")
        self.assertEqual(subscriber_text(10000123, legacy=True), "0123")
        self.assertEqual(subscriber_text(11000123, legacy=True), "11000123")

    def test_precision_limits_and_out_of_range_building_are_explicit(self):
        with modern_source() as (root, offsets), Database(root) as db:
            record = decode_residence(db.streams[0].data, offsets[0])
            partial = replace(record, optional_pc80=9)
            rows = list(db._joined(0, offsets[0], 1, partial))
            self.assertEqual(rows[0]["postcode"], "ZZ991A")
            self.assertEqual(
                (rows[0]["locality"], rows[0]["town"], rows[0]["county"]), ("", "", "")
            )
            full = replace(record, optional_pc80=10, optional_flag80=999999)
            rows = list(db._joined(0, offsets[0], 1, full))
            self.assertEqual(rows[0]["street"], "FICTION ROAD")
            self.assertIsNone(rows[0]["building"])
            self.assertIsNone(rows[0]["building_raw"])
            self.assertIn("building_reference_out_of_range", rows[0]["source_warning"])
            self.assertNotIn("partial_postcode_source", rows[0]["source_warning"])

    def test_deleting_a_nontelephone_record_is_rejected(self):
        with modern_source() as (root, offsets), Database(root) as db:
            db.streams[0].deleted.add(offsets[0])
            from ukinfo_reader.modern_validation import validate_database

            with self.assertRaisesRegex(FormatError, "without a telephone"):
                validate_database(db, root)

    def test_undefined_dictionary_bytes_are_preserved_as_flagged_replacements(self):
        with modern_source(undefined_name=True) as (root, _), Database(root) as db:
            row = next(db.records())
            self.assertEqual(row["firstName"], "\ufffdDA")
            self.assertIn("invalid_dictionary_text", row["source_warning"])
            report = validate(root)
            self.assertEqual(report["streams"]["base"]["invalid_dictionary_text_rows"], {"name": 1})

    def test_village_county_comes_from_its_town_and_unknown_type_fails(self):
        class Table:
            rows = {
                1: place("COUNTY", "C"),
                2: place("TOWN", "T"),
                3: place("VILLAGE", "V", 1, 2),
                4: place("UNKNOWN", "X"),
            }

            def raw(self, source_id):
                return self.rows[source_id]

        self.assertEqual(location(Table(), 3)["county"], "")
        with self.assertRaises(FormatError):
            location(Table(), 4)


if __name__ == "__main__":
    unittest.main()
