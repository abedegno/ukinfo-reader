"""Stage a host-native Go server and an operator-supplied PostgreSQL 18 runtime.

The PostgreSQL directory must include bin, lib, share and its redistribution notices.
No binaries or archives are downloaded; use a trusted, patched native distribution.
"""

import argparse
import importlib.metadata
import shutil
import subprocess
import sys
import sysconfig
from pathlib import Path


def stage_notices(root, output):
    """Retain available source notices; binary distributors still review runtimes."""
    notices = output / "notices"
    notices.mkdir(exist_ok=True)
    shutil.copy2(root / "desktop/THIRD_PARTY_NOTICES.md", notices)
    python_license = Path(sysconfig.get_path("stdlib")) / "LICENSE.txt"
    if not python_license.is_file():
        raise RuntimeError("Python standard-library LICENSE.txt is required for bundling")
    shutil.copy2(python_license, notices / "Python-LICENSE.txt")
    for name in ("pyinstaller", "altgraph", "macholib"):
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
    stage_notices(root, output)
    if (output / "pg").exists():
        shutil.rmtree(output / "pg")
    shutil.copytree(pg, output / "pg", symlinks=False)
    print(f"Staged {version.strip()} and Go server in {output}")


if __name__ == "__main__":
    main()
