"""Verified, lossless streaming interchange for the Go search catalogue importer.

This is an internal, versioned protocol, not the public CLI's JSONL row format.
The terminal frame lets the receiver reject a truncated or failed decoder pipeline.
"""

import argparse
import itertools
import json
import sys

from . import __version__
from .reader import Database

COUNTS = {
    "se-pcf78-1998-01": 27_983_545,
    "se4-pcf94-1999-04": 46_355_850,
    "se2004-pcp218-2004-07": 63_373_974,
    "se2005-pcf175-2005-06": 51_350_897,
}


def records(db):
    if db.format == "se4":
        for segment in range(db.segment_count):
            yield from db.records(segment)
    else:
        yield from db.records()


def source_id(row):
    if "segment_id" in row:
        return f"{row['segment_id']}:{row['entry_id']}"
    return str(row["entry_id"])


def emit(db, directory, output, limit=None):
    hashes = db.edition.verify(directory)
    dataset = db.edition.id + (f":sample:{limit}" if limit else "")

    def write(frame):
        output.write(json.dumps(frame, ensure_ascii=True, separators=(",", ":")) + "\n")

    write(
        dict(
            type="manifest",
            protocol=1,
            dataset=dataset,
            edition=db.edition.id,
            title=db.edition.title,
            reader_version=__version__,
            sha256=hashes,
            sample=limit is not None,
            expected=limit or COUNTS[db.edition.id],
        )
    )
    count = 0
    stream = records(db)
    if limit:
        stream = itertools.islice(stream, limit)
    for count, row in enumerate(stream, 1):
        write(dict(type="record", source_id=source_id(row), record=row))
    expected = limit or COUNTS[db.edition.id]
    if count != expected:
        raise ValueError(f"Decoded {count:,} entries; expected {expected:,}; import rejected")
    write(dict(type="end", count=count))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--data", required=True)
    parser.add_argument("--limit", type=int, help="explicitly import a labelled partial sample")
    args = parser.parse_args()
    if args.limit is not None and args.limit < 1:
        parser.error("--limit must be positive")
    with Database(args.data) as db:
        emit(db, args.data, sys.stdout, args.limit)


if __name__ == "__main__":
    main()
