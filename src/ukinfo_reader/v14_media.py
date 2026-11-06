"""Prepare the exact supplied v14 two-disc release without running Windows code."""

import argparse
import hashlib
import shutil
import struct
import tempfile
from pathlib import Path

from .codecs import FormatError
from .media import BoundedWriter, check_space, extract_cabinet, progress

INSTALL_IMAGE = (
    637314768,
    "e6a85e73d17e4c6c49b2b26b072a740baa264e79b0ca830e1c73f19d3b1a3e9a",
    2448,
)
DATA_IMAGE = (481052208, "787f799d074c1ecf015c45ea6fba62f42a8d520bc588e3c69fd39ae4a0db76a3", 2352)
DECODER_HASH = "3424b039abe88e9b7486e9540cc9165ab497d52d927efcab58f12b3299c42dd6"


def _sha256(stream):
    digest = hashlib.sha256()
    for chunk in iter(lambda: stream.read(1024 * 1024), b""):
        digest.update(chunk)
    return digest.hexdigest()


def read_prefixes(data, table_rva, count):
    """Read only a bounded array of source telephone prefixes from PE32 data."""
    try:
        if data[:2] != b"MZ":
            raise FormatError("Invalid v14 decoder header")
        pe = struct.unpack_from("<I", data, 0x3C)[0]
        if data[pe : pe + 4] != b"PE\0\0":
            raise FormatError("Invalid v14 PE signature")
        sections = struct.unpack_from("<H", data, pe + 6)[0]
        optional_size = struct.unpack_from("<H", data, pe + 20)[0]
        optional = pe + 24
        if not 1 <= sections <= 32 or struct.unpack_from("<H", data, optional)[0] != 0x10B:
            raise FormatError("Invalid v14 PE32 layout")
        base = struct.unpack_from("<I", data, optional + 28)[0]
        mappings = []
        for index in range(sections):
            _, rva, size, offset = struct.unpack_from(
                "<IIII", data, optional + optional_size + index * 40 + 8
            )
            if offset + size > len(data):
                raise FormatError("Truncated v14 PE section")
            mappings.append((rva, size, offset))

        def locate(rva, size):
            for start, length, offset in mappings:
                if start <= rva and rva + size <= start + length:
                    return offset + rva - start
            raise FormatError("v14 telephone metadata is outside PE sections")

        if not 1 <= count <= 4096:
            raise FormatError("Invalid v14 prefix count")
        table = locate(table_rva, count * 4)
        prefixes = []
        for index in range(count):
            pointer = struct.unpack_from("<I", data, table + index * 4)[0] - base
            start = locate(pointer, 1)
            end = data.find(b"\0", start, start + 17)
            if end <= start:
                raise FormatError("Invalid v14 prefix text")
            locate(pointer, end - start + 1)
            text = bytes(data[start:end])
            if any(c < 32 or c > 126 for c in text):
                raise FormatError("Non-ASCII v14 prefix text")
            prefixes.append(text)
        return b"\0".join(prefixes) + b"\0"
    except (struct.error, IndexError) as exc:
        raise FormatError("Truncated v14 telephone metadata") from exc


def extract_prefixes(decoder):
    data = Path(decoder).read_bytes()
    if hashlib.sha256(data).hexdigest() != DECODER_HASH:
        raise FormatError("Unrecognized v14 decoder identity")
    return read_prefixes(data, 0x4C658, 3078)


def extract_disc(path, identity, targets, destination):
    """Extract selected flat-root ISO files from an exactly fingerprinted raw CD."""
    path, destination = Path(path), Path(destination)
    size, expected, stride = identity
    if path.stat().st_size != size:
        raise FormatError("Unrecognized v14 disc size")
    with path.open("rb") as stream:
        progress("verifying v14 disc")
        if _sha256(stream) != expected:
            raise FormatError("Unrecognized v14 disc fingerprint")

        def read(lba, length):
            result = bytearray()
            while length:
                stream.seek(lba * stride + 24)
                chunk = stream.read(min(length, 2048))
                if not chunk:
                    raise FormatError("Truncated v14 raw CD")
                result.extend(chunk)
                length -= len(chunk)
                lba += 1
            return result

        pvd = read(16, 2048)
        if pvd[:7] != b"\x01CD001\x01":
            raise FormatError("Invalid v14 ISO descriptor")
        root = pvd[156:190]
        directory_size = int.from_bytes(root[10:14], "little")
        if not 34 <= directory_size <= 1 << 20:
            raise FormatError("Invalid v14 root directory bounds")
        directory = read(int.from_bytes(root[2:6], "little"), directory_size)
        index, found = 0, set()
        while index < len(directory):
            length = directory[index]
            if not length:
                index = (index // 2048 + 1) * 2048
                continue
            record = directory[index : index + length]
            index += length
            if len(record) < 34 or len(record) < 33 + record[32]:
                raise FormatError("Truncated v14 directory entry")
            raw_name = bytes(record[33 : 33 + record[32]])
            if raw_name in (b"\0", b"\1"):
                continue
            name = raw_name.decode("ascii").split(";", 1)[0].lower()
            if name not in targets:
                continue
            if name in found or record[25] & 2:
                raise FormatError("Ambiguous v14 source file")
            found.add(name)
            lba, length = (
                int.from_bytes(record[2:6], "little"),
                int.from_bytes(record[10:14], "little"),
            )
            if not 0 < length <= 1 << 30 or (lba + (length + 2047) // 2048) * stride > size:
                raise FormatError("Invalid v14 file bounds")
            writer = BoundedWriter(destination / name, length)
            try:
                while length:
                    amount = min(length, 2048 * 512)
                    writer.write(read(lba, amount))
                    length -= amount
                    lba += amount // 2048
            finally:
                writer.close()
        if found != set(targets):
            raise FormatError("Missing v14 source files")


def prepare(installation, data, destination):
    from .editions import V14

    destination = Path(destination)
    check_space(destination.parent, sum(size for _, size, _ in V14.files) + INSTALL_IMAGE[0])
    destination.mkdir(mode=0o700)
    try:
        with tempfile.TemporaryDirectory(
            prefix="v14-cabinets-", dir=destination.parent
        ) as temporary:
            temporary = Path(temporary)
            extract_disc(
                installation,
                INSTALL_IMAGE,
                ("data1.cab", "data1.hdr", "data2.cab", "icd_14.dll"),
                temporary,
            )
            extract_disc(data, DATA_IMAGE, ("uk.dat", "d5.dat", "lobdesc.dat"), destination)
            (destination / "telephone-prefixes.dat").write_bytes(
                extract_prefixes(temporary / "icd_14.dll")
            )
            extract_cabinet(temporary / "data1.cab", destination, V14)
            V14.verify(destination)
        return V14
    except BaseException:
        shutil.rmtree(destination)
        raise


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--installation", required=True, help="v14 installation MDF image")
    parser.add_argument("--data", required=True, help="v14 data IMG image")
    parser.add_argument("--output", required=True, help="new extracted database directory")
    args = parser.parse_args()
    try:
        edition = prepare(args.installation, args.data, args.output)
        print(edition.id)
    except (OSError, ValueError, FormatError) as exc:
        parser.exit(1, f"ukinfo v14 preparation: {exc}\n")


if __name__ == "__main__":
    main()
