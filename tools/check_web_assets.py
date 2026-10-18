"""Verify vendored Leaflet against its upstream published 1.9.4 integrity hashes."""

import base64
import hashlib
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1] / "server/cmd/ukinfo-server/web"
EXPECTED = {
    "leaflet.js": "20nQCchB9co0qIjJZRGuk2/Z9VM+kNiyxNV1lvTlZBo=",
    "leaflet.css": "p4NxAoJBhIIN+hmNHrzRCf9tD/miZyoHS5obTRR9BMY=",
}
for name, expected in EXPECTED.items():
    actual = base64.b64encode(hashlib.sha256((ROOT / name).read_bytes()).digest()).decode()
    if actual != expected:
        raise SystemExit(f"Unexpected Leaflet asset hash: {name}")
if not (ROOT / "leaflet-LICENSE.txt").is_file():
    raise SystemExit("Leaflet BSD license must accompany the assets")
print("Vendored Leaflet 1.9.4 hashes and license verified.")
