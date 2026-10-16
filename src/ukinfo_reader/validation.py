"""Full source-fingerprint, block, checksum and relationship validation."""

import argparse
import json
import struct
import sys
import time
from datetime import datetime, timezone
from pathlib import Path

from . import __version__
from .codecs import FormatError
from .reader import Database


def validate(directory="extracted", progress=None):
    """Validate the complete database and return a data-free report.

    progress, if supplied, is called with (completed_segments, total_segments, entries).
    """
    start = time.monotonic()
    totals = dict(postcodes=0, localities=0, addresses=0, entries=0)
    with Database(directory) as db:
        if db.format == "pcf78":
            from .pcf78_validation import validate_database

            return validate_database(db, directory, progress)
        if db.format == "modern":
            from .modern_validation import validate_database

            return validate_database(db, directory, progress)
        hashes = db.edition.verify(directory)
        last = 0
        for segment in range(db.segment_count):
            for field in db.fields:
                offset, size = struct.unpack_from(
                    "<II", db.idx, db.table + (segment * len(db.fields) + field["id"]) * 8
                )
                if offset != last:
                    raise FormatError(f"Noncontiguous block: {segment}:{field['name']}")
                last = offset + size
            try:
                stats = db.validate_segment(segment)
            except FormatError as exc:
                raise FormatError(f"Segment {segment} ({db.sector(segment)}): {exc}") from exc
            for key, value in stats.items():
                totals[key] += value
            if progress and (segment % 500 == 0 or segment == db.segment_count - 1):
                progress(segment + 1, db.segment_count, totals["entries"])
        if last != len(db.dat):
            raise FormatError("Unreferenced data at end")
        return dict(
            edition=db.edition.id,
            reader_version=__version__,
            validated_at=datetime.now(timezone.utc).isoformat(),
            segments=db.segment_count,
            blocks=db.segment_count * len(db.fields),
            totals=totals,
            elapsed_seconds=round(time.monotonic() - start, 2),
            sha256=hashes,
            checks=[
                "source_sha256",
                "contiguous_data_coverage",
                "numeric_checksums",
                "column_lengths",
                "parent_boundaries",
                "inverse_postcode_relationships",
            ],
        )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--data", default="extracted")
    parser.add_argument("--output", default="validation.json")
    args = parser.parse_args()

    def progress(done, total, entries):
        print(f"{done}/{total} units; {entries:,} entries", file=sys.stderr)

    try:
        report = validate(args.data, progress=progress)
        Path(args.output).write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
        print(json.dumps(report, indent=2))
    except (OSError, FormatError) as exc:
        print(f"ukinfo-validate: {exc}", file=sys.stderr)
        raise SystemExit(1) from None
