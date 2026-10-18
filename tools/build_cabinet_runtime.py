"""Build the narrow MIT-licensed InstallShield helper from pinned upstream source."""

import hashlib
import json
import shutil
import subprocess
import sys
import tarfile
import tempfile
import urllib.request
from pathlib import Path

VERSION = "1.6.2"
URL = f"https://github.com/twogood/unshield/archive/refs/tags/{VERSION}.tar.gz"
SHA256 = "a937ef596ad94d16e7ed2c8553ad7be305798dcdcfd65ae60210b1e54ab51a2f"


def build(output):
    if sys.platform not in ("darwin", "linux"):
        raise RuntimeError("Native cabinet helper supports macOS and Linux")
    output.mkdir(parents=True, exist_ok=True)
    if shutil.disk_usage(output).free < 5 << 30:
        raise RuntimeError("Retain 5 GiB free for building")
    with tempfile.TemporaryDirectory(prefix="ukinfo-unshield-") as temporary:
        root = Path(temporary)
        archive = root / "source.tar.gz"
        with urllib.request.urlopen(URL, timeout=60) as response, archive.open("wb") as stream:
            shutil.copyfileobj(response, stream)
        if hashlib.sha256(archive.read_bytes()).hexdigest() != SHA256:
            raise RuntimeError("Unshield source hash mismatch")
        with tarfile.open(archive) as source:
            source.extractall(root, filter="data")
        source = root / f"unshield-{VERSION}"
        build_dir = root / "build"
        subprocess.run(
            [
                "cmake",
                "-S",
                str(source),
                "-B",
                str(build_dir),
                "-DBUILD_STATIC=ON",
                "-DBUILD_TESTING=OFF",
                "-DUSE_OUR_OWN_MD5=ON",
                "-DCMAKE_BUILD_TYPE=Release",
            ],
            check=True,
        )
        subprocess.run(["cmake", "--build", str(build_dir), "--parallel", "2"], check=True)
        binary = output / "ukinfo-cabinet"
        subprocess.run(
            [
                "cc",
                "-O2",
                "-I",
                str(source / "lib"),
                str(Path(__file__).with_name("cabinet.c")),
                str(build_dir / "lib/libunshield.a"),
                str(build_dir / "lib/md5/CMakeFiles/md5.dir/md5c.c.o"),
                "-lz",
                "-o",
                str(binary),
            ],
            check=True,
        )
        notices = output / "cabinet-notices"
        notices.mkdir(exist_ok=True)
        shutil.copy2(source / "LICENSE", notices / "unshield-LICENSE")
        shutil.copytree(source / "lib/md5", notices / "md5-source-notices", dirs_exist_ok=True)
        # macOS libz is supplied by the OS. Linux's system zlib is supplied by
        # the distribution; retain its full copyright notice in the bundle.
        if sys.platform == "linux":
            shutil.copy2("/usr/share/doc/zlib1g/copyright", notices / "zlib-copyright")
        (notices / "BUILD.json").write_text(
            json.dumps(
                dict(
                    version=VERSION,
                    source=URL,
                    sha256=SHA256,
                    library="static libunshield; system zlib",
                ),
                indent=2,
            )
            + "\n"
        )
        if sys.platform == "darwin":
            subprocess.run(["codesign", "--force", "--sign", "-", str(binary)], check=True)
        subprocess.run([str(binary)], check=False)  # argument validation, no source data


if __name__ == "__main__":
    build(Path(sys.argv[1]))
