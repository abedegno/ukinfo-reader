"""Selective, bounded extraction of supported database files from ISO/7z media.

Archive names are lookup keys only: output paths come from verified edition
profiles. No autorun programs, installers, links or archive paths are executed.
"""

import json
import os
import shutil
import subprocess
import sys
import tempfile
import time
from pathlib import Path, PurePosixPath

from .editions import SUPPORTED_EDITIONS, UnsupportedEditionError

RESERVE = 5 << 30
MAX_ISO = 9 << 30
MAX_MEMBERS = 100_000


def check_space(directory, required=0):
    if shutil.disk_usage(directory).free < RESERVE + required:
        raise ValueError("Not enough temporary space: retain at least 5 GiB free after extraction")


def progress(phase, done=0, total=0):
    import sys

    print(
        "UKINFO_PROGRESS " + json.dumps(dict(phase=phase, done=done, total=total)),
        file=sys.stderr,
        flush=True,
    )


def candidates(members):
    """Require a complete profile in one directory; reject ambiguous media."""
    groups = {}
    for name, size in members:
        path = PurePosixPath(name.replace("\\", "/"))
        key = path.name.split(";", 1)[0].lower()
        files = groups.setdefault(str(path.parent), {})
        if key in files:
            raise ValueError("Ambiguous duplicate filenames in disc directory")
        files[key] = (name, size)
    matches = []
    for files in groups.values():
        for edition in SUPPORTED_EDITIONS:
            if all(
                name.lower() in files and files[name.lower()][1] == size
                for name, size, _ in edition.files
            ):
                matches.append(
                    (edition, {name: files[name.lower()][0] for name, _, _ in edition.files})
                )
    if len(matches) != 1:
        raise UnsupportedEditionError(
            "Expected one supported UK-Info database folder on this disc; found "
            + str(len(matches))
        )
    return matches[0]


class BoundedWriter:
    """A sequential, size-limited destination with continuous free-space checks."""

    def __init__(self, path, limit):
        self.path = path
        self.limit = limit
        self.stream = path.open("xb")
        self.count = 0
        self.reported = 0.0

    def write(self, data):
        if self.count + len(data) > self.limit:
            raise ValueError("Archive member exceeds its declared size")
        check_space(self.path.parent, len(data))
        written = self.stream.write(data)
        self.count += written
        now = time.monotonic()
        if now - self.reported > 0.3:
            progress("extracting disc files", self.count, self.limit)
            self.reported = now
        return written

    def seekable(self):
        return False

    def read(self, size=None):
        raise ValueError("Extraction destination is write-only")

    def seek(self, offset, whence=0):
        if whence == 0 and offset == self.count:
            return self.count
        if whence == 1 and offset == 0:
            return self.count
        raise ValueError("Non-sequential extraction is unsupported")

    def flush(self):
        if not self.stream.closed:
            self.stream.flush()

    def size(self):
        return self.count

    def close(self):
        self.stream.close()


def cabinet_helper():
    configured = os.environ.get("UKINFO_CABINET")
    bundled = Path(sys.executable).with_name("ukinfo-cabinet")
    development = Path(__file__).resolve().parents[2] / "build/desktop/ukinfo-cabinet"
    for candidate in (Path(configured) if configured else bundled, bundled, development):
        if candidate.is_file():
            return candidate
    raise ValueError("Cabinet extraction helper is missing; use an extracted source folder")


def extract_cabinet(cabinet, destination, edition):
    import resource

    helper = cabinet_helper()
    listing = subprocess.run(
        [str(helper), "list", str(cabinet)], capture_output=True, check=True, timeout=30
    )
    members = {}
    for line in listing.stdout.decode("utf-8").splitlines():
        index, size, name = line.split("\t", 2)
        key = PurePosixPath(name.replace("\\", "/")).name.lower()
        members.setdefault(key, []).append((int(index), int(size)))
    for name, size, _ in edition.files:
        if (destination / name).is_file():
            continue
        matches = [
            (index, length) for index, length in members.get(name.lower(), []) if length == size
        ]
        if len(matches) != 1:
            raise ValueError("Required database table missing or ambiguous in cabinet: " + name)
        index, _ = matches[0]
        check_space(destination, size)
        progress("unpacking database tables")

        def limit_output():
            resource.setrlimit(resource.RLIMIT_FSIZE, (size, size))

        # No cabinet path is an output path; the helper saves one numeric entry
        # to a canonical profile filename. Bound output size in the child kernel.
        child = subprocess.Popen(
            [str(helper), "save", str(cabinet), str(index), str(destination / name)],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            preexec_fn=limit_output,
        )
        try:
            while child.poll() is None:
                check_space(destination)
                time.sleep(0.1)
            if child.returncode or (destination / name).stat().st_size != size:
                raise ValueError("Cabinet extraction failed: " + name)
        finally:
            if child.poll() is None:
                child.terminate()
                try:
                    child.wait(timeout=3)
                except subprocess.TimeoutExpired:
                    child.kill()
                    child.wait()


