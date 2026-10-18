"""Verify all three native desktop archives and their build manifests before release."""

import argparse
import hashlib
import json
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    parser.add_argument("--tag", required=True)
    parser.add_argument("--commit", required=True)
    args = parser.parse_args()
    platforms = set()
    for file in args.directory.rglob("*.build.json"):
        metadata = json.loads(file.read_text())
        archive = metadata["archive"]
        if Path(archive).name != archive:
            raise RuntimeError("Unsafe release archive name")
        path = file.parent / archive
        checksum = hashlib.sha256(path.read_bytes()).hexdigest()
        recorded = (path.parent / (path.name + ".sha256")).read_text().strip()
        if recorded != checksum + "  " + archive or checksum != metadata["sha256"]:
            raise RuntimeError("Release archive checksum mismatch")
        if args.tag != "desktop-v" + metadata["version"] or metadata["commit"] != args.commit:
            raise RuntimeError("Release version/commit mismatch")
        if metadata["catalogue_included"] or metadata["onspd_included"]:
            raise RuntimeError("Release must not contain original source data")
        platform = metadata["platform"] + "-" + metadata["architecture"]
        if platform in platforms:
            raise RuntimeError("Duplicate native package")
        platforms.add(platform)
    if platforms != {"darwin-arm64", "darwin-x64", "linux-x64"}:
        raise RuntimeError("All three tested native packages are required for release")
    print("All native release packages match checksums, version and commit.")


if __name__ == "__main__":
    main()
