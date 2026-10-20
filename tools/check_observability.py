#!/usr/bin/env python3
"""Exercise the pinned Collector with invented telemetry and no network egress.

Requires Docker; no Splunk credentials, data packs or Python packages are needed.
"""

import json
import subprocess
import tempfile
import uuid
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
IMAGE = "otel/opentelemetry-collector-contrib:0.163.1@sha256:1175b03003053962012f5a231f5567f66a3e8fdb960c7835c0e48f7aa1555806"

FIXTURE = r"""
import http.server, json, threading, time, urllib.request
class Metrics(http.server.BaseHTTPRequestHandler):
    def log_message(self, *args): pass
    def do_GET(self):
        self.send_response(200); self.end_headers()
        self.wfile.write(b'# TYPE ukinfo_pool_acquired gauge\nukinfo_pool_acquired{route="/api/search",private="SECRET_LABEL"} 2\n# TYPE ukinfo_record_blocks_decoded_total counter\nukinfo_record_blocks_decoded_total 12\n# TYPE invented_private gauge\ninvented_private 42\n')
threading.Thread(target=http.server.HTTPServer(('127.0.0.1',9091), Metrics).serve_forever, daemon=True).start()
payload = {'resourceSpans':[{'resource':{'attributes':[{'key':'private','value':{'stringValue':'SECRET_RESOURCE'}}]},'scopeSpans':[{'spans':[{'traceId':'11111111111111111111111111111111','spanId':'2222222222222222','name':'HTTP /api/search','kind':2,'startTimeUnixNano':str(time.time_ns()-1000000),'endTimeUnixNano':str(time.time_ns()),'attributes':[{'key':'http.route','value':{'stringValue':'/api/search'}},{'key':'url.full','value':{'stringValue':'SECRET_QUERY'}}]}]}]}]}
for attempt in range(50):
    try:
        urllib.request.urlopen(urllib.request.Request('http://127.0.0.1:4318/v1/traces', data=json.dumps(payload).encode(), headers={'Content-Type':'application/json'}),timeout=2).read()
        break
    except OSError: time.sleep(.2)
else: raise SystemExit('OTLP receiver did not start')
for attempt in range(100):
    try:
        text=open('/out/signals.json').read()
        if 'ukinfo_pool_acquired' in text and 'HTTP /api/search' in text: break
    except OSError: pass
    time.sleep(.2)
else: raise SystemExit('Collector did not export both signals')
"""


def run(*args, **kwargs):
    return subprocess.run(args, check=True, text=True, capture_output=True, **kwargs)


def main():
    name = "ukinfo-otel-check-" + uuid.uuid4().hex[:10]
    with tempfile.TemporaryDirectory(prefix="ukinfo-otel-") as tmp:
        directory = Path(tmp)
        directory.chmod(0o755)
        output = directory / "out"
        output.mkdir(mode=0o777)
        output.chmod(0o777)
        config = (ROOT / "deploy/observability/collector.yaml").read_text()
        start = config.index("exporters:\n")
        end = config.index("service:\n", start)
        config = (
            config[:start]
            + "exporters:\n  file/test:\n    path: /out/signals.json\n"
            + config[end:]
        )
        config = (
            config.replace("[otlphttp/splunk]", "[file/test]")
            .replace("scrape_interval: 60s", "scrape_interval: 2s")
            .replace("timeout: 10s", "timeout: 1s")
        )
        (directory / "config.yaml").write_text(config)
        try:
            run(
                "docker",
                "run",
                "--detach",
                "--name",
                name,
                "--network",
                "none",
                "--read-only",
                "--cap-drop=ALL",
                "--security-opt=no-new-privileges",
                "--memory=128m",
                "--cpus=0.25",
                "--volume",
                f"{directory}/config.yaml:/etc/otelcol/config.yaml:ro",
                "--volume",
                f"{output}:/out",
                IMAGE,
                "--config=/etc/otelcol/config.yaml",
            )
            run(
                "docker",
                "run",
                "--rm",
                "--network",
                f"container:{name}",
                "--volume",
                f"{output}:/out:ro",
                "python:3.13-slim",
                "python",
                "-c",
                FIXTURE,
                timeout=40,
            )
            raw = (output / "signals.json").read_text()
            assert "SECRET" not in raw, "Collector leaked a canary attribute"
            assert "invented_private" not in raw, "Non-UKInfo metric was exported"
            assert "scrape_samples" not in raw and '"name":"up"' not in raw, (
                "Scraper internals exported"
            )
            assert '"service.name"' in raw, "Service identity missing"
            signals = [json.loads(line) for line in raw.splitlines() if line.strip()]
            names = {
                metric["name"]
                for item in signals
                for resource in item.get("resourceMetrics", [])
                for scope in resource.get("scopeMetrics", [])
                for metric in scope.get("metrics", [])
            }
            assert "ukinfo_record_blocks_decoded_total" in names, (
                f"Unexpected counter translation: {names}"
            )
            print(
                "Collector passed: OTLP traces, Prometheus metrics, privacy filtering and counter names; no egress."
            )
        except subprocess.CalledProcessError as exc:
            print(exc.stderr)
            raise
        finally:
            subprocess.run(["docker", "rm", "--force", name], capture_output=True, check=False)


if __name__ == "__main__":
    main()