def iso_profile(members):
    try:
        return (*candidates(members), None)
    except UnsupportedEditionError:
        # Modern cover discs store large base files beside InstallShield tables.
        matches = []
        for edition in SUPPORTED_EDITIONS:
            if edition.format != "modern":
                continue
            anchor_size = next(size for name, size, _ in edition.files if name == "uk.dat")
            for anchor, size in members:
                if (
                    PurePosixPath(anchor).name.split(";", 1)[0].lower() != "uk.dat"
                    or size != anchor_size
                ):
                    continue
                parent = str(PurePosixPath(anchor).parent)
                group = {
                    PurePosixPath(name).name.split(";", 1)[0].lower(): (name, length)
                    for name, length in members
                    if str(PurePosixPath(name).parent) == parent
                }
                if not all(name in group for name in ("data1.cab", "data1.hdr")):
                    continue
                targets = {
                    name: group[name.lower()][0]
                    for name, length, _ in edition.files
                    if name.lower() in group and group[name.lower()][1] == length
                }
                cabinets = {
                    name: value
                    for name, value in group.items()
                    if name in ("data1.cab", "data1.hdr", "data2.cab")
                }
                if any(not 0 < length <= 1 << 30 for _, length in cabinets.values()):
                    raise ValueError("Oversized cabinet")
                matches.append((edition, targets, cabinets))
        if len(matches) != 1:
            raise UnsupportedEditionError(
                "Disc does not contain one supported UK-Info edition"
            ) from None
        return matches[0]


def extract_iso(source, destination):
    import pycdlib

    image = pycdlib.PyCdlib()
    image.open(str(source))
    try:
        kind = (
            "joliet_path"
            if image.has_joliet()
            else "rr_path"
            if image.has_rock_ridge()
            else "iso_path"
        )
        members = []
        for parent, _, files in image.walk(**{kind: "/"}):
            for name in files:
                path = str(PurePosixPath(parent) / name)
                record = image.get_record(**{kind: path})
                if record.is_symlink():
                    continue
                members.append((path, record.data_length))
                if len(members) > MAX_MEMBERS:
                    raise ValueError("Too many disc files")
        edition, targets, cabinets = iso_profile(members)
        check_space(
            destination,
            sum(size for _, size, _ in edition.files)
            + sum(n for _, n in (cabinets or {}).values()),
        )
        for name, size, _ in edition.files:
            if name not in targets:
                continue
            writer = BoundedWriter(destination / name, size)
            try:
                with image.open_file_from_iso(**{kind: targets[name]}) as stream:
                    while chunk := stream.read(1 << 20):
                        writer.write(chunk)
                if writer.count != size:
                    raise ValueError("Truncated ISO member")
            finally:
                writer.close()
        if cabinets:
            with tempfile.TemporaryDirectory(prefix="cabinets-", dir=destination) as temporary:
                for name, (path, size) in cabinets.items():
                    writer = BoundedWriter(Path(temporary) / name, size)
                    try:
                        with image.open_file_from_iso(**{kind: path}) as stream:
                            while chunk := stream.read(1 << 20):
                                writer.write(chunk)
                        if writer.count != size:
                            raise ValueError("Truncated cabinet")
                    finally:
                        writer.close()
                extract_cabinet(Path(temporary) / "data1.cab", destination, edition)
        return edition
    finally:
        image.close()


def extract_7z(source, destination):
    import py7zr
    from py7zr.io import WriterFactory

    writers = []

    class Factory(WriterFactory):
        def __init__(self, targets):
            self.targets = targets
            self.seen = set()

        def create(self, filename):
            if filename not in self.targets or filename in self.seen:
                raise ValueError("Unexpected extraction destination")
            self.seen.add(filename)
            name, size = self.targets[filename]
            writer = BoundedWriter(destination / name, size)
            writers.append(writer)
            return writer

    with py7zr.SevenZipFile(source) as archive:
        if archive.needs_password():
            raise ValueError("Encrypted archives are unsupported")
        entries = archive.list()
        if len(entries) > MAX_MEMBERS:
            raise ValueError("Too many archive members")
        files = [
            (item.filename, item.uncompressed)
            for item in entries
            if item.is_file and not item.is_symlink
        ]
        if len({name for name, _ in files}) != len(files):
            raise ValueError("Duplicate archive members")
        # Prefer database files stored directly in a 7z; otherwise accept one ISO.
        try:
            edition, selected = candidates(files)
        except UnsupportedEditionError:
            images = [(name, size) for name, size in files if name.lower().endswith(".iso")]
            if len(images) != 1 or not 0 < images[0][1] <= MAX_ISO:
                raise ValueError(
                    "Choose a supported database archive or a 7z containing one ISO"
                ) from None
            name, size = images[0]
            # Reserve room for the image and the largest supported database set.
            check_space(
                destination, size + max(sum(n for _, n, _ in e.files) for e in SUPPORTED_EDITIONS)
            )
            targets = {name: ("disc.iso", size)}
            edition = None
        else:
            check_space(destination, sum(size for _, size, _ in edition.files))
            sizes = {name: size for name, size, _ in edition.files}
            targets = {member: (name, sizes[name]) for name, member in selected.items()}
        factory = Factory(targets)
        try:
            archive.extract(targets=list(targets), recursive=False, factory=factory)
            if factory.seen != set(targets) or any(
                writer.count != writer.limit for writer in writers
            ):
                raise ValueError("Incomplete archive extraction")
        finally:
            for writer in writers:
                writer.close()
    if edition is None:
        image = destination / "disc.iso"
        try:
            return extract_iso(image, destination)
        finally:
            image.unlink(missing_ok=True)
    return edition


def prepare(source, destination):
    """Extract into a new directory and verify every required SHA-256."""
    source, destination = Path(source), Path(destination)
    if source.suffix.lower() not in (".iso", ".7z") or not source.is_file():
        raise ValueError("Choose a completed ISO or 7z file")
    destination.mkdir(mode=0o700)  # refuse existing directories, even empty ones
    try:
        check_space(destination)
        progress("reading disc directory")
        edition = (
            extract_iso(source, destination)
            if source.suffix.lower() == ".iso"
            else extract_7z(source, destination)
        )
        progress("verifying disc files")
        edition.verify(destination)
        return edition
    except BaseException:
        shutil.rmtree(destination)
        raise
