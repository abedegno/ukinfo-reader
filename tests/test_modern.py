"""Invented binary fixtures; no bytes copied from source media."""

import hashlib
import io
import json
import struct
import tempfile
import unittest
from contextlib import redirect_stderr, redirect_stdout
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

from ukinfo_reader import FormatError, UnsupportedEditionError
from ukinfo_reader.editions import Edition
from ukinfo_reader.modern import (
    ModernFixedTable,
    ModernLeafIndex,
    decode_residence,
    residential_length,
)
from ukinfo_reader.modern_inspect import inspect, inspect_stream
from ukinfo_reader.modern_inspect import main as inspect_main


def leaf_fixture(entries, compression=12, split=None):
    size, width = 4096, 4
    data = bytearray(size * 4)
    struct.pack_into("<I", data, 24, len(entries))
    struct.pack_into("<H", data, 42, size)
    struct.pack_into("<H", data, 66, width)
    struct.pack_into("<I", data, 72, size * 2)
    data[57], data[60], data[64] = compression, 32, 1
    pages = [entries] if split is None else [entries[:split], entries[split:]]
    for page_number, records in enumerate(pages, 2):
        payload = bytearray()
        old = bytes(width)
        for key, pointer in records:
            shared = 0
            while shared < width and old[shared] == key[shared]:
                shared += 1
            # The first entry may not borrow the preceding page's key.
            if not payload:
                shared = 0
            tail = len(key) - len(key.rstrip(b" ")) if compression & 8 else 0
            tail = min(tail, width - shared)
            payload.extend(struct.pack("<IB", pointer, shared))
            if compression & 8:
                payload.append(tail)
            payload.extend(key[shared : width - tail])
            old = key
        offset = page_number * size
        nxt = offset + size if page_number - 1 < len(pages) else 0
        prev = offset - size if page_number > 2 else 0
        struct.pack_into("<IIHH", data, offset, nxt, prev, len(records), len(payload))
        data[offset + 17] = 1
        data[offset + 18 : offset + 18 + len(payload)] = payload
    return data


def record_fixture(kind, count, flags, pc_flags):
    names = [0x800001 + i for i in range(count)]
    data = bytearray([flags | (kind << 3) | count])
    data.extend(names[0].to_bytes(3, "little"))
    data.extend((3 | (pc_flags << 16)).to_bytes(3, "little"))
    data.extend((2).to_bytes(2, "little"))
    for code in names[1:]:
        data.extend(code.to_bytes(3, "little"))
    for width, present, value in [
        (3, flags & 128, 0x123456),
        (2, flags & 64, 0x3456),
        (3, kind in (2, 5), 0x456789),
        (3, 1 <= kind <= 6, 0x56789A),
        (2, pc_flags & 32, 0x6789),
        (1, pc_flags & 128, 7),
    ]:
        if present:
            data.extend(value.to_bytes(width, "little"))
    return bytes(data), tuple(names)


