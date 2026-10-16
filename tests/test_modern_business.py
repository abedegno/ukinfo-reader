"""Business examples are invented and contain no source directory data."""

import csv
import io
import json
import unittest
from contextlib import redirect_stdout
from unittest.mock import patch

from modern_fixtures import modern_source

from ukinfo_reader import Database, FormatError
from ukinfo_reader.cli import main
from ukinfo_reader.modern import decode_business
from ukinfo_reader.modern_business import BusinessStream, validate_business
from ukinfo_reader.validation import validate


class BusinessTests(unittest.TestCase):
    def test_joins_deletions_updates_and_filters(self):
        with modern_source(business=True) as (root, _), Database(root) as db:
            rows = list(db.search_business(organisation="fictional works", postcode="zz99 1aa"))
            self.assertEqual(len(rows), 2)
            base, update = rows
            self.assertEqual(base["source_stream"], "base")
            self.assertEqual(update["source_stream"], "update")
            self.assertEqual(base["houseno"], "10")
            self.assertEqual(update["houseno"], "20")
            self.assertEqual(base["industry"], "INVENTED INDUSTRY")
            self.assertEqual(base["sic_code"], "1234")
            self.assertEqual(base["phone"], "01234 0123")
            self.assertEqual(base["fax"], "01234 7654321")
            self.assertEqual(base["building"], "EXAMPLE HOUSE")
            self.assertEqual(base["subbuilding"], "FLAT 1")
            self.assertEqual(base["town"], "FIXTURE TOWN")
            self.assertNotEqual(base["entry_id"], update["entry_id"])
            self.assertEqual(len(list(db.records())), 10)
            self.assertEqual(len(list(db.search(entry_type="business", sector="ZZ99 1"))), 2)
            self.assertFalse(list(db.search(organisation="absent")))
            self.assertFalse(list(db.search(surname="EXAMPLE", entry_type="business")))
            self.assertEqual(len(list(db.search(entry_type="electoral"))), 6)
            report = validate(root)
            self.assertEqual(report["business_streams"]["base"]["counts"]["entries"], 1)

    def test_truncation_and_high_bits(self):
        raw = bytes([4, 0x50, 1, 0, 128, 1, 0, 1, 0, 0, 1, 0, 0])
        record = decode_business(raw)
        self.assertEqual(record.house_id, 65537)
        self.assertEqual(record.building_reference, 16777217)
        for end in range(len(raw)):
            with self.assertRaises(FormatError):
                decode_business(raw[:end])
        with self.assertRaises(FormatError):
            decode_business(raw, -1)

    def test_csv_schema_and_business_cli(self):
        with modern_source(business=True) as (root, _):
            for fmt in ("csv", "jsonl"):
                output = io.StringIO()
                with (
                    patch(
                        "sys.argv", ["ukinfo-read", "--data", str(root), "--all", "--format", fmt]
                    ),
                    redirect_stdout(output),
                ):
                    main()
                rows = (
                    list(csv.DictReader(io.StringIO(output.getvalue())))
                    if fmt == "csv"
                    else [json.loads(line) for line in output.getvalue().splitlines()]
                )
                self.assertEqual(len(rows), 10)
                self.assertEqual(rows[-1]["organisation"], "FICTIONAL WORKS")
                self.assertEqual(set(rows[0]), set(rows[-1]))
            output = io.StringIO()
            with (
                patch(
                    "sys.argv",
                    ["ukinfo-read", "--data", str(root), "--organisation", "fictional works"],
                ),
                redirect_stdout(output),
            ):
                main()
            self.assertEqual(len(output.getvalue().splitlines()), 2)

    def test_industry_wrap_candidates_are_explicit_and_ambiguity_is_not_guessed(self):
        stream = BusinessStream.__new__(BusinessStream)
        stream.industry = bytearray(b"\0LOW\0" + b"x" * ((1 << 24) + 10))
        stream.industry[1 << 24] = 0
        stream.industry[(1 << 24) + 1 : (1 << 24) + 6] = b"HIGH\0"
        self.assertEqual(stream._industry_text(1), (None, None, "ambiguous_industry_reference"))
        stream.industry[(1 << 24) + 1] = 0
        self.assertEqual(
            stream._industry_text(2), ("IGH", (1 << 24) + 2, "industry_reference_wrap_recovered")
        )
        self.assertEqual(stream._industry_text(10), (None, None, "invalid_industry_reference"))

    def test_invalid_deletion_boundary_and_name_reference_are_rejected(self):
        with modern_source(business=True) as (root, _), Database(root) as db:
            stream = db.business_streams[0]
            stream.deleted.add(1)
            with self.assertRaisesRegex(FormatError, "record boundary"):
                validate_business(db)
            stream.words = b"X\0"
            stream.word.cache_clear()
            with self.assertRaisesRegex(FormatError, "dictionary string reference"):
                stream.organisation(next(stream.physical_records()))

    def test_2004_has_no_separate_business_payload(self):
        with modern_source(updated=False) as (root, _), Database(root) as db:
            self.assertEqual(list(db.search_business()), [])
