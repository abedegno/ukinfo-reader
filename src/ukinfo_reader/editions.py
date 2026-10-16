"""Verified editions. Add releases only after independent format validation."""

import hashlib
from dataclasses import dataclass
from pathlib import Path

from .codecs import FormatError


class UnsupportedEditionError(FormatError):
    """The files do not match a supported release profile."""


@dataclass(frozen=True)
class Edition:
    """An independently verified pair of database files."""

    id: str
    title: str
    files: tuple[tuple[str, int, str], ...]

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
SUPPORTED_EDITIONS = (SE4_PCF94,)


def identify_edition(directory):
    """Select a candidate by exact file sizes; Edition.verify confirms its hashes.

    Unknown releases are deliberately rejected rather than decoded speculatively.
    """
    directory = Path(directory)
    if (directory / "Uk1.idx").is_file() and (directory / "Ukdemo.dat").is_file():
        raise UnsupportedEditionError(
            "Found the PC Format 78 filename pair (Uk1.idx, Ukdemo.dat). "
            "That earlier format is unsupported; see docs/pcf78-investigation.md. "
            "Currently supported: UK-Info Disk SE 4.0, PC Format 94, April 1999."
        )
    sizes = {name: (directory / name).stat().st_size for name in ("ukid.idx", "ukid.dat")}
    for edition in SUPPORTED_EDITIONS:
        if all(sizes[name] == size for name, size, _ in edition.files):
            return edition
    raise UnsupportedEditionError(
        "Unrecognized edition or incomplete files. Currently supported: "
        + "; ".join(edition.title for edition in SUPPORTED_EDITIONS)
        + ". See docs/adding-editions.md."
    )
