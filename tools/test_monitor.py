"""Exercise the production monitor with invented counters and no network access."""

import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


class MonitorTests(unittest.TestCase):
    def run_monitor(
        self,
        *,
        health=True,
        metrics=True,
        backup=True,
        free=999999999,
        counters='ukinfo_query_requests_total{class="name",status="200"} 50',
        alert=False,
    ):
        with tempfile.TemporaryDirectory() as temp:
            base = Path(temp)
            state = base / "state"
            state.mkdir()
            if backup:
                (state / "backup-epoch").write_text("2000000000\n")
            (base / "metrics").write_text(counters + "\n")
            (base / "webhook").write_text("http://127.0.0.1:12345/invented\n")
            scripts = {
                "date": "echo 2000000000",
                "df": f'echo "Filesystem 1024-blocks Used Available Capacity Mounted"\necho "invented 999999999 1 {free} 1% /state"',
                "sleep": 'n=0; test ! -f "$FIXTURE/loops" || n=$(cat "$FIXTURE/loops"); n=$((n+1)); echo "$n" > "$FIXTURE/loops"; test "$n" -lt 3',
                "wget": """case "$*" in
 *--post-file=*) cat "$FIXTURE/state/alert.tmp" >> "$FIXTURE/sent";;
 */api/health) test "$HEALTH" = yes;;
 */metrics) test "$METRICS" = yes || exit 1; cat "$FIXTURE/metrics" > "$FIXTURE/state/metrics.tmp";;
 *) exit 9;;
esac""",
            }
            for name, body in scripts.items():
                path = base / name
                path.write_text("#!/bin/sh\n" + body + "\n")
                path.chmod(0o700)
            env = dict(
                os.environ,
                PATH=f"{base}:/usr/bin:/bin",
                FIXTURE=temp,
                UKINFO_MONITOR_STATE=str(state),
                UKINFO_PUBLIC_ORIGIN="https://invented.example",
                UKINFO_ALERT_FILE=str(base / "webhook") if alert else "/dev/null",
                HEALTH="yes" if health else "no",
                METRICS="yes" if metrics else "no",
            )
            args = ["/bin/sh", str(ROOT / "deploy/production/monitor.sh")]
            if not alert:
                args.append("--once")
            run = subprocess.run(args, env=env, capture_output=True, text=True, timeout=5)
            self.assertEqual(run.returncode, 1 if alert else 0, run.stderr)
            status = json.loads((state / "status.json").read_text())
            self.assertEqual((state / "status.json").stat().st_mode & 0o777, 0o600)
            self.assertNotIn("invented.example", json.dumps(status))
            sent = (base / "sent").read_text() if (base / "sent").exists() else ""
            return status, sent

    def test_health_and_backup(self):
        self.assertEqual(self.run_monitor()[0]["status"], "healthy")
        self.assertEqual(self.run_monitor(backup=False)[0]["status"], "warning")

    def test_failures(self):
        for options in (
            {"health": False},
            {"metrics": False},
            {"free": 1},
            {"counters": 'ukinfo_query_requests_total{class="grouped",status="503"} 30'},
        ):
            with self.subTest(options=options):
                self.assertEqual(self.run_monitor(**options)[0]["status"], "critical")

    def test_alert_debounce_and_private_payload(self):
        status, sent = self.run_monitor(health=False, alert=True)
        self.assertEqual(status["status"], "critical")
        notifications = sent.strip().splitlines()
        self.assertEqual(len(notifications), 1)
        payload = json.loads(notifications[0])
        self.assertEqual(payload["status"], "critical")
        self.assertNotIn("invented", sent)
        self.assertNotIn("http", sent)


if __name__ == "__main__":
    unittest.main()
