"""Stage a host-native Go server and an operator-supplied PostgreSQL 18 runtime.

The PostgreSQL directory must include bin, lib, share and its redistribution notices.
No binaries or archives are downloaded; use a trusted, patched native distribution.
"""

import argparse
import shutil
import subprocess
from pathlib import Path


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
    if (output / "pg").exists():
        shutil.rmtree(output / "pg")
    shutil.copytree(pg, output / "pg", symlinks=False)
    print(f"Staged {version.strip()} and Go server in {output}")


if __name__ == "__main__":
    main()
