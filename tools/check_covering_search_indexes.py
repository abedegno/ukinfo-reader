#!/usr/bin/env python3
"""Check online hosted covering indexes in a disposable PostgreSQL 18 instance."""

import json
import subprocess
import time
import uuid
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def main():
    name = "ukinfo-cover-check-" + uuid.uuid4().hex[:10]

    def docker(*args, check=True, input=None):
        return subprocess.run(
            ["docker", *args], check=check, input=input, text=True, capture_output=True
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
        for _ in range(60):
            if docker("exec", name, "pg_isready", "-h", "127.0.0.1", "-U", "ukinfo", check=False).returncode == 0:
                break
            time.sleep(0.2)
        else:
            raise RuntimeError("Disposable PostgreSQL did not start")
        sql("""CREATE TABLE entries (dataset_id smallint,kind smallint,name_id integer,first_id integer,id integer,postcode_id integer) PARTITION BY LIST(dataset_id);
CREATE TABLE entries_d1 PARTITION OF entries FOR VALUES IN (1);
CREATE TABLE entries_d2 PARTITION OF entries FOR VALUES IN (2);
CREATE INDEX entries_name ON entries(dataset_id,kind,name_id,first_id,id);
CREATE INDEX entries_postcode ON entries(dataset_id,kind,postcode_id,name_id,first_id,id);
INSERT INTO entries SELECT d,0,i%10,i%1970,i,i%727 FROM generate_series(1,25000) i CROSS JOIN generate_series(1,2) d;
VACUUM (ANALYZE) entries;
""")
        query = "SELECT dataset_id,id,name_id,first_id,postcode_id FROM entries WHERE dataset_id=1 AND kind=0 AND name_id=3 AND first_id=ANY(ARRAY[3,13,23,33,43,53,63]) AND postcode_id=ANY(ARRAY(SELECT generate_series(0,726))) ORDER BY postcode_id,name_id,first_id,id,dataset_id LIMIT 101"
        before = sql(query).stdout
        script = (ROOT / "deploy/production/covering-search-indexes.sql").read_text()
        sql(script)
        sql(script)  # Safe idempotent reapplication of valid managed indexes.
        assert sql(query).stdout == before, "Covering indexes changed results/order"
        assert (
            sql(
                "SELECT count(*) FROM pg_index i JOIN pg_class c ON c.oid=i.indexrelid WHERE c.relname LIKE 'nas_entries_d%_name_postcode_cover' AND i.indisvalid AND i.indisready AND i.indnkeyatts=5 AND i.indnatts=6"
            ).stdout.strip()
            == "2"
        )
        plan = json.loads(sql("EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) " + query).stdout)[0]
        nodes = []

        def walk(node):
            nodes.append(node)
            for child in node.get("Plans", []):
                walk(child)

        walk(plan["Plan"])
        assert any(
            n.get("Index Name") == "nas_entries_d1_name_postcode_cover"
            and n["Node Type"] == "Index Only Scan"
            and n.get("Heap Fetches") == 0
            for n in nodes
        ), "Candidate search did not use heap-free covering scan"
        sql(
            "DROP INDEX nas_entries_d1_name_postcode_cover; CREATE INDEX nas_entries_d1_name_postcode_cover ON entries_d1(id);"
        )
        failure = sql(script, check=False)
        assert failure.returncode and "invalid or incompatible" in failure.stderr, (
            "Incompatible managed name was accepted"
        )
        assert sql(query).stdout == before, "Guard changed existing data"
        print(
            "Covering index checks passed: concurrent build, idempotency, exact results/order, index-only plan and incompatible-index guard."
        )
    except subprocess.CalledProcessError as error:
        print(error.stderr)
        raise
    finally:
        docker("rm", "--force", name, check=False)


if __name__ == "__main__":
    main()
