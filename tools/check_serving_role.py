"""Verify the production role policy in a disposable, network-isolated PostgreSQL."""

import os
import secrets
import subprocess
import time
import uuid
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def check():
    name = "ukinfo-role-check-" + uuid.uuid4().hex[:12]
    env = dict(
        os.environ,
        POSTGRES_PASSWORD=secrets.token_hex(24),
        UKINFO_SEARCH_PASSWORD=secrets.token_hex(24),
    )

    def docker(*args, input=None, check=True):
        result = subprocess.run(
            ["docker", *args],
            env=env,
            input=input,
            text=True,
            capture_output=True,
            check=False,
            timeout=120,
        )
        if check and result.returncode:
            raise RuntimeError(result.stderr.strip())
        return result

    def owner(sql, check=True):
        return docker(
            "exec",
            "-i",
            name,
            "psql",
            "-XAt",
            "-v",
            "ON_ERROR_STOP=1",
            "-U",
            "ukinfo",
            "-d",
            "ukinfo",
            input=sql,
            check=check,
        )

    def policy(check=True):
        return docker(
            "exec",
            name,
            "psql",
            "-XAt",
            "-v",
            "ON_ERROR_STOP=1",
            "-U",
            "ukinfo",
            "-d",
            "ukinfo",
            "-f",
            "/policy.sql",
            check=check,
        )

    def reader(sql):
        return docker(
            "exec",
            "-i",
            name,
            "sh",
            "-c",
            'PGPASSWORD="$UKINFO_SEARCH_PASSWORD" exec psql -XAt '
            "-v ON_ERROR_STOP=1 -h 127.0.0.1 -U ukinfo_search -d ukinfo",
            input=sql,
            check=False,
        )

    def denied():
        selected = reader("SELECT count(*) FROM invented_probe;")
        assert selected.returncode == 0 and selected.stdout.strip() == "1", selected.stderr
        run = reader(
            "SET default_transaction_read_only=off; UPDATE invented_probe SET id=2 WHERE false;"
        )
        assert run.returncode != 0 and "permission denied" in run.stderr, run.stderr

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
            "--cpus",
            "2",
            "--tmpfs",
            "/var/lib/postgresql:rw,nosuid,nodev,size=256m",
            "--env",
            "POSTGRES_USER=ukinfo",
            "--env",
            "POSTGRES_DB=ukinfo",
            "--env",
            "POSTGRES_PASSWORD",
            "--env",
            "UKINFO_SEARCH_PASSWORD",
            "--mount",
            f"type=bind,src={ROOT / 'deploy/production/search-role.sql'},dst=/policy.sql,readonly",
            "postgres:18.6-alpine",
        )
        for attempt in range(90):
            if owner("SELECT 1;", check=False).returncode == 0:
                # The entrypoint's temporary server is not the final instance.
                if docker("exec", name, "cat", "/proc/1/comm").stdout.strip() == "postgres":
                    break
            time.sleep(1)
        else:
            raise RuntimeError("Disposable PostgreSQL did not become ready")
        policy()
        owner(
            "CREATE TABLE invented_probe(id integer); INSERT INTO invented_probe VALUES (1); "
            "CREATE SEQUENCE invented_sequence;"
        )
        denied()
        owner(
            "ALTER ROLE ukinfo_search SUPERUSER CREATEDB CREATEROLE REPLICATION BYPASSRLS; "
            "GRANT ALL ON invented_probe TO ukinfo_search; "
            "GRANT UPDATE(id) ON invented_probe TO ukinfo_search; "
            "GRANT ALL ON SEQUENCE invented_sequence TO ukinfo_search; "
            "GRANT CREATE ON SCHEMA public TO ukinfo_search; "
            "GRANT CREATE ON DATABASE ukinfo TO ukinfo_search;"
        )
        policy()
        flags = owner(
            "SELECT rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls "
            "FROM pg_roles WHERE rolname='ukinfo_search';"
        ).stdout.strip()
        assert flags == "f"
        denied()
        owner("CREATE ROLE invented_group; GRANT invented_group TO ukinfo_search;")
        assert policy(check=False).returncode != 0
        owner(
            "REVOKE invented_group FROM ukinfo_search; "
            "ALTER TABLE invented_probe OWNER TO ukinfo_search;"
        )
        assert policy(check=False).returncode != 0
        owner(
            "ALTER TABLE invented_probe OWNER TO ukinfo; GRANT UPDATE ON invented_probe TO PUBLIC;"
        )
        assert policy(check=False).returncode != 0
        owner("REVOKE UPDATE ON invented_probe FROM PUBLIC;")
        policy()
        denied()
        print(
            "Serving policy verified: fresh/existing roles, column grants, ownership, membership and PUBLIC grants"
        )
    finally:
        docker("rm", "--force", name, check=False)


if __name__ == "__main__":
    check()
