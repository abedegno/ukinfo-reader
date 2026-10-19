"""Archive a tested, source-data-free native desktop bundle with build provenance."""

import argparse
import hashlib
import json
import platform
import re
import subprocess
import tarfile
from pathlib import Path

if __package__:
    from .macos_signing import notarize, ticket_check
else:
    from macos_signing import notarize, ticket_check


def macos_signing_state(app, submit):
    """Verify this app before archiving; submission performs the full Apple gate."""
    subprocess.run(["codesign", "--verify", "--deep", "--strict", str(app)], check=True)
    result = subprocess.run(
        ["codesign", "-dv", "--verbose=4", str(app)], capture_output=True, text=True, check=True
    )
    developer_id = "Authority=Developer ID Application:" in result.stderr
    if submit and not developer_id:
        raise RuntimeError("Notarization requires a Developer ID signed app")
    if developer_id:
        if submit:
            # This call requires Accepted, stapling, ticket validation, strict
            # signatures and Gatekeeper. No flag/previous-step status substitutes
            # for those checks. Archive this same app immediately afterwards.
            notarize(app)
        else:
            ticket_check(["xcrun", "stapler", "validate", str(app)])
        return "Developer ID, notarized and stapled"
    return "ad-hoc, not notarized"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=Path("dist/releases"))
    parser.add_argument(
        "--notarize",
        action="store_true",
        help="Run the complete Apple acceptance/ticket/signature/Gatekeeper gate before archiving",
    )
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    version = json.loads((root / "desktop/package.json").read_text())["version"]
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+(?:-[a-zA-Z0-9.-]+)?", version):
        parser.error("desktop version must be a safe semantic version")
    system = {"Darwin": "darwin", "Linux": "linux"}.get(platform.system())
    arch = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "x64", "AMD64": "x64"}.get(
        platform.machine()
    )
    if not system or not arch:
        parser.error("unsupported native platform")
    if args.notarize and system != "darwin":
        parser.error("notarization requires macOS")
    bundle = root / f"dist/desktop/UK-Info Archive-{system}-{arch}"
    if not bundle.is_dir():
        parser.error("run the packaged app check before creating its release archive")
    for file in bundle.rglob("*"):
        if file.is_file() and file.suffix.lower() in {
            ".iso",
            ".7z",
            ".csv",
            ".jsonl",
            ".db",
            ".sqlite",
            ".parquet",
        }:
            raise RuntimeError(f"Source/data artifact in release bundle: {file.name}")
        if file.name in {
            "PG_VERSION",
            "research-lists.json",
            "research-lists.v1-backup.json",
            "workbench.json",
            "research-drafts.json",
            "setup-state.json",
            "password",
        }:
            raise RuntimeError("User data must not be packaged")
    args.output.mkdir(parents=True, exist_ok=True)
    label = f"ukinfo-archive-{version}-{system}-{arch}"
    archive = args.output / (label + (".zip" if system == "darwin" else ".tar.gz"))
    signing = "unsigned"
    if system == "darwin":
        app = bundle / "UK-Info Archive.app"
        signing = macos_signing_state(app, args.notarize)
        subprocess.run(
            [
                "ditto",
                "-c",
                "-k",
                "--sequesterRsrc",
                "--keepParent",
                str(app),
                str(archive.resolve()),
            ],
            check=True,
        )
        runtime = app / "Contents/Resources/runtime"
    else:
        with tarfile.open(archive, "w:gz") as target:
            target.add(bundle, arcname=bundle.name)
        runtime = bundle / "resources/runtime"
    manifest = {
        "version": version,
        "commit": subprocess.check_output(
            ["git", "rev-parse", "HEAD"], cwd=root, text=True
        ).strip(),
        "platform": system,
        "architecture": arch,
        "python": platform.python_version(),
        "go": subprocess.check_output(["go", "version"], text=True).strip(),
        "electron": json.loads((root / "desktop/package.json").read_text())["devDependencies"][
            "electron"
        ],
        "postgresql": json.loads((runtime / "pg/BUILD.json").read_text()),
        "cabinet": json.loads((runtime / "cabinet-notices/BUILD.json").read_text()),
        "components": json.loads((runtime / "components.json").read_text()),
        "media_readers": {"pycdlib": "1.22.0", "py7zr": "1.1.4"},
        "signing": signing,
        "catalogue_included": False,
        "onspd_included": False,
        "archive": archive.name,
        "sha256": hashlib.sha256(archive.read_bytes()).hexdigest(),
    }
    (args.output / (label + ".build.json")).write_text(json.dumps(manifest, indent=2) + "\n")
    (args.output / (archive.name + ".sha256")).write_text(
        manifest["sha256"] + "  " + archive.name + "\n"
    )
    print(archive)


if __name__ == "__main__":
    main()
