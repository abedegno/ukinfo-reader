#!/usr/bin/env python3
"""Run the bounded synthetic ARM64 search spike; never touches an existing DB."""

import argparse
import hashlib
import io
import json
import os
import shutil
import subprocess
import tempfile
import urllib.request
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
NAME = "ukinfo-search-index-spike"
IMAGE = "ukinfo-search-spike:pg18-pgs0.26.3-text1.5.1"
BASE = (
    "pgvector/pgvector:pg18@sha256:2358fcba361ed2233a5ed81b5fe4ca779ccb304120ce531a3bf51c0ed7e2bc11"
)
ASSETS = [
    (
        "https://github.com/paradedb/paradedb/releases/download/v0.26.3/postgresql-18-pg-search_0.26.3-1PARADEDB-bookworm_arm64.deb",
        "ab554b531d1e1d644a1d0a6e75eda06ffbb3448ad19502b8ff584ba1e0bd0433",
        "pg_search.deb",
    ),
    (
        "https://github.com/timescale/pg_textsearch/releases/download/v1.5.1/pg-textsearch-v1.5.1-pg18-arm64.zip",
        "54843473cbad144812be0437bc708233087c7823018700ffb99a2fb92d1466bb",
        "pg_textsearch.zip",
    ),
]


def run(*args, **kwargs):
    return subprocess.run(args, check=True, **kwargs)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--rows", type=int, default=1000000)
    p.add_argument("--rounds", type=int, default=4)
    p.add_argument("--output", type=Path, required=True)
    p.add_argument(
        "--custom-output", type=Path, help="also measure custom plans on the same fixtures"
    )
    args = p.parse_args()
    if not 1000 <= args.rows <= 2000000 or not 1 <= args.rounds <= 10:
        p.error("rows must be 1000..2000000 and rounds 1..10")
    if shutil.disk_usage(ROOT).free < 8 * 1024**3:
        p.error("need at least 8 GiB free workspace disk; check Docker VM capacity too")
    arch = subprocess.check_output(
        ["docker", "info", "--format", "{{.Architecture}}"], text=True
    ).strip()
    if arch not in ("aarch64", "arm64"):
        p.error("this measured package lock is ARM64 only; do not silently emulate")
    names = subprocess.check_output(
        ["docker", "ps", "-a", "--format", "{{.Names}}"], text=True
    ).splitlines()
    if NAME in names:
        p.error(f"{NAME} already exists; inspect/remove your previous experiment explicitly")
    args.output = args.output.resolve()
    if args.custom_output:
        args.custom_output = args.custom_output.resolve()
        if args.custom_output == args.output or args.custom_output.exists():
            p.error("custom output must be a distinct new report path")
    if args.output.exists():
        p.error("output already exists; choose a new report path")
    with tempfile.TemporaryDirectory(prefix="ukinfo-search-spike-") as tmp:
        tmp = Path(tmp)
        for url, digest, filename in ASSETS:
            with urllib.request.urlopen(url, timeout=60) as response:
                data = response.read(64 * 1024**2)
            if hashlib.sha256(data).hexdigest() != digest:
                raise RuntimeError(f"checksum mismatch: {filename}")
            if filename.endswith(".zip"):
                archive = zipfile.ZipFile(io.BytesIO(data))
                name = "pg-textsearch-postgresql-18_1.5.1-1_arm64.deb"
                (tmp / "pg_textsearch.deb").write_bytes(archive.read(name))
            else:
                (tmp / filename).write_bytes(data)
        (tmp / "Dockerfile").write_text(f"""FROM {BASE}
COPY pg_search.deb pg_textsearch.deb /tmp/
RUN apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends /tmp/pg_search.deb /tmp/pg_textsearch.deb && rm -rf /var/lib/apt/lists/* /tmp/*.deb
""")
        env = dict(os.environ, GOOS="linux", GOARCH="arm64", CGO_ENABLED="0")
        run(
            "go",
            "build",
            "-o",
            str(tmp / "benchmark"),
            "./cmd/search-index-spike",
            cwd=ROOT / "server",
            env=env,
        )
        run("docker", "build", "-t", IMAGE, str(tmp))
        started = False
        try:
            run(
                "docker",
                "run",
                "-d",
                "--name",
                NAME,
                "--cpus",
                "2",
                "--memory",
                "2g",
                "--shm-size",
                "256m",
                "--network",
                "none",
                "-e",
                "POSTGRES_HOST_AUTH_METHOD=trust",
                "-e",
                "POSTGRES_DB=spike",
                IMAGE,
                "-c",
                "shared_preload_libraries=pg_search,pg_textsearch",
                "-c",
                "shared_buffers=512MB",
                "-c",
                "max_parallel_workers_per_gather=0",
                "-c",
                "work_mem=32MB",
                "-c",
                "jit=off",
            )
            started = True
            # Local readiness polling inside the container: no published ports/network.
            run(
                "docker",
                "exec",
                NAME,
                "sh",
                "-c",
                'i=0; until pg_isready -U postgres -d spike; do i=$((i+1)); [ "$i" -lt 30 ] || exit 1; sleep 1; done',
            )
            run("docker", "cp", str(tmp / "benchmark"), NAME + ":/tmp/benchmark")
            result = subprocess.run(
                [
                    "docker",
                    "exec",
                    NAME,
                    "/tmp/benchmark",
                    "--rows",
                    str(args.rows),
                    "--rounds",
                    str(args.rounds),
                    "--output",
                    "/tmp/report.json",
                ]
            )
            args.output.parent.mkdir(parents=True, exist_ok=True)
            copy = subprocess.run(["docker", "cp", NAME + ":/tmp/report.json", str(args.output)])
            if result.returncode == 0:
                copy.check_returncode()
            if args.custom_output and copy.returncode == 0:
                # Continue after recorded query failures, but not an aborted build/oracle check.
                json.loads(args.output.read_text())
                custom = subprocess.run(
                    [
                        "docker",
                        "exec",
                        NAME,
                        "/tmp/benchmark",
                        "--rows",
                        str(args.rows),
                        "--rounds",
                        str(args.rounds),
                        "--measure-existing",
                        "/tmp/report.json",
                        "--plan-mode",
                        "force_custom_plan",
                        "--output",
                        "/tmp/custom-report.json",
                    ]
                )
                args.custom_output.parent.mkdir(parents=True, exist_ok=True)
                run("docker", "cp", NAME + ":/tmp/custom-report.json", str(args.custom_output))
                custom.check_returncode()
            result.check_returncode()
        finally:
            if started:
                run("docker", "rm", "-f", "-v", NAME)
    print(f"Report: {args.output}. The pinned image/build cache is retained for reuse.")


if __name__ == "__main__":
    main()
