"""Pinned build-source downloads recover from transport failures, never bad hashes."""

import hashlib
import io
import tempfile
import unittest
import urllib.error
from pathlib import Path
from unittest.mock import patch

from tools import build_cabinet_runtime as cabinet


class BuildDownloadTests(unittest.TestCase):
    def test_dns_failure_uses_official_fallback_with_same_hash(self):
        payload = b"invented source archive"
        with tempfile.TemporaryDirectory() as directory:
            archive = Path(directory) / "source.tar.gz"
            with (
                patch.object(cabinet, "SHA256", hashlib.sha256(payload).hexdigest()),
                patch.object(
                    cabinet.urllib.request,
                    "urlopen",
                    side_effect=[
                        urllib.error.URLError("temporary DNS failure"),
                        io.BytesIO(payload),
                    ],
                ) as request,
                patch.object(cabinet.time, "sleep") as pause,
            ):
                cabinet.download_source(archive)
            self.assertEqual(archive.read_bytes(), payload)
            self.assertIn("codeload.github.com", request.call_args.args[0])
            pause.assert_called_once_with(1)

    def test_bad_hash_is_never_retried_or_accepted(self):
        with tempfile.TemporaryDirectory() as directory:
            with (
                patch.object(
                    cabinet.urllib.request, "urlopen", return_value=io.BytesIO(b"wrong archive")
                ) as request,
                patch.object(cabinet.time, "sleep") as pause,
            ):
                with self.assertRaisesRegex(RuntimeError, "hash mismatch"):
                    cabinet.download_source(Path(directory) / "source.tar.gz")
            self.assertEqual(request.call_count, 1)
            pause.assert_not_called()

    def test_transport_failures_are_bounded_and_partial_file_removed(self):
        with tempfile.TemporaryDirectory() as directory:
            archive = Path(directory) / "source.tar.gz"
            archive.write_bytes(b"partial")
            with (
                patch.object(
                    cabinet.urllib.request, "urlopen", side_effect=urllib.error.URLError("offline")
                ) as request,
                patch.object(cabinet.time, "sleep"),
            ):
                with self.assertRaisesRegex(RuntimeError, "four attempts"):
                    cabinet.download_source(archive)
            self.assertEqual(request.call_count, 4)
            self.assertFalse(archive.exists())
