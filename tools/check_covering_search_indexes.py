#!/usr/bin/env python3
"""Check online hosted covering indexes in a disposable PostgreSQL 18 instance."""

import json
import os
import subprocess
import time
import uuid
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def check_plan_matrix(sql):
    """Candidate SQL shapes: fixed work budgets, both cached-plan modes.

    The fixture deliberately has a dominant surname, rare names, 1,970 given-name
    variants, 727 postcode terms, businesses and two overlapping editions.
    SQL timings describe a small cached fixture, not production HDD capacity.
    """
    cases = [
        ("common-name-district", [1], 0, [3], list(range(1970)), list(range(100))),
        ("rare-name-district", [1], 0, [9], list(range(1970)), list(range(100))),
        ("given-name-variants", [1], 0, [3], list(range(3, 1970, 10)), list(range(727))),
        ("full-postcode", [1], 0, [3], list(range(1970)), [3]),
        ("postcode-sector", [1], 0, [3], list(range(1970)), list(range(10))),
        ("business-district", [1, 2], 1, [3], list(range(1970)), list(range(100))),
        ("all-editions", [1, 2], 0, [3], list(range(1970)), list(range(100))),
    ]
    rows = []
    for label, editions, kind, names, firsts, postcodes in cases:
        for page in [0, 1]:
            query = """SELECT dataset_id,id,name_id,first_id,postcode_id FROM entries
WHERE dataset_id=ANY($1) AND kind=$2 AND name_id=ANY($3)
AND first_id=ANY($4) AND postcode_id=ANY($5)
AND (postcode_id,name_id,first_id,id,dataset_id)>($6,$7,$8,$9,$10)
ORDER BY postcode_id,name_id,first_id,id,dataset_id LIMIT 51"""
            # Page two starts at the independently evaluated page-one boundary.
            args = [editions, kind, names, firsts, postcodes, -1, -1, -1, -1, -1]

            def literal(value):
                return "ARRAY" + str(value) if isinstance(value, list) else str(value)

            def expanded():
                text = query
                for i in range(len(args), 0, -1):
                    text = text.replace("$" + str(i), literal(args[i - 1]))
                return text

            if page:
                boundary = sql(expanded().replace("LIMIT 51", "LIMIT 50")).stdout.splitlines()
                if len(boundary) < 50:
                    continue
                d, identity, name, first, postcode = map(int, boundary[-1].split("|"))
                args[5:] = [postcode, name, first, identity, d]
            # Independent result reference with index access disabled.
            reference = sql(
                "BEGIN; SET LOCAL enable_indexscan=off; SET LOCAL enable_indexonlyscan=off; SET LOCAL enable_bitmapscan=off; "
                + expanded()
                + "; ROLLBACK;"
            ).stdout
            for mode in ["force_custom_plan", "force_generic_plan"]:
                prepare = (
                    "BEGIN; SET LOCAL plan_cache_mode="
                    + mode
                    + "; PREPARE matrix(smallint[],smallint,integer[],integer[],integer[],integer,integer,integer,integer,smallint) AS "
                    + query
                    + "; "
                )
                execute = "EXECUTE matrix(" + ",".join(map(literal, args)) + ")"
                assert sql(prepare + execute + "; ROLLBACK;").stdout == reference, (
                    label + ": result/order mismatch"
                )
                plan = json.loads(
                    sql(
                        prepare + "EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) " + execute + "; ROLLBACK;"
                    ).stdout
                )[0]
                nodes = []

                def walk(node):
                    nodes.append(node)
                    for child in node.get("Plans", []):
                        walk(child)

                walk(plan["Plan"])
                scans = [n for n in nodes if n.get("Relation Name", "").startswith("entries_d")]
                assert scans and all(
                    n["Node Type"] == "Index Only Scan" and n.get("Heap Fetches") == 0
                    for n in scans
                ), label + ": heap/sequence scan regression"
                blocks = plan["Plan"].get("Shared Hit Blocks", 0) + plan["Plan"].get(
                    "Shared Read Blocks", 0
                )
                assert blocks < 1500, label + ": candidate buffer budget exceeded"
                rows.append(
                    {
                        "case": label,
                        "page": page + 1,
                        "mode": mode,
                        "milliseconds": plan["Execution Time"],
                        "shared_blocks": blocks,
                        "shared_read_blocks": plan["Plan"].get("Shared Read Blocks", 0),
                        "heap_fetches": sum(n.get("Heap Fetches", 0) for n in scans),
                    }
                )
    report = {
        "fixture_entries": 50000,
        "conditions": "cached disposable PostgreSQL; not a cold-disk or capacity benchmark",
        "plans": rows,
    }
    if output := os.environ.get("UKINFO_PLAN_REPORT"):
        Path(output).parent.mkdir(parents=True, exist_ok=True)
        Path(output).write_text(json.dumps(report, indent=2) + "\n")
    print(
        f"Search plan matrix passed: {len(rows)} custom/generic page plans; exact results, zero heap fetches and bounded buffers."
    )


