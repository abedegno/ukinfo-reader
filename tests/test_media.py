"""Invented ISO/7z fixtures exercise extraction, path handling and cleanup."""

import hashlib
import io
import shutil
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

try:
    import py7zr
    import pycdlib
except ImportError:
    py7zr = pycdlib = None

from ukinfo_reader.editions import Edition
from ukinfo_reader.media import BoundedWriter, candidates, prepare


@unittest.skipIf(pycdlib is None or py7zr is None, "install .[media]")
class MediaTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        self.content = b"Invented database bytes only."
        self.edition = Edition(
            "invented",
            "Invented",
            (("table.dat", len(self.content), hashlib.sha256(self.content).hexdigest()),),
        )
        self.profile = patch("ukinfo_reader.media.SUPPORTED_EDITIONS", (self.edition,))
        self.profile.start()
        self.space = patch(
            "ukinfo_reader.media.shutil.disk_usage",
            return_value=shutil._ntuple_diskusage(100 << 30, 0, 100 << 30),
        )
        self.space.start()

    def tearDown(self):
        self.space.stop()
        self.profile.stop()
        self.temp.cleanup()

    def image(self):
        image = self.root / "fixture.iso"
        iso = pycdlib.PyCdlib()
        iso.new()
        iso.add_fp(io.BytesIO(self.content), len(self.content), iso_path="/TABLE.DAT;1")
        iso.write(str(image))
        iso.close()
        return image

    def test_iso_canonical_names_and_hashes(self):
        target = self.root / "result"
        self.assertEqual(prepare(self.image(), target), self.edition)
        self.assertEqual(list(p.name for p in target.iterdir()), ["table.dat"])
        self.assertEqual((target / "table.dat").read_bytes(), self.content)
        with self.assertRaises(FileExistsError):
            prepare(self.image(), target)

    def test_7z_nested_iso_and_direct_files(self):
        for nested in (False, True):
            archive = self.root / f"fixture-{nested}.7z"
            with py7zr.SevenZipFile(archive, "w") as output:
                if nested:
                    output.write(self.image(), arcname="cover/disc.iso")
                else:
                    output.writestr(self.content, "archive-only/TABLE.DAT")
                    output.writestr(b"never extracted", "unrelated.exe")
            target = self.root / f"result-{nested}"
            self.assertEqual(prepare(archive, target), self.edition)
            self.assertEqual([p.name for p in target.iterdir()], ["table.dat"])
            self.assertFalse((self.root / "unrelated.exe").exists())

    def test_wrong_fingerprint_and_low_disk_clean_temporary_output(self):
        image = self.image()
        bad = Edition("invented", "Invented", (("table.dat", len(self.content), "0" * 64),))
        target = self.root / "bad"
        with patch("ukinfo_reader.media.SUPPORTED_EDITIONS", (bad,)):
            with self.assertRaisesRegex(ValueError, "SHA-256"):
                prepare(image, target)
        self.assertFalse(target.exists())
        with patch(
            "ukinfo_reader.media.shutil.disk_usage",
            return_value=shutil._ntuple_diskusage(10 << 30, 6 << 30, 4 << 30),
        ):
            with self.assertRaisesRegex(ValueError, "5 GiB"):
                prepare(image, target)
        self.assertFalse(target.exists())

    def test_ambiguous_and_truncated_members_rejected(self):
        with self.assertRaisesRegex(ValueError, "Ambiguous"):
            candidates([("/TABLE.DAT", len(self.content)), ("/table.dat", len(self.content))])
        with self.assertRaisesRegex(ValueError, "found 2"):
            candidates([("/a/table.dat", len(self.content)), ("/b/table.dat", len(self.content))])
        writer = BoundedWriter(self.root / "bounded", 3)
        try:
            with self.assertRaisesRegex(ValueError, "declared size"):
                writer.write(b"four")
        finally:
            writer.close()
