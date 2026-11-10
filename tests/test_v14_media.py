"""Invented raw CD sectors; no original image data."""

import hashlib
import struct
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from ukinfo_reader.codecs import FormatError
from ukinfo_reader.v14_media import extract_disc


def directory_record(lba, size, name, directory=False):
    length = 33 + len(name)
    if length % 2:
        length += 1
    record = bytearray(length)
    record[0] = length
    struct.pack_into("<I", record, 2, lba)
    struct.pack_into(">I", record, 6, lba)
    struct.pack_into("<I", record, 10, size)
    struct.pack_into(">I", record, 14, size)
    record[25] = 2 if directory else 0
    record[32] = len(name)
    record[33 : 33 + len(name)] = name
    return record


def raw_disc(stride, duplicate=False):
    image = bytearray(stride * 24)
    payload = b"invented database bytes"
    entry = directory_record(21, len(payload), b"UK.DAT;1")
    root = directory_record(20, 2048, b"\0", True)
    pvd = bytearray(2048)
    pvd[:7] = b"\x01CD001\x01"
    pvd[156:190] = root
    for lba, data in ((16, pvd), (20, entry + entry if duplicate else entry), (21, payload)):
        image[lba * stride + 24 : lba * stride + 24 + len(data)] = data
    return image, payload


class RawV14MediaTests(unittest.TestCase):
    def test_both_sector_sizes_extract_only_requested_database_files(self):
        for stride in (2352, 2448):
            with self.subTest(stride=stride), tempfile.TemporaryDirectory() as folder:
                root = Path(folder)
                image, payload = raw_disc(stride)
                source = root / "source.img"
                source.write_bytes(image)
                output = root / "output"
                output.mkdir()
                identity = (len(image), hashlib.sha256(image).hexdigest(), stride)
                with patch("ukinfo_reader.v14_media.progress"):
                    extract_disc(source, identity, ("uk.dat",), output)
                self.assertEqual((output / "uk.dat").read_bytes(), payload)
                self.assertEqual([p.name for p in output.iterdir()], ["uk.dat"])
                # Exclusive output creation prevents replacing an existing database.
                with self.assertRaises(FileExistsError), patch("ukinfo_reader.v14_media.progress"):
                    extract_disc(source, identity, ("uk.dat",), output)

    def test_mixed_or_altered_media_is_rejected_before_extracting(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            image, _ = raw_disc(2352)
            source = root / "source.img"
            source.write_bytes(image)
            with self.assertRaises(FormatError), patch("ukinfo_reader.v14_media.progress"):
                extract_disc(source, (len(image), "0" * 64, 2352), ("uk.dat",), root)
            self.assertFalse((root / "uk.dat").exists())

    def test_duplicate_and_missing_members_are_rejected(self):
        for duplicate, targets in ((True, ("uk.dat",)), (False, ("missing.dat",))):
            with self.subTest(duplicate=duplicate), tempfile.TemporaryDirectory() as folder:
                root = Path(folder)
                image, _ = raw_disc(2352, duplicate)
                source = root / "source.img"
                source.write_bytes(image)
                identity = (len(image), hashlib.sha256(image).hexdigest(), 2352)
                with self.assertRaises(FormatError), patch("ukinfo_reader.v14_media.progress"):
                    extract_disc(source, identity, targets, root)