def check_vocabulary_plans(sql):
    """Cover the dictionary stage, including bounded prefixes and broad expansion."""
    for label, types, query, values in [
        (
            "given-prefix",
            "text,text",
            "SELECT id,key FROM terms WHERE key >= $1 AND key < $2 ORDER BY key LIMIT 8193",
            "'fixturegiven','fixturegiven' || chr(1114111)",
        ),
        (
            "postcode-district",
            "text,text",
            "SELECT id,key FROM terms WHERE key >= $1 AND key < $2 AND (length(key)=6 OR key=$1) ORDER BY key LIMIT 8193",
            "'ZZ1','ZZ1' || chr(1114111)",
        ),
        (
            "broad-expansion",
            "text,text",
            "SELECT array_agg(id) FROM terms WHERE key >= $1 AND key < $2",
            "'fixturegiven','fixturegiven' || chr(1114111)",
        ),
        (
            "label-from-id",
            "integer[]",
            "SELECT id,key FROM terms WHERE id=ANY($1) ORDER BY id",
            "ARRAY[1,17,400,1771,20003,45001]",
        ),
    ]:
        for mode in ("force_custom_plan", "force_generic_plan"):
            start = (
                "BEGIN; SET LOCAL plan_cache_mode="
                + mode
                + "; PREPARE vocabulary("
                + types
                + ") AS "
                + query
                + "; "
            )
            execute = "EXECUTE vocabulary(" + values + ")"
            reference = sql(
                start
                + "SET LOCAL enable_indexscan=off; SET LOCAL enable_indexonlyscan=off; SET LOCAL enable_bitmapscan=off; "
                + execute
                + "; ROLLBACK;"
            ).stdout
            # array_agg has no ordering contract; compare its members independently.
            actual = sql(start + execute + "; ROLLBACK;").stdout
            if label == "broad-expansion":
                assert sorted(actual.strip().strip("{}").split(",")) == sorted(
                    reference.strip().strip("{}").split(",")
                )
            else:
                assert actual == reference, label + ": vocabulary results/order changed"
            plan = json.loads(
                sql(
                    start + "EXPLAIN(ANALYZE,BUFFERS,FORMAT JSON) " + execute + "; ROLLBACK;"
                ).stdout
            )[0]
            nodes = []

            def walk(node):
                nodes.append(node)
                for child in node.get("Plans", []):
                    walk(child)

            walk(plan["Plan"])
            scans = [n for n in nodes if n.get("Relation Name") == "terms"]
            assert scans and all(
                n["Node Type"] == "Index Only Scan"
                and n.get("Heap Fetches") == 0
                and n.get("Index Name")
                == (
                    "nas_terms_id_key_cover"
                    if label == "label-from-id"
                    else "nas_terms_key_id_cover"
                )
                for n in scans
            ), label + ": vocabulary heap-scan regression"
            assert plan["Plan"].get("Actual Rows", 0) > 0
            assert (
                plan["Plan"].get("Shared Hit Blocks", 0) + plan["Plan"].get("Shared Read Blocks", 0)
                < 512
            ), label + ": vocabulary buffer budget exceeded"
    print(
        "Vocabulary plans passed: eight custom/generic prefix, expansion and ID-to-label checks; exact results, zero heap fetches and bounded buffers."
    )


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
        sql("""CREATE TABLE terms(id integer PRIMARY KEY,key text COLLATE "C" NOT NULL UNIQUE);
INSERT INTO terms SELECT i,CASE WHEN i%50=0 THEN 'ZZ1'||lpad((i/50)::text,3,'0') WHEN i%2=0 THEN 'fixturegiven '||lpad(i::text,5,'0') ELSE 'unrelated '||i::text END FROM generate_series(1,49999) i ORDER BY (i*7919)%50000;
VACUUM(ANALYZE) terms;
CREATE TABLE entries (dataset_id smallint,kind smallint,name_id integer,first_id integer,id integer,postcode_id integer) PARTITION BY LIST(dataset_id);
CREATE TABLE entries_d1 PARTITION OF entries FOR VALUES IN (1);
CREATE TABLE entries_d2 PARTITION OF entries FOR VALUES IN (2);
CREATE INDEX entries_name ON entries(dataset_id,kind,name_id,first_id,id);
CREATE INDEX entries_postcode ON entries(dataset_id,kind,postcode_id,name_id,first_id,id);
INSERT INTO entries SELECT d,CASE WHEN i%20=0 THEN 1 ELSE 0 END,CASE WHEN i%10<7 THEN 3 ELSE i%10 END,i%1970,i,i%727 FROM generate_series(1,25000) i CROSS JOIN generate_series(1,2) d;
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
        generic = (
            query.replace("dataset_id=1", "dataset_id=$1")
            .replace("kind=0", "kind=$2")
            .replace("name_id=3", "name_id=$3")
            .replace("ARRAY[3,13,23,33,43,53,63]", "$4")
            .replace("ARRAY(SELECT generate_series(0,726))", "$5")
        )
        generic_plan = json.loads(
            sql(
                "BEGIN; SET LOCAL plan_cache_mode=force_generic_plan; PREPARE fixture(smallint,smallint,integer,integer[],integer[]) AS "
                + generic
                + "; EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) EXECUTE fixture(1,0,3,ARRAY[3,13,23,33,43,53,63],ARRAY"
                + str(list(range(727)))
                + "); ROLLBACK;"
            ).stdout
        )[0]
        nodes.clear()
        walk(generic_plan["Plan"])
        assert any(
            n.get("Index Name") == "nas_entries_d1_name_postcode_cover"
            and n["Node Type"] == "Index Only Scan"
            and n.get("Heap Fetches") == 0
            for n in nodes
        ), "Prepared generic plan did not use heap-free covering scan"
        check_vocabulary_plans(sql)
        check_plan_matrix(sql)
        sql(
            "DROP INDEX nas_entries_d1_name_postcode_cover; CREATE INDEX nas_entries_d1_name_postcode_cover ON entries_d1(id);"
        )
        failure = sql(script, check=False)
        assert failure.returncode and "invalid or incompatible" in failure.stderr, (
            "Incompatible managed name was accepted"
        )
        assert sql(query).stdout == before, "Guard changed existing data"
        sql(
            "DROP INDEX nas_terms_key_id_cover; CREATE INDEX nas_terms_key_id_cover ON terms(id) INCLUDE(key);"
        )
        failure = sql(script, check=False)
        assert (
            failure.returncode
            and "vocabulary covering index is invalid or incompatible" in failure.stderr
        ), "Incompatible vocabulary index was accepted"
        sql("DROP INDEX nas_terms_key_id_cover; DROP INDEX nas_entries_d1_name_postcode_cover;")
        sql(script)
        sql(
            "DROP INDEX nas_terms_id_key_cover; CREATE INDEX nas_terms_id_key_cover ON terms(key) INCLUDE(id);"
        )
        failure = sql(script, check=False)
        assert (
            failure.returncode
            and "reverse vocabulary covering index is invalid or incompatible" in failure.stderr
        ), "Incompatible reverse vocabulary index was accepted"
        print(
            "Covering index checks passed: concurrent build, idempotency, exact results/order, custom/generic index-only plans and incompatible-index guard."
        )
    except subprocess.CalledProcessError as error:
        print(error.stderr)
        raise
    finally:
        docker("rm", "--force", name, check=False)


if __name__ == "__main__":
    main()
