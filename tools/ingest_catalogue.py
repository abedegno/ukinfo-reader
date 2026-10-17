"""Run the verified decoder and Go importer with local disk headroom monitoring.

DATABASE_URL is inherited by the Go child. Original records flow through a pipe,
not a temporary JSONL export. The guard checks the selected local filesystem; it
cannot measure free space on a remote PostgreSQL host.
"""

import argparse
import os
import shutil
import subprocess
import sys
import time
from pathlib import Path


def stop(process):
    if process.poll() is None:
        process.terminate()
        try:
            process.wait(timeout=20)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--data", required=True, type=Path)
    parser.add_argument("--server", type=Path, default=Path("build/desktop/ukinfo-server"))
    parser.add_argument("--limit", type=int)
    parser.add_argument("--min-free-gib", type=float, default=5)
    parser.add_argument("--watch-filesystem", type=Path, default=Path.cwd())
    parser.add_argument("--data-dir", type=Path, help="local app-managed PostgreSQL directory")
    parser.add_argument("--pg-bin", type=Path, help="native PostgreSQL 18 bin directory")
    args = parser.parse_args()
    if args.min_free_gib < 1:
        parser.error("retain at least 1 GiB of local free space")
    minimum = args.min_free_gib * 1024**3
    if shutil.disk_usage(args.watch_filesystem).free < minimum:
        parser.error("insufficient local disk headroom before starting")
    producer_args = [
        sys.executable,
        "-m",
        "ukinfo_reader.search_export",
        "--data",
        str(args.data.resolve()),
    ]
    if args.limit is not None:
        if args.limit < 1:
            parser.error("--limit must be positive")
        producer_args += ["--limit", str(args.limit)]
    consumer_args = [str(args.server.resolve()), "import"]
    if args.data_dir:
        if not args.pg_bin:
            parser.error("--data-dir requires --pg-bin")
        if os.environ.get("DATABASE_URL"):
            parser.error("unset DATABASE_URL when importing into --data-dir")
        consumer_args += [
            "--data-dir",
            str(args.data_dir.resolve()),
            "--pg-bin",
            str(args.pg_bin.resolve()),
        ]
    producer = subprocess.Popen(producer_args, stdout=subprocess.PIPE)
    consumer = None
    try:
        consumer = subprocess.Popen(consumer_args, stdin=producer.stdout)
        producer.stdout.close()
        while consumer.poll() is None:
            if shutil.disk_usage(args.watch_filesystem).free < minimum:
                raise RuntimeError(
                    "local disk headroom reached; canceling import; check catalogue status before retrying"
                )
            if producer.poll() not in (None, 0):
                raise RuntimeError("source decoder failed; canceling import")
            time.sleep(1)
        if consumer.returncode or producer.wait():
            raise RuntimeError(
                "import did not finish cleanly; check catalogue status before retrying"
            )
    finally:
        stop(producer)
        if consumer is not None:
            stop(consumer)


if __name__ == "__main__":
    try:
        main()
    except (KeyboardInterrupt, RuntimeError) as exc:
        print(
            str(exc) or "Import canceled; check catalogue status before retrying", file=sys.stderr
        )
        raise SystemExit(1) from None
