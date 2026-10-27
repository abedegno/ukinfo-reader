"""Build a private native oracle; never change serving deadlines or settings."""

import json
import os
import subprocess
import tempfile
from pathlib import Path

root = Path(__file__).resolve().parents[2] / "server"
with tempfile.TemporaryDirectory(prefix="ukinfo-reference-") as directory:
    directory = Path(directory)
    replace = {}
    for relative, changes in {
        "internal/catalog/http.go": [("timeout = 5 * time.Second", "timeout = 120 * time.Second")],
        "internal/catalog/ordered.go": [
            ("statement_timeout='4000'", "statement_timeout='120000'"),
            ("SET LOCAL work_mem='32MB'", "SET LOCAL work_mem='256MB'; SET LOCAL jit=off"),
        ],
        "internal/catalog/addresses.go": [
            ("statement_timeout='4000'", "statement_timeout='120000'"),
        ],
        "internal/catalog/geography.go": [
            ("statement_timeout='4000'", "statement_timeout='120000'"),
        ],
    }.items():
        path = root / relative
        source = path.read_text()
        for before, after in changes:
            if source.count(before) != 1:
                raise RuntimeError("Reference overlay requires manual review after serving changes")
            source = source.replace(before, after)
        target = directory / path.name
        target.write_text(source)
        replace[str(path)] = str(target)
    overlay = directory / "overlay.json"
    overlay.write_text(json.dumps({"Replace": replace}))
    subprocess.run(
        [
            "go",
            "build",
            "-overlay",
            str(overlay),
            "-trimpath",
            "-o",
            "/private/tmp/ukinfo-ordered-reference",
            "./cmd/ukinfo-parade-trial",
        ],
        cwd=root,
        env=dict(os.environ, GOOS="linux", GOARCH="amd64", CGO_ENABLED="0"),
        check=True,
    )
