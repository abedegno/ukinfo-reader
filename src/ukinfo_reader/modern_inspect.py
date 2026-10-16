"""Reproduce structural checks without printing or exporting personal records."""

import argparse
import json
import mmap
import struct
import sys
from collections import Counter
from contextlib import ExitStack
from pathlib import Path

from .codecs import FormatError
from .editions import UnsupportedEditionError
from .modern import ModernFixedTable, ModernLeafIndex, residential_length
from .modern_sources import EXPERIMENTAL_EDITIONS


def inspect_stream(data, surname_index, tables, name_mask, deletion_index=None):
    """Check every record boundary, name/postcode/house reference and surname group.

    A bitset takes one bit per source byte, avoiding a Python set of tens of
    millions of record offsets. Nothing in the returned report identifies a person.
    Zero dictionary references are preserved as absent, not resolved to a row.
    """
    boundaries = bytearray((len(data) + 7) // 8)
    kinds, maxima, absent = Counter(), Counter(), Counter()
    offset = count = names = 0
    while offset < len(data):
        length = residential_length(data, offset)
        flags = data[offset]
        num = flags & 7
        boundaries[offset >> 3] |= 1 << (offset & 7)
        first = int.from_bytes(data[offset + 1 : offset + 4], "little") & name_mask
        pc = int.from_bytes(data[offset + 4 : offset + 7], "little") & 0x1FFFFF
        house = int.from_bytes(data[offset + 7 : offset + 9], "little")
        references = [("name", first), ("postcode", pc), ("house", house)]
        references.extend(
            ("name", int.from_bytes(data[pos : pos + 3], "little") & name_mask)
            for pos in range(offset + 9, offset + 6 + num * 3, 3)
        )
        for field, value in references:
            if value > tables[field].count:
                raise FormatError(f"{field} reference exceeds dictionary at byte {offset}")
            maxima[field] = max(maxima[field], value)
            absent[field] += value == 0
        kinds[str((flags >> 3) & 7)] += 1
        offset += length
        count += 1
        names += num
    groups = 0
    previous = -1
    first_indexed_byte = None
    for key, source_id in surname_index.keys():
        if len(key) != 4:
            raise FormatError("Unexpected surname range key width")
        start = int.from_bytes(key, "big")
        if (
            start >= len(data)
            or not boundaries[start >> 3] & (1 << (start & 7))
            or start <= previous
            or not 1 <= source_id <= tables["surname"].count
        ):
            raise FormatError("Invalid surname range reference")
        if not groups:
            first_indexed_byte = start
        previous = start
        groups += 1
    if count and not groups:
        raise FormatError("Missing surname ranges")
    report = dict(
        bytes=offset,
        physical_records=count,
        stored_name_codes=names,
        kind_counts=dict(sorted(kinds.items())),
        maximum_dictionary_ids=dict(maxima),
        absent_dictionary_ids=dict(absent),
        surname_groups=groups,
        unindexed_prefix_bytes=first_indexed_byte,
        record_boundaries_valid=True,
        dictionary_references_valid=True,
        surname_range_references_valid=True,
    )
    if deletion_index is not None:
        deleted = set()
        for key, _ in deletion_index.keys():
            start = int.from_bytes(key, "little")
            if (
                len(key) != 4
                or start >= len(data)
                or not boundaries[start >> 3] & (1 << (start & 7))
                or start in deleted
            ):
                raise FormatError("Invalid residential deletion reference")
            deleted.add(start)
        report["deletion_references_valid"] = True
        report["deletion_index_keys"] = len(deleted)
    return report


def inspect(directory):
    """Verify an exact structural source profile and inspect residential streams.

    This is structural verification, not verification of the original program's
    results. The 2005 base and update streams are reported separately.
    """
    directory = Path(directory)
    edition = next(
        (
            e
            for e in EXPERIMENTAL_EDITIONS
            if all(
                (directory / n).is_file() and (directory / n).stat().st_size == size
                for n, size, _ in e.files
            )
        ),
        None,
    )
    if edition is None:
        raise UnsupportedEditionError("Unknown structural source or incomplete files")
    hashes = edition.verify(directory)
    report = dict(
        edition=edition.id,
        title=edition.title,
        status="structural-only",
        files={n: dict(bytes=size, sha256=hashes[n]) for n, size, _ in edition.files},
        streams={},
        dictionaries={},
        limitations=[
            "Physical source records and stored name codes are not live directory totals.",
            "This report does not check joined residential output or business records.",
            "Use ukinfo-validate with the complete residential profile for joined output checks.",
        ],
    )
    with ExitStack() as resources:
        mapped = {}
        for name, _, _ in edition.files:
            stream = resources.enter_context((directory / name).open("rb"))
            mapped[name] = resources.enter_context(
                mmap.mmap(stream.fileno(), 0, access=mmap.ACCESS_READ)
            )
        for suffix, dat, idx in [
            ("", "uk.dat", "ukspos.idx"),
            ("upd", "ukupd.dat", "uksposupd.idx"),
        ]:
            if dat not in mapped:
                continue
            tables = {
                field: ModernFixedTable(mapped[name + suffix + ".dat"])
                for field, name in [
                    ("surname", "d2"),
                    ("name", "d3"),
                    ("house", "d4"),
                    ("postcode", "d6new"),
                ]
            }
            for table in tables.values():
                name = next(n for n, data in mapped.items() if data is table.data)
                report["dictionaries"][name] = dict(
                    first_record=table.start, width=table.width, count=table.count
                )
            report["streams"][dat] = inspect_stream(
                mapped[dat],
                ModernLeafIndex(mapped[idx]),
                tables,
                0x3FFFFF if edition.id.startswith("se2004") else 0x1FFFFF,
                ModernLeafIndex(mapped["delr.idx"])
                if not suffix and "delr.idx" in mapped
                else None,
            )
        if "delr.idx" in mapped:
            report["limitations"].append(
                "2005 deletion keys are structurally decoded but not applied; "
                "base and update streams are not merged."
            )
    return report


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--data", type=Path, required=True, help="Flattened extracted source files")
    parser.add_argument("--output", type=Path, help="Write aggregate JSON report")
    args = parser.parse_args(argv)
    try:
        report = inspect(args.data)
        result = json.dumps(report, indent=2) + "\n"
        if args.output:
            args.output.write_text(result, encoding="utf-8")
        else:
            print(result, end="")
    except (OSError, FormatError, struct.error) as exc:
        print(f"ukinfo-inspect: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
