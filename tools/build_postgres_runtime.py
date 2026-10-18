"""Build a minimal native PostgreSQL runtime from hash-pinned official source.

No database or source directory records are used. Needs a C compiler, GNU make,
Bison/Flex, and patchelf on Linux. Output must not already exist.
"""

import argparse
import hashlib
import json
import os
import platform
import shutil
import subprocess
import tarfile
import tempfile
import urllib.request
from pathlib import Path

VERSION = "18.6"
SHA256 = "555610c24d53e4316da5b7d3fc25c279d96856d5e0e23ee308c328c5fa881d9f"
URL = f"https://ftp.postgresql.org/pub/source/v{VERSION}/postgresql-{VERSION}.tar.bz2"


def run(args, **kwargs):
    subprocess.run([str(a) for a in args], check=True, **kwargs)


def relocate(output):
    for file in list((output / "bin").iterdir()) + list((output / "lib").rglob("*")):
        if not file.is_file() or file.is_symlink():
            continue
        magic = file.read_bytes()[:4]
        if platform.system() == "Darwin" and magic in (b"\xcf\xfa\xed\xfe", b"\xfe\xed\xfa\xcf"):
            dependencies = subprocess.check_output(["otool", "-L", file], text=True).splitlines()[
                1:
            ]
            for dependency in dependencies:
                source = dependency.strip().split(" (", 1)[0]
                if source.startswith(str(output) + "/"):
                    relative = os.path.relpath(source, file.parent)
                    run(["install_name_tool", "-change", source, "@loader_path/" + relative, file])
                elif not source.startswith(("/usr/lib/", "/System/Library/", "@")):
                    raise RuntimeError(f"Nonportable dependency in {file.name}: {source}")
            if file.suffix == ".dylib":
                run(["install_name_tool", "-id", "@rpath/" + file.name, file])
            run(["codesign", "--force", "--sign", "-", file])
        elif platform.system() == "Linux" and magic == b"\x7fELF":
            relative = os.path.relpath(output / "lib", file.parent)
            run(["patchelf", "--set-rpath", "$ORIGIN/" + relative, file])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--jobs", type=int, default=min(os.cpu_count() or 2, 4))
    args = parser.parse_args()
    output = args.output.resolve()
    if output.exists():
        parser.error("output already exists; choose a fresh runtime directory")
    if platform.system() not in ("Darwin", "Linux"):
        parser.error("native macOS/Linux runtime builds are supported")
    if platform.system() == "Linux" and not shutil.which("patchelf"):
        parser.error("install patchelf before building the Linux runtime")
    if args.jobs < 1 or args.jobs > 16:
        parser.error("jobs must be 1–16")
    if shutil.disk_usage(tempfile.gettempdir()).free < 5 * 1024**3:
        parser.error("retain at least 5 GiB free before building")
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="ukinfo-postgres-build-") as temp:
        work = Path(temp)
        archive = work / "postgres.tar.bz2"
        with urllib.request.urlopen(URL, timeout=60) as response, archive.open("wb") as target:
            shutil.copyfileobj(response, target)
        if hashlib.sha256(archive.read_bytes()).hexdigest() != SHA256:
            raise RuntimeError("PostgreSQL source SHA-256 mismatch")
        with tarfile.open(archive) as source:
            source.extractall(work, filter="data")
        source = work / f"postgresql-{VERSION}"
        options = [
            "--without-" + name
            for name in (
                "readline",
                "zlib",
                "icu",
                "ldap",
                "libxml",
                "libxslt",
                "lz4",
                "zstd",
                "gssapi",
                "pam",
                "liburing",
            )
        ]
        run(
            [
                source / "configure",
                f"--prefix={output}",
                "--disable-nls",
                "--disable-rpath",
                *options,
            ],
            cwd=source,
        )
        run(["make", f"-j{args.jobs}"], cwd=source)
        run(["make", "install"], cwd=source)
        run(["make", "-C", "contrib/cube", f"-j{args.jobs}", "install"], cwd=source)
        shutil.copy2(source / "COPYRIGHT", output / "PostgreSQL-COPYRIGHT")
        # Retain the complete source notices; generated scanner/parser exceptions
        # and other in-tree notices accompany the runtime too.
        notices = output / "source-notices"
        for item in source.rglob("*"):
            if item.is_file() and item.name.upper().startswith(
                ("COPYRIGHT", "LICENSE", "NOTICE", "COPYING")
            ):
                target = notices / item.relative_to(source)
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(item, target)
        relocate(output)
        metadata = {
            "version": VERSION,
            "source_url": URL,
            "source_sha256": SHA256,
            "platform": platform.system(),
            "architecture": platform.machine(),
            "configure": options + ["--disable-nls", "--disable-rpath"],
            "extensions": ["cube"],
        }
        (output / "BUILD.json").write_text(json.dumps(metadata, indent=2) + "\n")
        # Move away from the build prefix: smoke tests must not accidentally find
        # libraries or share files in the original installation location.
        relocated = output.with_name(output.name + "-relocation-check")
        output.rename(relocated)
        try:
            run([relocated / "bin/postgres", "--version"])
            run([relocated / "bin/initdb", "--version"])
        finally:
            relocated.rename(output)
    print(f"Built relocatable PostgreSQL {VERSION} with cube: {output}")


if __name__ == "__main__":
    main()
