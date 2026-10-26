"""Validate optional search Compose contracts without deploying or reading secrets."""

import json
import os
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def configuration(files, values, profile):
    env = dict(os.environ, **values)
    command = ["docker", "compose", "--env-file", os.devnull]
    for file in files:
        command.extend(["-f", str(ROOT / file)])
    command.extend(["--profile", profile, "config", "--format", "json"])
    result = subprocess.run(command, env=env, capture_output=True, check=False)
    if result.returncode:
        raise RuntimeError("Compose validation failed; no expanded environment was printed")
    return json.loads(result.stdout)


def main():
    preview = configuration(
        ["deploy/compose.production.yaml", "deploy/compose.parade-preview.yaml"],
        {
            "UKINFO_DB_PASSWORD": "fixture-owner",
            "UKINFO_SEARCH_PASSWORD": "fixture-reader",
            "UKINFO_PUBLIC_ORIGIN": "https://archive.example.org",
            "UKINFO_PARADE_OWNER_PASSWORD": "a" * 64,
            "UKINFO_PARADE_SEARCH_PASSWORD": "b" * 64,
            "UKINFO_PARADE_WORK_DIR": "/tmp/parade-compose-fixture",
        },
        "parade-preview",
    )
    services = preview["services"]
    database = services["parade-db"]
    # Unquoted commas in a YAML flow list silently become separate mount paths;
    # config --quiet alone accepts the invalid split. Preserve one complete item.
    expected = ["/docker-entrypoint-initdb.d:rw,noexec,nosuid,size=4k"]
    assert database["tmpfs"] == expected, "Derived DB tmpfs mount was split"
    assert database["init"] and not database.get("ports")
    assert not services["parade-indexer"].get("ports")
    assert services["parade-indexer"]["read_only"]
    assert services["parade-indexer"]["cap_drop"] == ["ALL"]
    assert "--search-backend" not in services["api"]["command"], "Overlay activated serving"
    assert "SOURCE_DATABASE_URL" not in services["api"]["environment"]
    assert "UKINFO_PARADE_OWNER_PASSWORD" not in services["api"]["environment"]
    spike = configuration(
        ["tools/real-search-spike/compose.yaml"],
        {
            "SPIKE_PASSWORD": "fixture-only",
            "SPIKE_ROOT": "/tmp/parade-compose-fixture",
            "SOURCE_DATABASE_URL": "postgres://reader:fixture@source/source",
            "TARGET_DATABASE_URL": "postgres://owner:fixture@target/target",
        },
        "operator",
    )["services"]
    assert spike["database"]["tmpfs"] == expected, "Spike DB tmpfs mount was split"
    assert spike["client"]["tmpfs"] == ["/tmp:rw,noexec,nosuid,size=64m"]
    assert not spike["database"].get("ports")
    print(
        "ParadeDB Compose contracts verified: private ports, opt-in serving and valid tmpfs mounts"
    )


if __name__ == "__main__":
    main()
