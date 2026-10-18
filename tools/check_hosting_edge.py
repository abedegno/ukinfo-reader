"""Check the nginx example's TLS and quotas with a dummy upstream and no network.

Requires Docker, Go and OpenSSL. No catalogue or credentials are mounted. This
checks edge policy, not a deployed host, DNS, public certificates or Go throughput.
"""

import json
import os
import subprocess
import tempfile
from pathlib import Path

CLIENT = r"""
package main
import("crypto/tls";"crypto/x509";"encoding/json";"fmt";"io";"net/http";"os";"time")
func main(){
 cert,e:=os.ReadFile("/verify/cert/fullchain.pem");if e!=nil{panic(e)}
 roots:=x509.NewCertPool();if !roots.AppendCertsFromPEM(cert){panic("test certificate")}
 client:=&http.Client{Transport:&http.Transport{TLSClientConfig:&tls.Config{RootCAs:roots,ServerName:"archive.example.org",MinVersion:tls.VersionTLS12}},Timeout:2*time.Second}
 get:=func(path string)int{response,e:=client.Get("https://127.0.0.1"+path);if e!=nil{return 0};io.Copy(io.Discard,response.Body);response.Body.Close();return response.StatusCode}
 for i:=0;i<50;i++{if get("/")==200{break};if i==49{panic("test edge not ready")};time.Sleep(100*time.Millisecond)}
 results:=map[string]map[int]int{}
 for _,stage:=range []struct{name,path string;requests,accepted int}{{"search","/api/health",20,11},{"export","/api/export",6,3}}{
  counts:=map[int]int{};for i:=0;i<stage.requests;i++{counts[get(stage.path)]++}
  if counts[200]!=stage.accepted || counts[429]!=stage.requests-stage.accepted{panic(fmt.Sprintf("unexpected %s quota results: %v",stage.name,counts))};results[stage.name]=counts
 }
 json.NewEncoder(os.Stdout).Encode(map[string]any{"tls_verified":true,"upstream":"invented static responses","quotas":results})
}
"""


def check(root):
    image = "nginx:1.29.5-alpine"
    # Pull only if absent; the test image is not a recommended production image pin.
    probe = subprocess.run(
        ["docker", "image", "inspect", "--format", "{{.Architecture}}", image],
        capture_output=True,
        text=True,
    )
    if probe.returncode:
        subprocess.run(["docker", "pull", image], check=True)
        probe = subprocess.run(
            ["docker", "image", "inspect", "--format", "{{.Architecture}}", image],
            capture_output=True,
            text=True,
            check=True,
        )
    with tempfile.TemporaryDirectory(prefix="ukinfo-edge-") as directory:
        directory = Path(directory)
        cert = directory / "cert"
        cert.mkdir()
        (directory / "cert.conf").write_text(
            "[req]\nprompt=no\ndistinguished_name=dn\nx509_extensions=ext\n[dn]\nCN=archive.example.org\n[ext]\nsubjectAltName=DNS:archive.example.org\n"
        )
        subprocess.run(
            [
                "openssl",
                "req",
                "-x509",
                "-newkey",
                "rsa:2048",
                "-nodes",
                "-days",
                "1",
                "-config",
                str(directory / "cert.conf"),
                "-keyout",
                str(cert / "privkey.pem"),
                "-out",
                str(cert / "fullchain.pem"),
            ],
            check=True,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        (directory / "client.go").write_text(CLIENT)
        environment = {
            **os.environ,
            "GOOS": "linux",
            "GOARCH": probe.stdout.strip(),
            "CGO_ENABLED": "0",
            "GOTOOLCHAIN": "go1.26.9",
        }
        subprocess.run(
            ["go", "build", "-o", str(directory / "client"), str(directory / "client.go")],
            env=environment,
            check=True,
        )
        (directory / "nginx.conf").write_text(
            'events {}\nhttp { server {listen 127.0.0.1:8080; location / { return 200 "invented upstream"; }} include /ukinfo.conf; }\n'
        )
        container = subprocess.check_output(
            [
                "docker",
                "run",
                "--detach",
                "--rm",
                "--network",
                "none",
                "--memory",
                "128m",
                "--cpus",
                "1",
                "--read-only",
                "--tmpfs",
                "/var/cache/nginx",
                "--tmpfs",
                "/var/run",
                "--tmpfs",
                "/var/log/nginx",
                "-v",
                f"{directory}:/verify:ro",
                "-v",
                f"{directory / 'nginx.conf'}:/etc/nginx/nginx.conf:ro",
                "-v",
                f"{root / 'deploy/nginx/ukinfo.conf'}:/ukinfo.conf:ro",
                "-v",
                f"{cert}:/etc/letsencrypt/live/archive.example.org:ro",
                "--entrypoint",
                "nginx",
                image,
                "-g",
                "daemon off;",
            ],
            text=True,
        ).strip()
        try:
            raw = subprocess.check_output(
                ["docker", "exec", container, "/verify/client"], text=True
            )
            print(json.dumps(json.loads(raw), sort_keys=True))
        finally:
            subprocess.run(
                ["docker", "rm", "--force", container], check=True, stdout=subprocess.DEVNULL
            )


if __name__ == "__main__":
    check(Path(__file__).resolve().parents[1])
