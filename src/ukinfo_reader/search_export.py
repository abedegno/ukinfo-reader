"""Verified, lossless streaming interchange for the Go search catalogue importer.

This is an internal, versioned protocol, not the public CLI's JSONL row format.
The terminal frame lets the receiver reject a truncated or failed decoder pipeline.
"""

import argparse
import hashlib
import itertools
import json
import signal
import struct
import sys

from . import __version__
from .reader import Database

COUNTS = {
    "synthetic-demo": 126,
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
            protocol=2,
            dataset=dataset,
            edition=db.edition.id,
            title=db.edition.title,
            reader_version=__version__,
            sha256=hashes,
            sample=limit is not None,
            expected=limit or COUNTS[db.edition.id],
        )
    )
    digest = hashlib.sha256()
    count = 0
    stream = records(db)
    if limit:
        stream = itertools.islice(stream, limit)
    for count, row in enumerate(stream, 1):
        identity = source_id(row)
        raw = json.dumps(row, ensure_ascii=True, separators=(",", ":"))
        identity_bytes = identity.encode("utf-8")
        raw_bytes = raw.encode("utf-8")
        digest.update(struct.pack("<HI", len(identity_bytes), len(raw_bytes)))
        digest.update(identity_bytes)
        digest.update(raw_bytes)
        output.write(
            '{"type":"record","source_id":' + json.dumps(identity) + ',"record":' + raw + "}\n"
        )
    expected = limit or COUNTS[db.edition.id]
    if count != expected:
        raise ValueError(f"Decoded {count:,} entries; expected {expected:,}; import rejected")
    # Recheck the source after decoding so an ordinary concurrent change fails closed.
    if db.edition.verify(directory) != hashes:
        raise ValueError("Source files changed while decoding; import rejected")
    write(dict(type="end", count=count, sha256=digest.hexdigest()))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument("--data")
    source.add_argument(
        "--prepare-media", help="extract and verify supported ISO/7z database files"
    )
    parser.add_argument("--output-folder", help="new temporary folder for --prepare-media")
    source.add_argument("--demo", action="store_true", help="use invented demo entries")
    parser.add_argument(
        "--identify", action="store_true", help="identify and verify without exporting"
    )
    parser.add_argument("--limit", type=int, help="explicitly import a labelled partial sample")
    args = parser.parse_args()
    if args.limit is not None and args.limit < 1:
        parser.error("--limit must be positive")

    if args.prepare_media:
        from .media import prepare

        if not args.output_folder:
            parser.error("--prepare-media requires --output-folder")

        # Native cancellation must unwind extraction before the owning process
        # removes its temporary root. SIGKILL remains a final native fallback.
        def interrupted(signum, frame):
            raise KeyboardInterrupt("Disc preparation canceled")

        signal.signal(signal.SIGTERM, interrupted)
        edition = prepare(args.prepare_media, args.output_folder)
        print(json.dumps(dict(edition=edition.id, title=edition.title, entries=COUNTS[edition.id])))
        return

    def run(db):
        if args.identify:
            print(
                json.dumps(
                    dict(
                        edition=db.edition.id,
                        title=db.edition.title,
                        entries=COUNTS[db.edition.id],
                        sha256=db.edition.verify(args.data),
                    )
                )
            )
        else:
            emit(db, args.data, sys.stdout, args.limit)

    if args.demo:
        from .demo import database

        run(database())
    else:
        with Database(args.data) as db:
            run(db)


if __name__ == "__main__":
    main()
