"""Verified editions. Add releases only after independent format validation."""

import hashlib
from dataclasses import dataclass
from pathlib import Path

from .codecs import FormatError


class UnsupportedEditionError(FormatError):
    """The files do not match a supported release profile."""


@dataclass(frozen=True)
class Edition:
    """An independently verified set of database files."""

    id: str
    title: str
    files: tuple[tuple[str, int, str], ...]

    format: str = "se4"

    def verify(self, directory):
        """Check SHA-256 fingerprints, streaming without loading whole files."""
        actual = {}
        for name, _, expected in self.files:
            digest = hashlib.sha256()
            with (Path(directory) / name).open("rb") as stream:
                for chunk in iter(lambda: stream.read(1024 * 1024), b""):
                    digest.update(chunk)
            actual[name] = digest.hexdigest()
            if actual[name] != expected:
                raise UnsupportedEditionError(f"{name}: SHA-256 differs from {self.id}")
        return actual


SE4_PCF94 = Edition(
    id="se4-pcf94-1999-04",
    title="UK-Info Disk SE 4.0 — PC Format 94, April 1999",
    files=(
        ("ukid.idx", 82609022, "9029772fd7d47e9e55a3c30908c464146c69c1dd6d2f13ec0d5fda2040080195"),
        ("ukid.dat", 496659177, "9e1eb85ecac003f3bc4cf74f050f984860dcd502c5be3f847eca8bb95faf0ce6"),
    ),
)
PCF78 = Edition(
    id="se-pcf78-1998-01",
    title="UK-Info Disk SE — PC Format 78, January 1998",
    files=(
        ("Uk1.idx", 109507314, "16beac35d7aa3dfea8a44c848b5f9148e8e11b8ab433325a1f71a94b33095cc7"),
        (
            "Ukdemo.dat",
            329429733,
            "7016525fe30110ed9dd15d5b0d5b17e29667496b766cd5fc90a16780d0985bcd",
        ),
        ("d1.dat", 1052672, "f5fb840dfc8b1ddd78ad617ba9e623d63e43e358aa137e94d449376549edf09d"),
        ("d2.dat", 5799936, "92aee9a144ea724e19ebd2ce3faabb6511f35b9575522069af806cccd3e4309b"),
        ("d3.dat", 3653632, "be17e492a4ebf0f68c8d47c9569147df7e97096f7724a67a15077edc21ba563b"),
        ("d4.dat", 102400, "32dcaa41ce31e790d6cc6fe760d4185c018a1ac35504e8cb294c3a8be656c03a"),
        ("d5.idx", 9949184, "4da5d4740a0815a8a4b9cac46a4188c8867418230c8a53e4e65b87e79f30d757"),
        ("d6.dat", 13975552, "19ddd4cd62fbc0ed7129e39bf37afc41889d0223c61fb9b53c4d51363ec8adf0"),
        ("d7.dat", 9064448, "faf20038dc387880c54ae3fd8c365d9315ddc2fa1cb0d8c491ed28db497f14a6"),
    ),
    format="pcf78",
)
SUPPORTED_EDITIONS = (SE4_PCF94, PCF78)


def identify_edition(directory):
    """Select by exact required filenames/sizes; verify() confirms source hashes."""
    directory = Path(directory)
    for edition in SUPPORTED_EDITIONS:
        if all(
            (directory / name).is_file() and (directory / name).stat().st_size == size
            for name, size, _ in edition.files
        ):
            return edition
    raise UnsupportedEditionError(
        "Unrecognized edition or incomplete files. Currently supported: "
        + "; ".join(edition.title for edition in SUPPORTED_EDITIONS)
        + ". See docs/adding-editions.md."
    )
