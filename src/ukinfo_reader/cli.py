"""Command-line search and export interface."""

import argparse
import csv
import json
import os
import sys

from . import __version__
from .codecs import FormatError
from .editions import SUPPORTED_EDITIONS
from .reader import Database


def run():
    p = argparse.ArgumentParser(
        description="Search the supported UK-Info Disk edition and export decoded records"
    )
    p.add_argument("--version", action="version", version=__version__)
    p.add_argument(
        "--editions", action="store_true", help="list supported releases without opening files"
    )
    p.add_argument(
        "--identify",
        action="store_true",
        help="identify the release and verify source SHA-256 hashes",
    )
    p.add_argument(
        "--data",
        default="extracted",
        help="directory containing the edition's extracted database files",
    )
    p.add_argument("--segment", type=int, help="restrict to one numbered segment")
    p.add_argument("--sector", help='restrict to a postcode sector, e.g. "AB10 1"')
    p.add_argument("--surname", help="exact surname, ignoring case")
    p.add_argument("--first-name", help="exact first name, ignoring case")
    p.add_argument("--postcode", help="exact full postcode, ignoring spaces/case")
    p.add_argument("--limit", type=int, help="maximum records to emit")
    p.add_argument("--field", help="decode a single column (requires --segment)")
    p.add_argument("--schema", action="store_true", help="show field definitions")
    p.add_argument("--all", action="store_true", help="export every directory entry (large output)")
    p.add_argument("--format", choices=["jsonl", "csv"], default="jsonl")
    args = p.parse_args()
    if args.limit is not None and args.limit < 1:
        p.error("--limit must be positive")
    if args.editions:
        for edition in SUPPORTED_EDITIONS:
            print(f"{edition.id}: {edition.title}")
        return
    if args.segment is not None and args.segment < 0:
        p.error("--segment must be nonnegative")
    if args.field and args.segment is None:
        p.error("--field requires --segment")
    if not any(
        (
            args.identify,
            args.schema,
            args.field,
            args.all,
            args.sector,
            args.surname,
            args.first_name,
            args.postcode,
            args.segment is not None,
        )
    ):
        p.error("choose --schema, --identify, --all or a search filter")
    db = Database(args.data)
    try:
        if args.identify:
            hashes = db.edition.verify(args.data)
            print(
                json.dumps(
                    dict(edition=db.edition.id, title=db.edition.title, sha256=hashes), indent=2
                )
            )
            return
        if db.format in ("pcf78", "modern"):
            if args.segment is not None or args.field:
                p.error("This edition has no SE 4.0 segments or columns")
            if args.schema:
                print(json.dumps(dict(edition=db.edition.id, fields=db.fields), indent=2))
                return
            if db.format == "pcf78":
                print(
                    "Source note: known mismatched index keys are excluded; unresolved source "
                    "values are preserved and flagged. See docs/formats/pcf78.md.",
                    file=sys.stderr,
                )
            rows = db.search(
                surname=args.surname,
                first_name=args.first_name,
                postcode=args.postcode,
                sector=args.sector,
            )
            writer = None
            for emitted, row in enumerate(rows, 1):
                if args.format == "jsonl":
                    print(json.dumps(row, ensure_ascii=False))
                else:
                    if writer is None:
                        writer = csv.DictWriter(sys.stdout, fieldnames=list(row))
                        writer.writeheader()
                    writer.writerow(row)
                if args.limit and emitted >= args.limit:
                    break
            return
        if args.segment is not None and args.segment >= db.segment_count:
            p.error(f"--segment must be below {db.segment_count}")
        if args.schema:
            print(
                json.dumps(
                    dict(edition=db.edition.id, segments=db.segment_count, fields=db.fields),
                    indent=2,
                )
            )
            return
        if args.field:
            if args.field not in db.names:
                p.error(f"unknown field: {args.field}")
            print(json.dumps(db.column(args.segment, args.field), ensure_ascii=False))
            return
        segments = [args.segment] if args.segment is not None else range(db.segment_count)
        emitted = 0
        writer = None
        for segment in segments:
            sector = db.sector(segment)
            if args.sector and sector.casefold() != args.sector.strip().casefold():
                continue
            if args.postcode and not args.postcode.replace(" ", "").upper().startswith(
                sector.replace(" ", "")
            ):
                continue
            indices = None
            if args.surname:
                indices = [
                    i
                    for i, v in enumerate(db.column(segment, "surname"))
                    if v.casefold() == args.surname.casefold()
                ]
                if not indices:
                    continue
            if args.first_name:
                names = db.column(segment, "firstName")
                indices = [
                    i
                    for i in (range(len(names)) if indices is None else indices)
                    if names[i].casefold() == args.first_name.casefold()
                ]
                if not indices:
                    continue
            for row in db.records(segment, indices):
                if (
                    args.postcode
                    and row["postcode"].replace(" ", "").upper()
                    != args.postcode.replace(" ", "").upper()
                ):
                    continue
                if args.format == "jsonl":
                    print(json.dumps(row, ensure_ascii=False))
                else:
                    if writer is None:
                        writer = csv.DictWriter(sys.stdout, fieldnames=list(row))
                        writer.writeheader()
                    writer.writerow(row)
                emitted += 1
                if args.limit and emitted >= args.limit:
                    return
    finally:
        db.close()


def main():
    """Console entry point with concise errors and normal pipeline behavior."""
    if hasattr(sys.stdout, "reconfigure"):
        sys.stdout.reconfigure(encoding="utf-8", newline="")
    try:
        run()
    except BrokenPipeError:
        # Prevent a second flush error when a consumer such as head closes stdout.
        with open(os.devnull, "w") as sink:
            os.dup2(sink.fileno(), sys.stdout.fileno())
    except (OSError, FormatError) as exc:
        print(f"ukinfo-read: {exc}", file=sys.stderr)
        raise SystemExit(1) from None
