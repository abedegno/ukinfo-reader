"""Stage a host-native Go server and an operator-supplied PostgreSQL 18 runtime.

The PostgreSQL directory must include bin, lib, share and its redistribution notices.
No executables are downloaded; use a trusted, patched native distribution. Matching
CPython borrowed-code notice text is fetched from the official versioned source.
"""

import argparse
import importlib.metadata
import shutil
import subprocess
import sys
import sysconfig
from pathlib import Path

from build_cabinet_runtime import build as build_cabinet
from desktop_notices import dependency_notices


def stage_notices(root, output, python_license=None):
    """Retain available source notices; binary distributors still review runtimes."""
    notices = output / "notices"
    if notices.exists():
        shutil.rmtree(notices)
    notices.mkdir()
    shutil.copy2(root / "desktop/THIRD_PARTY_NOTICES.md", notices)
    if python_license is None:
        candidates = [
            Path(sysconfig.get_path("stdlib")) / "LICENSE.txt",
            Path(sys.base_prefix) / "LICENSE.txt",
            Path(sys.base_prefix) / "LICENSE",
        ]
        python_license = next((p for p in candidates if p.is_file()), None)
    if python_license is None or not python_license.is_file():
        raise RuntimeError(
            "Provide --python-license with this Python distribution's full license notice"
        )
    shutil.copy2(python_license, notices / "Python-LICENSE.txt")
    for name in (
        "pyinstaller",
        "altgraph",
        "macholib",
        "pycdlib",
        "py7zr",
        "texttable",
        "pycryptodomex",
        "brotli",
        "psutil",
        "pyppmd",
        "pybcj",
        "multivolumefile",
        "inflate64",
    ):
        try:
            distribution = importlib.metadata.distribution(name)
        except importlib.metadata.PackageNotFoundError:
            continue
        for item in distribution.files or ():
            if item.name.upper().startswith(("LICENSE", "COPYING", "NOTICE")):
                destination = notices / name / str(item)
                destination.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(distribution.locate_file(item), destination)
    goroot = subprocess.check_output(["go", "env", "GOROOT"], text=True).strip()
    go_license = next(
        (p for p in (Path(goroot) / "LICENSE", Path(goroot).parent / "LICENSE") if p.is_file()),
        None,
    )
    if go_license is None:
        raise RuntimeError("Go distribution LICENSE is required for bundling")
    shutil.copy2(go_license, notices / "Go-LICENSE")
    raw = subprocess.check_output(
        [
            "go",
            "list",
            "-deps",
            "-f",
            "{{if and .Module (not .Module.Main)}}{{.Module.Path}}\t{{.Module.Dir}}{{end}}",
            "./cmd/ukinfo-server",
        ],
        cwd=root / "server",
        text=True,
    )
    for module in sorted(set(raw.splitlines()) - {""}):
        module_path, module_directory = module.split("\t", 1)
        directory = Path(module_directory)
        for source in directory.rglob("*"):
            if source.is_file() and source.name.upper().startswith(
                ("LICENSE", "COPYING", "NOTICE")
            ):
                destination = (
                    notices / module_path.replace("/", "__") / source.relative_to(directory)
                )
                destination.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(source, destination)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--postgres", type=Path, required=True, help="PostgreSQL runtime root")
    parser.add_argument(
        "--python-license",
        type=Path,
        help="full CPython license notice if not installed under the interpreter prefix",
    )
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    pg = args.postgres.resolve()
    for name in ("bin/postgres", "bin/initdb", "lib", "share"):
        if not (pg / name).exists():
            parser.error(f"missing {pg / name}")
    version = subprocess.check_output([str(pg / "bin/postgres"), "--version"], text=True)
    if not version.startswith("postgres (PostgreSQL) 18."):
        parser.error("PostgreSQL 18 is required")
    output = root / "build/desktop"
    if pg.is_relative_to(output / "pg"):
        parser.error(
            "supply a PostgreSQL runtime outside build/desktop/pg; staging replaces that directory"
        )
    output.mkdir(parents=True, exist_ok=True)
    subprocess.run(
        ["go", "build", "-trimpath", "-o", str(output / "ukinfo-server"), "./cmd/ukinfo-server"],
        cwd=root / "server",
        check=True,
    )
    subprocess.run(
        [
            sys.executable,
            "-m",
            "PyInstaller",
            "--noconfirm",
            "--onefile",
            "--name",
            "ukinfo-import",
            "--paths",
            str(root / "src"),
            "--collect-submodules",
            "ukinfo_reader",
            "--collect-submodules",
            "py7zr",
            "--collect-submodules",
            "Cryptodome",
            "--distpath",
            str(output),
            "--workpath",
            str(root / "build/pyinstaller"),
            "--specpath",
            str(root / "build"),
            str(root / "tools/importer_entry.py"),
        ],
        cwd=root,
        check=True,
    )
    build_cabinet(output)
    stage_notices(root, output, args.python_license)
    dependency_notices(root, output / "notices")
    if (output / "pg").exists():
        shutil.rmtree(output / "pg")
    shutil.copytree(pg, output / "pg", symlinks=False)
    print(f"Staged {version.strip()} and Go server in {output}")


if __name__ == "__main__":
    main()
