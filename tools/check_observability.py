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
received = set()
class Sink(http.server.BaseHTTPRequestHandler):
    def log_message(self, *args): pass
    def do_POST(self):
        body = self.rfile.read(int(self.headers.get('Content-Length', 0)))
        assert self.headers.get('X-SF-Token') == 'invented-test-token', 'Invalid auth header'
        assert body, 'Empty export payload'
        received.add(self.path)
        self.send_response(200); self.send_header('Content-Type', 'application/x-protobuf'); self.end_headers()
threading.Thread(target=http.server.HTTPServer(('127.0.0.1',14318), Sink).serve_forever, daemon=True).start()
scrapes = 0
class Metrics(http.server.BaseHTTPRequestHandler):
    def log_message(self, *args): pass
    def do_GET(self):
        self.send_response(200); self.end_headers()
        global scrapes
        scrapes += 1
        latency = f'# TYPE ukinfo_api_request_duration_seconds histogram\nukinfo_api_request_duration_seconds_bucket{{le="1"}} {2*scrapes}\nukinfo_api_request_duration_seconds_bucket{{le="+Inf"}} {3*scrapes}\nukinfo_api_request_duration_seconds_count {3*scrapes}\nukinfo_api_request_duration_seconds_sum {2*scrapes}\n' 
        self.wfile.write(latency.encode())
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
        if 'ukinfo_pool_acquired' in text and 'HTTP /api/search' in text and 'ukinfo_api_request_duration_seconds' in text and received == {'/v1/metrics', '/v1/traces'}: break
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
        # Keep the real HTTP exporter and file-provider authentication path.
        # Add a file exporter in parallel to inspect the filtered signal contents.
        config = config.replace(
            "exporters:\n", "exporters:\n  file/test:\n    path: /out/signals.json\n", 1
        )
        config = (
            config.replace("[otlphttp/splunk]", "[otlphttp/splunk, file/test]")
            .replace("scrape_interval: 60s", "scrape_interval: 2s")
            .replace("timeout: 10s", "timeout: 1s")
            .replace(
                "https://ingest.${env:SPLUNK_REALM}.observability.splunkcloud.com/v2/trace/otlp",
                "http://127.0.0.1:14318/v1/traces",
            )
            .replace(
                "https://ingest.${env:SPLUNK_REALM}.observability.splunkcloud.com/v2/datapoint/otlp",
                "http://127.0.0.1:14318/v1/metrics",
            )
        )
        # The file provider preserves bytes, including newlines. HTTP headers
        # require exactly the token value with no trailing line ending.
        (directory / "token").write_bytes(b"invented-test-token")
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
                "--volume",
                f"{directory}/token:/run/secrets/splunk_ingest_token:ro",
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
            histograms = [
                metric["histogram"]
                for item in signals
                for resource in item.get("resourceMetrics", [])
                for scope in resource.get("scopeMetrics", [])
                for metric in scope.get("metrics", [])
                if metric["name"] == "ukinfo_api_request_duration_seconds"
            ]
            assert histograms and all(h["aggregationTemporality"] == 1 for h in histograms), (
                "Latency histogram must export delta temporality"
            )
            points = [point for histogram in histograms for point in histogram["dataPoints"]]
            assert points and all(
                int(point["count"]) == 3 and list(map(int, point["bucketCounts"])) == [2, 1]
                for point in points
            ), "Delta histogram did not preserve interval counts"
            print(
                "Collector passed: OTLP traces, Prometheus metrics, privacy filtering, counter names and authenticated HTTP export; no egress."
            )
        except subprocess.CalledProcessError as exc:
            print(exc.stderr)
            raise
        finally:
            subprocess.run(["docker", "rm", "--force", name], capture_output=True, check=False)


if __name__ == "__main__":
    main()