class ModernFormatTests(unittest.TestCase):
    def test_both_compression_modes_fill_runs_and_page_resets(self):
        entries = [(b"AA  ", 0x12345678), (b"AB  ", 7), (b"ABC ", 11), (b"ABCD", 17), (b"ABCE", 19)]
        for compression in (4, 12):
            for split in (None, 2):
                with self.subTest(compression=compression, split=split):
                    index = ModernLeafIndex(leaf_fixture(entries, compression, split))
                    self.assertEqual(list(index.keys()), entries)

    def test_corrupt_indexes_fail(self):
        source = leaf_fixture([(b"ABCD", 9)])
        for offset, value in [(57, 0), (64, 0), (8192 + 22, 5), (8192 + 17, 0)]:
            data = source.copy()
            data[offset] = value
            with self.subTest(offset=offset), self.assertRaises(FormatError):
                list(ModernLeafIndex(data).keys())
        data = source.copy()
        struct.pack_into("<I", data, 8192, 8192)
        with self.assertRaisesRegex(FormatError, "leaf chain"):
            list(ModernLeafIndex(data).keys())
        data = source.copy()
        struct.pack_into("<I", data, 24, 2)
        with self.assertRaisesRegex(FormatError, "count mismatch"):
            list(ModernLeafIndex(data).keys())
        with self.assertRaisesRegex(FormatError, "Unsorted"):
            list(ModernLeafIndex(leaf_fixture([(b"BBBB", 1), (b"AAAA", 2)])).keys())

    def test_fixed_table_resource_count_and_one_based_ids(self):
        data = bytearray(4096)
        struct.pack_into("<I", data, 8, 279)
        struct.pack_into("<I", data, 20, 4)  # metadata record is not a data row
        struct.pack_into("<I", data, 24, 3)
        struct.pack_into("<H", data, 44, 8)
        data[160:172] = b"FirstRecPos\0"
        struct.pack_into("<I", data, 172, 256)
        data[256:280] = b"ALPHA\0\0\0BETA\0\0\0\0GAMMA\0\0\0"
        table = ModernFixedTable(data)
        self.assertEqual(table.count, 3)
        self.assertEqual(table.raw(1), b"ALPHA\0\0\0")
        self.assertEqual(table.raw(3), b"GAMMA\0\0\0")
        for source_id in (0, 4):
            with self.assertRaises(FormatError):
                table.raw(source_id)
        struct.pack_into("<I", data, 24, 4)
        with self.assertRaisesRegex(FormatError, "bounds"):
            ModernFixedTable(data)

    def test_all_record_conditions_and_truncation(self):
        for kind in range(8):
            for count in range(1, 6):
                for flags in (0, 64, 128, 192):
                    for pc_flags in range(0, 256, 32):
                        with self.subTest(kind=kind, count=count, flags=flags, pc=pc_flags):
                            data, names = record_fixture(kind, count, flags, pc_flags)
                            row = decode_residence(data)
                            self.assertEqual(row.length, len(data))
                            self.assertEqual(row.name_codes, names)
                            self.assertEqual(row.postcode_id, 3)
                            self.assertEqual(row.house_id, 2)
                            self.assertEqual(
                                row.optional_kind25, 0x456789 if kind in (2, 5) else None
                            )
                            self.assertEqual(
                                row.optional_kind16, 0x56789A if 1 <= kind <= 6 else None
                            )
                            self.assertEqual(
                                row.optional_flag80,
                                (0x123456 | (1 << 24 if pc_flags & 64 else 0))
                                if flags & 128
                                else None,
                            )
                            self.assertEqual(row.optional_pc80, 7 if pc_flags & 128 else None)
                            with self.assertRaises(FormatError):
                                decode_residence(data[:-1])
        for data, offset in [(b"\0" * 9, 0), (b"\1" * 9, -1), (b"", 0)]:
            with self.assertRaises(FormatError):
                residential_length(data, offset)

    def test_structural_report_checks_group_boundaries_and_references(self):
        a, _ = record_fixture(0, 1, 0, 0)
        b, _ = record_fixture(3, 2, 0, 0)
        data = a + b
        tables = {n: SimpleNamespace(count=3) for n in ("surname", "name", "house", "postcode")}
        index = ModernLeafIndex(leaf_fixture([(bytes(4), 1), (len(a).to_bytes(4, "big"), 2)]))
        report = inspect_stream(data, index, tables, 0x1FFFFF)
        self.assertEqual(report["physical_records"], 2)
        self.assertEqual(report["stored_name_codes"], 3)
        self.assertEqual(report["surname_groups"], 2)
        deletions = ModernLeafIndex(leaf_fixture([(len(a).to_bytes(4, "little"), 1)]))
        checked = inspect_stream(data, index, tables, 0x1FFFFF, deletions)
        self.assertTrue(checked["deletion_references_valid"])
        self.assertEqual(checked["deletion_index_keys"], 1)
        bad_deletions = ModernLeafIndex(leaf_fixture([((len(a) + 1).to_bytes(4, "little"), 1)]))
        with self.assertRaisesRegex(FormatError, "deletion reference"):
            inspect_stream(data, index, tables, 0x1FFFFF, bad_deletions)
        missing_prefix = ModernLeafIndex(leaf_fixture([(len(a).to_bytes(4, "big"), 2)]))
        checked = inspect_stream(data, missing_prefix, tables, 0x1FFFFF)
        self.assertEqual(checked["unindexed_prefix_bytes"], len(a))
        broken = ModernLeafIndex(
            leaf_fixture([(bytes(4), 1), ((len(a) + 1).to_bytes(4, "big"), 2)])
        )
        with self.assertRaisesRegex(FormatError, "surname range"):
            inspect_stream(data, broken, tables, 0x1FFFFF)
        tables["postcode"].count = 2
        with self.assertRaisesRegex(FormatError, "postcode reference"):
            inspect_stream(data, index, tables, 0x1FFFFF)

    def test_unrecognized_sources_are_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaises(UnsupportedEditionError):
                inspect(directory)

    def test_inspector_cli_hash_verification_and_aggregate_output(self):
        table = bytearray(4096)
        struct.pack_into("<I", table, 8, 279)
        struct.pack_into("<I", table, 24, 3)
        struct.pack_into("<H", table, 44, 8)
        table[160:172] = b"FirstRecPos\0"
        struct.pack_into("<I", table, 172, 256)
        data, _ = record_fixture(0, 1, 0, 0)
        files = {"uk.dat": data, "ukspos.idx": leaf_fixture([(bytes(4), 1)])}
        files.update({name: table for name in ("d2.dat", "d3.dat", "d4.dat", "d6new.dat")})
        edition = Edition(
            "synthetic-modern",
            "Invented fixture",
            tuple(
                (name, len(content), hashlib.sha256(content).hexdigest())
                for name, content in files.items()
            ),
        )
        with tempfile.TemporaryDirectory() as directory:
            for name, content in files.items():
                (Path(directory) / name).write_bytes(content)
            output = io.StringIO()
            with patch("ukinfo_reader.modern_inspect.EXPERIMENTAL_EDITIONS", (edition,)):
                with redirect_stdout(output):
                    self.assertEqual(inspect_main(["--data", directory]), 0)
                report = json.loads(output.getvalue())
                self.assertEqual(report["status"], "structural-only")
                self.assertEqual(report["streams"]["uk.dat"]["physical_records"], 1)
                self.assertEqual(
                    report["files"]["uk.dat"]["sha256"], hashlib.sha256(data).hexdigest()
                )
                changed = bytearray(data)
                changed[-1] ^= 1
                (Path(directory) / "uk.dat").write_bytes(changed)
                error = io.StringIO()
                with redirect_stderr(error):
                    self.assertEqual(inspect_main(["--data", directory]), 1)
                self.assertIn("SHA-256 differs", error.getvalue())


if __name__ == "__main__":
    unittest.main()
