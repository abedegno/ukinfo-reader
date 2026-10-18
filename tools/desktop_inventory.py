"""Inventory the staged native runtime, without build paths or user data.

This is an auditable component inventory, not a completeness certificate or a
vulnerability verdict. Native suppliers' inventories and upstream advisories remain
part of release review (notably statically linked libraries and Electron).
"""

import argparse
import ast
import hashlib
import importlib.metadata
import json
import platform
import subprocess
import sys
from pathlib import Path


def collect(root, output):
    components = [{"name": "CPython", "version": platform.python_version(), "role": "runtime"}]
    packages = importlib.metadata.packages_distributions()
    included = set()

    def walk(value):
        if not isinstance(value, (tuple, list)):
            return
        if len(value) == 3 and isinstance(value[2], str) and value[2] in {"PYMODULE", "EXTENSION"}:
            included.update(packages.get(value[0].split(".")[0], []))
        else:
            for item in value:
                walk(item)

    walk(ast.literal_eval((root / "build/pyinstaller/ukinfo-import/Analysis-00.toc").read_text()))
    # The PyInstaller bootloader is part of the standalone executable.
    included.add("pyinstaller")
    for name in sorted(included, key=str.lower):
        distribution = importlib.metadata.distribution(name)
        components.append(
            {
                "name": distribution.metadata["Name"],
                "version": distribution.version,
                "ecosystem": "pypi",
                "role": "decoder or bootloader",
            }
        )
    go = subprocess.check_output(["go", "version", "-m", str(output / "ukinfo-server")], text=True)
    for line in go.splitlines():
        parts = line.strip().split("\t")
        if parts[0] == "dep" and len(parts) >= 3:
            components.append(
                {
                    "name": parts[1],
                    "version": parts[2],
                    "checksum": parts[3] if len(parts) > 3 else "",
                    "ecosystem": "golang",
                    "role": "server runtime",
                }
            )
    components.append(
        {
            "name": "Go",
            "version": go.splitlines()[0].split()[-1],
            "role": "server toolchain and runtime",
        }
    )
    files = []
    for name in ["ukinfo-server", "ukinfo-import", "ukinfo-cabinet"]:
        file = output / name
        with file.open("rb") as stream:
            digest = hashlib.file_digest(stream, "sha256").hexdigest()
        files.append({"name": name, "sha256": digest, "bytes": file.stat().st_size})
    value = {
        "format": "ukinfo-runtime-inventory",
        "version": 1,
        "platform": sys.platform,
        "architecture": platform.machine(),
        "components": components,
        "executables": files,
        "postgresql": json.loads((output / "pg/BUILD.json").read_text()),
        "cabinet": json.loads((output / "cabinet-notices/BUILD.json").read_text()),
        "decoder_native_suppliers": json.loads(
            (output / "notices/native-dependencies.json").read_text()
        ),
        "limitations": [
            "OS libraries and statically linked subcomponents need supplier advisory review.",
            "Python packages are identified from the decoder analysis, not every build-environment package.",
            "Electron/Chromium/Node versions are added during native packaging.",
        ],
    }
    (output / "components.json").write_text(json.dumps(value, indent=2) + "\n")
    return value


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=Path("build/desktop"))
    args = parser.parse_args()
    collect(Path(__file__).resolve().parents[1], args.output.resolve())


if __name__ == "__main__":
    main()
