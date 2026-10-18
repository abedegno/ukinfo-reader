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


def public_check(args):
    """Report diagnostics only for commands that use no plaintext credentials."""
    allowed = (
        ("xcrun", "notarytool", "submit"),
        ("xcrun", "notarytool", "log"),
        ("xcrun", "stapler", "staple"),
        ("xcrun", "stapler", "validate"),
        ("codesign", "--verify"),
        ("spctl", "--assess"),
    )
    if not any(tuple(args[: len(prefix)]) == prefix for prefix in allowed):
        raise RuntimeError("Diagnostics are restricted to public signing checks")
    print("Running " + " ".join(args[:3]), flush=True)
    result = subprocess.run(args, capture_output=True, text=True)
    if result.returncode:
        raise RuntimeError(
            f"{' '.join(args[:3])} failed (exit {result.returncode}):\n"
            + result.stdout[-16000:]
            + result.stderr[-16000:]
        )
    return result.stdout


def runner_root():
    if sys.platform != "darwin" or os.environ.get("GITHUB_ACTIONS") != "true":
        raise RuntimeError("This credential helper runs only on a macOS GitHub Actions runner")
    return Path(os.environ["RUNNER_TEMP"]).resolve()


def decode_material(value):
    # electron-builder's usual CSC_LINK form is the base64 PKCS#12 content.
    # Also accept its base64 data-URL form; credentials are never printed.
    if value.startswith("data:"):
        header, value = value.split(",", 1)
        if not header.endswith(";base64"):
            raise RuntimeError("Certificate data URL must contain base64 material")
    try:
        if len(value) > 10 << 20:
            raise ValueError("oversized credential")
        return base64.b64decode("".join(value.split()), validate=True)
    except (ValueError, TypeError):
        raise RuntimeError("Signing material must be base64-encoded file content") from None


def signing_identity(listing, team):
    matches = re.findall(
        r'\b([A-Fa-f0-9]{40})\s+"Developer ID Application: [^"\r\n]+\(' + re.escape(team) + r'\)"',
        listing,
    )
    if len(matches) != 1:
        raise RuntimeError("Expected one valid Developer ID Application identity for APPLE_TEAM_ID")
    return matches[0]


def prepare():
    root = runner_root()
    names = (
        "CSC_LINK",
        "CSC_KEY_PASSWORD",
        "APPLE_API_KEY",
        "APPLE_API_KEY_ID",
        "APPLE_API_ISSUER",
        "APPLE_TEAM_ID",
    )
    if any(not os.environ.get(name) for name in names):
        raise RuntimeError(
            "Configure all six signing/notarization secrets in the desktop-signing environment"
        )
    team = os.environ["APPLE_TEAM_ID"]
    if not re.fullmatch(r"[A-Za-z0-9]{10}", team):
        raise RuntimeError("APPLE_TEAM_ID must contain ten alphanumeric characters")
    certificate_bytes = decode_material(os.environ["CSC_LINK"])
    api_bytes = decode_material(os.environ["APPLE_API_KEY"])
    if not api_bytes.startswith(b"-----BEGIN PRIVATE KEY-----"):
        raise RuntimeError("APPLE_API_KEY must encode an App Store Connect .p8 private key")
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
        certificate.write_bytes(certificate_bytes)
        certificate.chmod(0o600)
        run(
            [
                "security",
                "import",
                str(certificate),
                "-k",
                str(keychain),
                "-P",
                os.environ["CSC_KEY_PASSWORD"],
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
    identity = signing_identity(
        run(["security", "find-identity", "-v", "-p", "codesigning", str(keychain)]), team
    )
    api_key = root / "ukinfo-notary-key.p8"
    descriptor = os.open(api_key, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(descriptor, "wb") as stream:
        stream.write(api_bytes)
    run(
        [
            "xcrun",
            "notarytool",
            "store-credentials",
            "ukinfo-notary",
            "--key",
            str(api_key),
            "--key-id",
            os.environ["APPLE_API_KEY_ID"],
            "--issuer",
            os.environ["APPLE_API_ISSUER"],
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
            public_check(
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
        print("Apple submission status: " + str(report.get("status")), flush=True)
        if report.get("status") != "Accepted":
            submission = report.get("id", "")
            if re.fullmatch(r"[A-Fa-f0-9-]{36}", submission):
                print(
                    public_check(
                        [
                            "xcrun",
                            "notarytool",
                            "log",
                            submission,
                            "--keychain-profile",
                            "ukinfo-notary",
                            "--keychain",
                            str(keychain),
                        ]
                    ),
                    flush=True,
                )
            raise RuntimeError(
                "Apple did not accept notarization; inspect the submission in the Apple developer account"
            )
    public_check(["xcrun", "stapler", "staple", str(app)])
    public_check(["xcrun", "stapler", "validate", str(app)])
    public_check(["codesign", "--verify", "--deep", "--strict", str(app)])
    public_check(["spctl", "--assess", "--type", "execute", str(app)])
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
        try:
            if keychain.exists():
                run(["security", "delete-keychain", str(keychain)])
        finally:
            (root / "ukinfo-notary-key.p8").unlink(missing_ok=True)


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
