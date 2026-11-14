#!/usr/bin/env python3
"""Check optional source group-key covering indexes with invented data only."""

import argparse
import json
import subprocess
import time
import uuid
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--baseline", action="store_true", help="negative control: omit the optional index"
    )
    args = parser.parse_args()
    name = "ukinfo-group-cover-check-" + uuid.uuid4().hex[:10]

    def docker(*argv, input=None, check=True):
        return subprocess.run(
            ["docker", *argv], input=input, text=True, capture_output=True, check=check, timeout=120
        )

    def sql(text, check=True):
        return docker(
            "exec",
            "-i",
            name,
            "psql",
            "-XAtq",
            "-v",
            "ON_ERROR_STOP=1",
            "-v",
            "dataset=invented-one",
            "-U",
            "ukinfo",
            "-d",
            "ukinfo",
            input=text,
            check=check,
        )

    try:
        docker(
            "run",
            "--detach",
            "--name",
            name,
            "--network",
            "none",
            "--cpus",
            "1",
            "--memory",
            "512m",
            "--tmpfs",
            "/var/lib/postgresql:rw,size=256m",
            "--env",
            "POSTGRES_USER=ukinfo",
            "--env",
            "POSTGRES_DB=ukinfo",
            "--env",
            "POSTGRES_PASSWORD=invented-test-only",
            "postgres:18.6-alpine",
        )
        for _ in range(100):
            if (
                docker(
                    "exec", name, "pg_isready", "-h", "127.0.0.1", "-U", "ukinfo", check=False
                ).returncode
                == 0
            ):
                break
            time.sleep(0.2)
        else:
            raise RuntimeError("Disposable PostgreSQL did not start")
        sql("""CREATE TABLE datasets(id smallint PRIMARY KEY,key text UNIQUE,entries bigint);
INSERT INTO datasets VALUES(1,'invented-one',50000),(2,'invented-hidden',50000);
CREATE TABLE research_indexes(dataset_id smallint PRIMARY KEY,version integer,entries bigint);
INSERT INTO research_indexes VALUES(1,1,50000),(2,1,50000);
CREATE TABLE research_keys(dataset_id smallint,id integer,group_key bytea NOT NULL,source_hash bytea,sic text,industry text,PRIMARY KEY(dataset_id,id)) PARTITION BY LIST(dataset_id);
CREATE TABLE research_d1 PARTITION OF research_keys FOR VALUES IN(1);
CREATE TABLE research_d2 PARTITION OF research_keys FOR VALUES IN(2);
INSERT INTO research_keys SELECT d,i,decode(md5(i::text)||md5('g'||i::text),'hex'),decode(md5(i::text)||md5(i::text),'hex'),'','' FROM generate_series(1,50000) i CROSS JOIN generate_series(1,2) d;
VACUUM(ANALYZE) research_keys;
""")
        ids = ",".join(str(1 + i * 1500) for i in range(32))
        query = f"SELECT id,encode(group_key,'hex') FROM research_keys WHERE dataset_id=1 AND id=ANY(ARRAY[{ids}]) ORDER BY id"
        oracle = sql(
            "BEGIN; SET LOCAL enable_indexscan=off; SET LOCAL enable_indexonlyscan=off; SET LOCAL enable_bitmapscan=off;"
            + query
            + ";COMMIT;"
        ).stdout
        if not args.baseline:
            script = (ROOT / "deploy/production/group-key-indexes.sql").read_text()
            sql(script)
            sql(script)
            assert (
                sql(
                    "SELECT count(*) FROM pg_indexes WHERE indexname LIKE 'nas_research_d%_id_group_cover'"
                ).stdout.strip()
                == "1"
            ), "Unselected edition was indexed"
        observations = []
        for mode in ("force_custom_plan", "force_generic_plan"):
            prepare = f"BEGIN; SET LOCAL plan_cache_mode={mode}; PREPARE probe(smallint,integer[]) AS SELECT id,group_key FROM research_keys WHERE dataset_id=$1 AND id=ANY($2);"
            plan = json.loads(
                sql(
                    prepare
                    + f"EXPLAIN(ANALYZE,BUFFERS,FORMAT JSON) EXECUTE probe(1,ARRAY[{ids}]);COMMIT;"
                ).stdout
            )[0]["Plan"]
            nodes = []

            def visit(node):
                nodes.append(node)
                for child in node.get("Plans", []):
                    visit(child)

            visit(plan)
            scans = [n for n in nodes if n.get("Relation Name") == "research_d1"]
            assert scans and all(
                n["Node Type"] == "Index Only Scan" and n.get("Heap Fetches") == 0 for n in scans
            ), "Group-key lookup still reads source heap rows"
            assert sql(query).stdout == oracle, "Source group keys or ordering changed"
            observations.append(
                {
                    "mode": mode,
                    "heap_fetches": sum(n["Heap Fetches"] for n in scans),
                    "buffers": plan.get("Shared Hit Blocks", 0) + plan.get("Shared Read Blocks", 0),
                }
            )
        if not args.baseline:
            sql(
                "DROP INDEX nas_research_d1_id_group_cover; CREATE INDEX nas_research_d1_id_group_cover ON research_d1(dataset_id,id);"
            )
            assert sql(script, check=False).returncode != 0, (
                "Incompatible existing index was silently accepted"
            )
            sql(
                "DROP INDEX nas_research_d1_id_group_cover; UPDATE research_indexes SET entries=49999 WHERE dataset_id=1;"
            )
            assert sql(script, check=False).returncode != 0, (
                "Incomplete source research projection was accepted"
            )
        print(
            json.dumps(
                {"passed": True, "synthetic_only": True, "network": "none", "plans": observations}
            )
        )
    finally:
        docker("rm", "-f", name, check=False)


if __name__ == "__main__":
    main()
