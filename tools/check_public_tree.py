"""Reject original media, database exports and large files in Git's staged tree."""

import subprocess
import sys
from pathlib import Path


def main():
    paths = subprocess.check_output(["git", "ls-files", "-z"]).decode("utf-8").split("\0")
    forbidden = {
        ".iso",
        ".bin",
        ".cue",
        ".dat",
        ".idx",
        ".dll",
        ".exe",
        ".cab",
        ".zip",
        ".7z",
        ".csv",
        ".jsonl",
        ".sqlite",
        ".db",
    }
    errors = []
    for name in filter(None, paths):
        path = Path(name)
        if (
            path.parts[0] in {"private", "data", "extracted", ".venv"}
            or path.suffix.lower() in forbidden
        ):
            errors.append(f"Forbidden source/data artifact: {name}")
        if path.is_file() and path.stat().st_size > 1024 * 1024:
            errors.append(f"File exceeds 1 MiB: {name}")
    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    print("Public tree check passed: no source media, database files or exports are tracked.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
