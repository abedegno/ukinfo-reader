"""Interchange provenance and failure semantics; all records here are invented."""

import io
import json
import unittest
from types import SimpleNamespace

from ukinfo_reader.search_export import emit, records, source_id


class FakeDB:
    format = "modern"
    edition = SimpleNamespace(
        id="invented", title="Synthetic fixture", verify=lambda _: {"fixture": "invented-sha"}
    )

    def records(self):
        yield {"entry_id": "base:12:0", "surname": "Li", "building": None}
        yield {"entry_id": "business:update:8", "entry_type": "business"}


class SearchExportTests(unittest.TestCase):
    def test_lossless_frames_and_sample_label(self):
        output = io.StringIO()
        emit(FakeDB(), "unused", output, limit=2)
        frames = [json.loads(line) for line in output.getvalue().splitlines()]
        self.assertEqual(frames[0]["dataset"], "invented:sample:2")
        self.assertTrue(frames[0]["sample"])
        self.assertIsNone(frames[1]["record"]["building"])
        self.assertEqual(frames[2]["source_id"], "business:update:8")
        self.assertEqual(frames[-1], {"type": "end", "count": 2})

    def test_short_stream_has_no_terminal_frame(self):
        output = io.StringIO()
        with self.assertRaisesRegex(ValueError, "Decoded 2 entries; expected 3"):
            emit(FakeDB(), "unused", output, limit=3)
        self.assertNotIn('"type":"end"', output.getvalue())

    def test_segment_qualified_identity_and_iteration(self):
        db = SimpleNamespace(
            format="se4",
            segment_count=2,
            records=lambda segment: iter([dict(segment_id=segment, entry_id=0)]),
        )
        self.assertEqual([source_id(row) for row in records(db)], ["0:0", "1:0"])

    def test_hash_failure_precedes_manifest(self):
        db = FakeDB()
        db.edition = SimpleNamespace(verify=lambda _: (_ for _ in ()).throw(ValueError("hash")))
        output = io.StringIO()
        with self.assertRaisesRegex(ValueError, "hash"):
            emit(db, "unused", output, limit=1)
        self.assertEqual(output.getvalue(), "")
