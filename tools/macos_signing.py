"""Ephemeral macOS CI keychain setup and notarization. Never print credential arguments."""

import base64
import json
import os
import re
import secrets
import shlex
import subprocess
import sys
import tempfile
from pathlib import Path


def run(args):
    result = subprocess.run(args, capture_output=True, text=True)
    if result.returncode:
        # CalledProcessError includes argv, which may contain passwords.
        raise RuntimeError(
            f"{Path(args[0]).name} failed (exit {result.returncode}); credentials were not logged"
        )
    return result.stdout


def runner_root():
    if sys.platform != "darwin" or os.environ.get("GITHUB_ACTIONS") != "true":
        raise RuntimeError("This credential helper runs only on a macOS GitHub Actions runner")
    return Path(os.environ["RUNNER_TEMP"]).resolve()


def prepare():
    root = runner_root()
    names = (
        "MAC_CERT_P12_BASE64",
        "MAC_CERT_PASSWORD",
        "MAC_CODESIGN_IDENTITY",
        "APPLE_ID",
        "APPLE_TEAM_ID",
        "APPLE_APP_PASSWORD",
    )
    if any(not os.environ.get(name) for name in names):
        raise RuntimeError(
            "Configure all six signing/notarization secrets in the desktop-signing environment"
        )
    identity = os.environ["MAC_CODESIGN_IDENTITY"]
    if not re.fullmatch(r"[a-fA-F0-9]{40}", identity):
        raise RuntimeError(
            "MAC_CODESIGN_IDENTITY must be the Developer ID certificate's SHA-1 fingerprint"
        )
    keychain = root / "ukinfo-signing.keychain-db"
    saved = root / "ukinfo-keychain-search.json"
    original = shlex.split(run(["security", "list-keychains", "-d", "user"]))
    saved.write_text(json.dumps(original))
    password = secrets.token_urlsafe(32)
    run(["security", "create-keychain", "-p", password, str(keychain)])
    run(["security", "set-keychain-settings", "-lut", "21600", str(keychain)])
    run(["security", "unlock-keychain", "-p", password, str(keychain)])
    with tempfile.TemporaryDirectory(dir=root, prefix="ukinfo-cert-") as temporary:
        certificate = Path(temporary) / "identity.p12"
        certificate.write_bytes(base64.b64decode(os.environ["MAC_CERT_P12_BASE64"], validate=True))
        certificate.chmod(0o600)
        run(
            [
                "security",
                "import",
                str(certificate),
                "-k",
                str(keychain),
                "-P",
                os.environ["MAC_CERT_PASSWORD"],
                "-T",
                "/usr/bin/codesign",
                "-T",
                "/usr/bin/security",
            ]
        )
    run(
        [
            "security",
            "set-key-partition-list",
            "-S",
            "apple-tool:,apple:,codesign:",
            "-s",
            "-k",
            password,
            str(keychain),
        ]
    )
    run(["security", "list-keychains", "-d", "user", "-s", str(keychain), *original])
    run(
        [
            "xcrun",
            "notarytool",
            "store-credentials",
            "ukinfo-notary",
            "--apple-id",
            os.environ["APPLE_ID"],
            "--team-id",
            os.environ["APPLE_TEAM_ID"],
            "--password",
            os.environ["APPLE_APP_PASSWORD"],
            "--keychain",
            str(keychain),
        ]
    )
    with Path(os.environ["GITHUB_ENV"]).open("a") as stream:
        stream.write(f"UKINFO_CODESIGN_KEYCHAIN={keychain}\nUKINFO_CODESIGN_IDENTITY={identity}\n")
    print("Temporary signing keychain prepared.")


def notarize(app):
    root = runner_root()
    app = app.resolve(strict=True)
    if not app.name.endswith(".app"):
        raise RuntimeError("Choose the packaged application")
    keychain = root / "ukinfo-signing.keychain-db"
    with tempfile.TemporaryDirectory(dir=root, prefix="ukinfo-notary-") as temporary:
        archive = Path(temporary) / "application.zip"
        run(["ditto", "-c", "-k", "--keepParent", str(app), str(archive)])
        report = json.loads(
            run(
                [
                    "xcrun",
                    "notarytool",
                    "submit",
                    str(archive),
                    "--keychain-profile",
                    "ukinfo-notary",
                    "--keychain",
                    str(keychain),
                    "--wait",
                    "--timeout",
                    "30m",
                    "--output-format",
                    "json",
                ]
            )
        )
        if report.get("status") != "Accepted":
            raise RuntimeError(
                "Apple did not accept notarization; inspect the submission in the Apple developer account"
            )
    run(["xcrun", "stapler", "staple", str(app)])
    run(["xcrun", "stapler", "validate", str(app)])
    run(["codesign", "--verify", "--deep", "--strict", str(app)])
    run(["spctl", "--assess", "--type", "execute", str(app)])
    print("Developer ID app notarized, stapled and accepted by Gatekeeper.")


def cleanup():
    root = runner_root()
    saved = root / "ukinfo-keychain-search.json"
    keychain = root / "ukinfo-signing.keychain-db"
    try:
        if saved.exists():
            run(["security", "list-keychains", "-d", "user", "-s", *json.loads(saved.read_text())])
            saved.unlink()
    finally:
        if keychain.exists():
            run(["security", "delete-keychain", str(keychain)])


if __name__ == "__main__":
    try:
        if sys.argv[1] == "prepare":
            prepare()
        elif sys.argv[1] == "cleanup":
            cleanup()
        elif sys.argv[1] == "notarize":
            notarize(Path(sys.argv[2]))
        else:
            raise RuntimeError("Expected prepare, notarize APP or cleanup")
    except Exception as error:
        # Our run helper never includes subprocess argv in this message.
        print(str(error), file=sys.stderr)
        sys.exit(1)
