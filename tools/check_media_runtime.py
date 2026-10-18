"""Check frozen ISO/7z dependency loading with tiny invented, unsupported discs."""

import subprocess
import sys
import tempfile
from pathlib import Path

import py7zr
import pycdlib


def main():
    decoder = Path(sys.argv[1]).resolve()
    with tempfile.TemporaryDirectory(prefix="ukinfo-media-runtime-") as temporary:
        root = Path(temporary)
        image = root / "invented.iso"
        iso = pycdlib.PyCdlib()
        iso.new()
        iso.write(str(image))
        iso.close()
        archive = root / "invented.7z"
        with py7zr.SevenZipFile(archive, "w") as output:
            output.write(image, arcname="nested/invented.iso")
        for source in (image, archive):
            target = root / "output"
            result = subprocess.run(
                [str(decoder), "--prepare-media", str(source), "--output-folder", str(target)],
                capture_output=True,
                text=True,
                timeout=60,
            )
            if (
                result.returncode == 0
                or "UnsupportedEditionError" not in result.stderr
                or "ModuleNotFoundError" in result.stderr
            ):
                raise RuntimeError(
                    "Frozen media reader failed dependency/unsupported-disc check: "
                    + result.stderr[-2000:]
                )
            if target.exists():
                raise RuntimeError("Failed preparation left temporary output")
        result = subprocess.run(
            [str(decoder.with_name("ukinfo-cabinet")), "list", str(image)],
            capture_output=True,
            timeout=30,
        )
        if result.returncode != 3:
            raise RuntimeError("Cabinet helper did not reject a non-cabinet file")
    print("Frozen ISO/7z readers load and clean unsupported discs; cabinet helper executes.")


if __name__ == "__main__":
    main()
