#!/usr/bin/env python3
"""Measure trusted private SELECT plans without printing SQL or personal values.

Requires psql and DATABASE_URL (not a command-line credential). Reports aggregate
work only. Never clears caches; a first observation is not necessarily cold.
"""

import argparse
import json
import os
import subprocess
from pathlib import Path
from urllib.parse import parse_qsl, unquote, urlsplit

ROOT = Path(__file__).resolve().parents[1]


def summarize(plan):
    nodes = []

    def walk(node):
        nodes.append(node)
        for child in node.get("Plans", []):
            walk(child)

    top = plan["Plan"]
    walk(top)
    return {
        "milliseconds": plan["Execution Time"],
        "shared_hit_blocks": top.get("Shared Hit Blocks", 0),
        "shared_read_blocks": top.get("Shared Read Blocks", 0),
        "temp_read_blocks": top.get("Temp Read Blocks", 0),
        "temp_written_blocks": top.get("Temp Written Blocks", 0),
        "heap_fetches": sum(n.get("Heap Fetches", 0) for n in nodes),
        "sequential_scans": sum(n["Node Type"] == "Seq Scan" for n in nodes),
        "rows_removed_by_filter": sum(
            n.get("Rows Removed by Filter", 0) * n.get("Actual Loops", 1) for n in nodes
        ),
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cases", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--repeats", type=int, default=3)
    parser.add_argument("--timeout-ms", type=int, default=2000)
    args = parser.parse_args()
    if not args.cases.resolve().is_relative_to(ROOT / "private"):
        parser.error("cases must be an operator-owned file under private/")
    if not 1 <= args.repeats <= 10 or not 100 <= args.timeout_ms <= 5000:
        parser.error("use 1–10 repeats and a 100–5000 ms timeout")
    # Pass libpq fields through the environment, never through process argv.
    env = os.environ.copy()
    connection = urlsplit(env.pop("DATABASE_URL", ""))
    if connection.scheme not in {"postgres", "postgresql"} or not connection.hostname:
        parser.error("DATABASE_URL must be a PostgreSQL URL with a hostname")
    env.update(
        PGHOST=connection.hostname,
        PGPORT=str(connection.port or 5432),
        PGDATABASE=unquote(connection.path.removeprefix("/")),
        PGUSER=unquote(connection.username or ""),
        PGPASSWORD=unquote(connection.password or ""),
    )
    options = {
        "sslmode": "PGSSLMODE",
        "sslrootcert": "PGSSLROOTCERT",
        "sslcert": "PGSSLCERT",
        "sslkey": "PGSSLKEY",
        "connect_timeout": "PGCONNECT_TIMEOUT",
        "application_name": "PGAPPNAME",
    }
    for key, value in parse_qsl(connection.query):
        if key not in options:
            parser.error(
                "unsupported connection URL option; use standard libpq environment variables"
            )
        env[options[key]] = value
    cases = json.loads(args.cases.read_text())
    if not isinstance(cases, list) or not 1 <= len(cases) <= 100:
        parser.error("expected 1–100 trusted SELECT strings")
    report = []
    for index, query in enumerate(cases):
        if not isinstance(query, str) or not query.lstrip().upper().startswith(
            ("SELECT ", "WITH ")
        ):
            parser.error("cases must contain trusted SELECT strings")
        for repeat in range(args.repeats):
            sql = (
                f"BEGIN READ ONLY; SET LOCAL statement_timeout='{args.timeout_ms}ms'; "
                "SET LOCAL lock_timeout='100ms'; EXPLAIN (ANALYZE,BUFFERS,TIMING OFF,FORMAT JSON) "
                + query.rstrip().removesuffix(";")
                + "; ROLLBACK;"
            )
            process = subprocess.run(
                ["psql", "-XAtq", "-v", "ON_ERROR_STOP=1"],
                input=sql,
                text=True,
                capture_output=True,
                env=env,
                timeout=args.timeout_ms / 1000 + 10,
            )
            row = {"case": index, "observation": repeat + 1}
            if process.returncode:
                # PostgreSQL error details can include the query: do not echo stderr.
                row["failed"] = True
            else:
                row.update(summarize(json.loads(process.stdout)[0]))
            report.append(row)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(
        json.dumps(
            {
                "conditions": "No cache manipulation; first observation is not necessarily cold. shared_read_blocks are PostgreSQL cache misses and may be served by OS cache.",
                "plans": report,
            },
            indent=2,
        )
        + "\n"
    )
    print(
        f"Saved {len(report)} aggregate plan observations; {sum(r.get('failed', False) for r in report)} failed."
    )
    return int(any(r.get("failed", False) for r in report))


if __name__ == "__main__":
    raise SystemExit(main())
