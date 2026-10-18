"""Collect notices for the actual standalone decoder's bundled native libraries."""

import ast
import hashlib
import importlib.metadata
import json
import shutil
import subprocess
import sys
import urllib.request
from pathlib import Path


def dependency_notices(root, notices):
    toc = root / "build/pyinstaller/ukinfo-import/Analysis-00.toc"
    binaries = []

    def walk(value):
        if isinstance(value, (list, tuple)):
            if (
                len(value) == 3
                and isinstance(value[2], str)
                and value[2] in ("BINARY", "EXTENSION")
            ):
                binaries.append(Path(value[1]).resolve())
            else:
                for item in value:
                    walk(item)

    walk(ast.literal_eval(toc.read_text()))
    prefix = Path(sys.base_prefix).resolve()
    version = ".".join(str(n) for n in sys.version_info[:3])
    url = f"https://raw.githubusercontent.com/python/cpython/v{version}/Doc/license.rst"
    # Retain CPython's full borrowed-code notices as well as its base PSF license,
    # including statically compiled decimal/hash/compression extension notices.
    with urllib.request.urlopen(url, timeout=30) as response:
        notice = response.read(1024 * 1024)
    (notices / "CPython-stdlib-notices.rst").write_bytes(notice)
    reports = [
        {
            "supplier": f"CPython-{version}",
            "source_url": url,
            "sha256": hashlib.sha256(notice).hexdigest(),
            "notices": ["Python-LICENSE.txt", "CPython-stdlib-notices.rst"],
        }
    ]
    # Wheels can carry their own native extensions and private libraries. Their
    # supplier is the installed distribution, not the host package manager.
    wheel_owners = {}
    for distribution in importlib.metadata.distributions():
        for item in distribution.files or ():
            if str(item).endswith((".so", ".dylib", ".pyd")):
                wheel_owners[Path(distribution.locate_file(item)).resolve()] = distribution
    for source in sorted(set(binaries)):
        # Python's framework/library and its own extension modules are covered by
        # the retained CPython notices. External shared libraries need supplier notices.
        if source.is_relative_to(prefix) and (
            "lib-dynload" in source.parts
            or source.name == "Python"
            or source.name.startswith("libpython")
        ):
            continue
        if source in wheel_owners:
            distribution = wheel_owners[source]
            label = distribution.metadata["Name"] + "-" + distribution.version
            retained = []
            for item in distribution.files or ():
                if item.name.upper().startswith(("LICENSE", "COPYING", "COPYRIGHT", "NOTICE")):
                    original = Path(distribution.locate_file(item))
                    if not original.is_file():
                        continue
                    destination = notices / "wheels" / label / str(item).replace("../", "parent/")
                    destination.parent.mkdir(parents=True, exist_ok=True)
                    shutil.copy2(original, destination)
                    retained.append(str(destination.relative_to(notices)))
            if not retained:
                raise RuntimeError(f"Missing wheel notices for {label}")
            reports.append({"library": source.name, "supplier": label, "notices": retained})
            continue
        supplier = next((p for p in source.parents if p.parent.parent.name == "Cellar"), None)
        if supplier:
            label = supplier.parent.name + "-" + supplier.name
            files = [
                p
                for p in supplier.rglob("*")
                if p.is_file()
                and p.name.upper().startswith(("LICENSE", "COPYING", "COPYRIGHT", "NOTICE"))
                and "man" not in p.parts
            ]
            base = supplier
        elif source.is_relative_to(prefix):
            label = f"CPython-{version}-distribution"
            base = prefix
            files = [
                p
                for p in prefix.rglob("*")
                if p.is_file()
                and p.name.upper().startswith(("LICENSE", "COPYRIGHT", "COPYING", "NOTICE"))
                and "site-packages" not in p.parts
            ]
        elif sys.platform.startswith("linux"):
            owner = None
            candidates = [str(source), str(source).replace("/usr/lib/", "/lib/", 1)]
            for candidate in dict.fromkeys(candidates):
                result = subprocess.run(
                    ["dpkg-query", "-S", candidate], text=True, capture_output=True
                )
                if result.returncode == 0:
                    owner = result.stdout.splitlines()[0].rsplit(": ", 1)[0]
                    break
            if owner is None:
                raise RuntimeError(f"No package copyright found for decoder library {source.name}")
            label = owner.replace(":", "-")
            base = Path("/usr/share/doc") / owner.split(":", 1)[0]
            files = [base / "copyright"] if (base / "copyright").is_file() else []
        else:
            raise RuntimeError(f"No supplier notices found for decoder library {source.name}")
        if not files:
            raise RuntimeError(f"Missing redistribution notices for decoder library {source.name}")
        copied = []
        for file in files:
            target = notices / "native-dependencies" / label / file.relative_to(base)
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(file, target)
            copied.append(str(target.relative_to(notices)))
        reports.append({"library": source.name, "supplier": label, "notices": copied})
    (notices / "native-dependencies.json").write_text(json.dumps(reports, indent=2) + "\n")
