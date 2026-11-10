"""Invented source-policy examples; no directory records."""

import hashlib
import io
import json
import struct
import unittest
from dataclasses import replace
from types import SimpleNamespace
from unittest.mock import patch

from ukinfo_reader.hosted_selection import V14_CO1, get_selection
from ukinfo_reader.search_export import emit


class HostedSelectionTests(unittest.TestCase):
    def test_selection_is_exactly_scoped_to_v14_and_strict_about_flags(self):
        self.assertIsNone(get_selection("anything", None))
        with self.assertRaises(ValueError):
            get_selection("another-edition", "exclude-source-co1")
        with self.assertRaises(ValueError):
            get_selection("v14-2008-03", "unknown")
        for flag in (None, "", 0, 1, "false", "true"):
            with self.subTest(flag=flag), self.assertRaises(ValueError):
                V14_CO1.accept({"entry_type": "electoral", "source_co1": flag})
        self.assertTrue(V14_CO1.accept({"entry_type": "telephone", "source_co1": False}))
        self.assertFalse(V14_CO1.accept({"entry_type": "electoral", "source_co1": True}))
        self.assertTrue(V14_CO1.accept({"entry_type": "business", "source_co1": ""}))
        with self.assertRaises(ValueError):
            V14_CO1.accept({"entry_type": "business", "source_co1": True})
        with self.assertRaises(ValueError):
            V14_CO1.accept({"entry_type": "unknown", "source_co1": False})

    def test_published_counts_distinguish_source_and_selected_occurrences(self):
        self.assertEqual(V14_CO1.source_expected, 43799502)
        self.assertEqual(V14_CO1.excluded_expected, 54869)
        self.assertEqual(V14_CO1.expected, 43744633)
        self.assertEqual(
            V14_CO1.type_counts, {"electoral": 26922737, "telephone": 10729888, "business": 6092008}
        )

    def test_stream_has_explicit_policy_and_fresh_digest_for_only_selected_rows(self):
        rows = [
            {"entry_id": str(i), "entry_type": kind, "source_co1": flag}
            for i, (kind, flag) in enumerate(
                [
                    ("electoral", False),
                    ("electoral", True),
                    ("telephone", True),
                    ("telephone", False),
                    ("business", ""),
                ]
            )
        ]
        edition = SimpleNamespace(
            id="v14-2008-03", title="Invented v14", verify=lambda _: {"fixture": "a" * 64}
        )
        db = SimpleNamespace(edition=edition, format="modern", records=lambda: iter(rows))
        policy = replace(V14_CO1, source_counts=(2, 2, 1), excluded_counts=(1, 1, 0))
        out = io.StringIO()
        with patch("ukinfo_reader.search_export.get_selection", return_value=policy):
            emit(db, "unused-invented-folder", out, selection="exclude-source-co1")
        frames = [json.loads(line) for line in out.getvalue().splitlines()]
        self.assertEqual(frames[0]["dataset"], "v14-2008-03:co1-excluded")
        self.assertEqual(frames[0]["expected"], 3)
        self.assertEqual(frames[0]["selection"]["excluded_expected"], 2)
        self.assertFalse(frames[0]["sample"])
        kept = frames[1:-1]
        self.assertEqual([f["source_id"] for f in kept], ["0", "3", "4"])
        digest = hashlib.sha256()
        for line in out.getvalue().splitlines()[1:-1]:
            frame = json.loads(line)
            identity = frame["source_id"].encode()
            raw = json.dumps(frame["record"], ensure_ascii=True, separators=(",", ":")).encode()
            digest.update(struct.pack("<HI", len(identity), len(raw)))
            digest.update(identity)
            digest.update(raw)
        self.assertEqual(frames[-1], {"type": "end", "count": 3, "sha256": digest.hexdigest()})
        # A raw-source or suppression-count mismatch must never emit a valid terminal frame.
        for short in (rows[:-1], rows[:1] + rows[2:]):
            db.records = lambda: iter(short)
            out = io.StringIO()
            with (
                patch("ukinfo_reader.search_export.get_selection", return_value=policy),
                self.assertRaises(ValueError),
            ):
                emit(db, "unused", out, selection="exclude-source-co1")
            self.assertNotIn('"type":"end"', out.getvalue())
