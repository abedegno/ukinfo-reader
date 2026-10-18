"""Check exact-address views against exhaustive postcode queries; publish aggregates only."""

import argparse
import json
import re
import time
import urllib.parse
import urllib.request
from pathlib import Path


def normalize(value):
    return " ".join(str(value or "").split()).lower()


def identity(row):
    code = "".join((row.get("postcode") or "").upper().split())
    if not re.fullmatch(r"(?:[A-Z]{1,2}[0-9][A-Z0-9]?[0-9][A-Z]{2}|GIR0AA)", code):
        return None
    warnings = (row.get("source_warning") or "").split(";")
    if any(w and w != "synthetic_demo_not_a_real_person" for w in warnings):
        return None
    if row.get("postcode_precision") not in (None, 0, 10):
        return None

    def parts(*values):
        result = []
        for value in values:
            value = normalize(value)
            if value and (not result or result[-1] != value):
                result.append(value)
        return tuple(result)

    streets = parts(row.get("street") or row.get("street_1"), row.get("street_2"))
    localities = parts(row.get("locality") or row.get("locality_1"), row.get("locality_2"))
    house = normalize(row.get("houseno"))
    if house == "0":
        house = ""
    building = normalize(row.get("building"))
    if not streets or not (house or building):
        return None
    return (
        code,
        house,
        building,
        normalize(row.get("subbuilding")),
        streets,
        localities,
        normalize(row.get("town")),
        normalize(row.get("county")),
    )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", default="http://127.0.0.1:58320")
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()

    def fetch(path, params=None):
        url = args.url + path
        if params:
            url += "?" + urllib.parse.urlencode(params)
        started = time.monotonic()
        with urllib.request.urlopen(url, timeout=15) as response:
            data = json.load(response)
        return data, round((time.monotonic() - started) * 1000, 2)

    datasets, _ = fetch("/api/datasets")
    complete = [
        d
        for d in datasets
        if not d["manifest"].get("sample")
        or not any(
            other["manifest"]["edition"] == d["manifest"]["edition"]
            and not other["manifest"].get("sample")
            for other in datasets
        )
    ]
    real = [d for d in complete if d["key"] != "synthetic-demo" and not d["manifest"].get("sample")]
    reports = []
    targets = [(dataset, "person") for dataset in real]
    targets += [(dataset, "business") for dataset in real if dataset["key"].startswith("se2005-")]
    for dataset, kind in targets:
        # These are private query choices, not saved source-address fixtures.
        found, _ = fetch(
            "/api/search",
            {"dataset": dataset["key"], "kind": kind, "postcode": "WS13", "limit": 100},
        )
        anchor = next((item for item in found["items"] if identity(item["record"])), None)
        if not anchor:
            raise AssertionError("No eligible anchor in this edition's bounded selection")
        key = identity(anchor["record"])
        expected = {}
        for candidate_dataset in complete:
            matches = set()
            for listing_kind in ("person", "business"):
                query = {
                    "dataset": candidate_dataset["key"],
                    "kind": listing_kind,
                    "postcode": key[0],
                    "limit": 100,
                }
                while True:
                    page, _ = fetch("/api/search", query)
                    for item in page["items"]:
                        if identity(item["record"]) == key:
                            matches.add((item["dataset"], item["source_id"]))
                    if not page["next_cursor"]:
                        break
                    query["cursor"] = page["next_cursor"]
            expected[candidate_dataset["key"]] = matches
        scopes = []
        for scope in ("edition", "all"):
            query = {
                "dataset": anchor["dataset"],
                "id": anchor["id"],
                "source_id": anchor["source_id"],
                "scope": scope,
                "limit": 2,
            }
            seen = set()
            timings = []
            while True:
                page, elapsed = fetch("/api/address", query)
                timings.append(elapsed)
                for item in page["items"]:
                    occurrence = (item["dataset"], item["source_id"])
                    if occurrence in seen or identity(item["record"]) != key:
                        raise AssertionError("Duplicate or incorrect premises match")
                    seen.add(occurrence)
                if not page["next_cursor"]:
                    break
                query["cursor"] = page["next_cursor"]
            wanted = (
                expected[anchor["dataset"]]
                if scope == "edition"
                else set().union(*expected.values())
            )
            if seen != wanted:
                raise AssertionError("Address results differ from exhaustive postcode reference")
            scopes.append(
                {
                    "scope": scope,
                    "records_checked": len(seen),
                    "page_ms": timings,
                    "reference_match": True,
                }
            )
        reports.append({"edition": dataset["key"], "anchor_kind": kind, "scopes": scopes})
        print(json.dumps(reports[-1]), flush=True)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(
        json.dumps(
            {
                "method": "One eligible person anchor per full edition plus a 2005 business anchor, from bounded postcode-district queries. Exact-premises results at page size two compared with exhaustive person and business postcode queries over all complete editions/demo. Recomputed display-field keys, scoped occurrence sets and paging uniqueness. Source anchors and fields are not written. Local HTTP, not a hosted capacity test.",
                "cases": reports,
            },
            indent=2,
        )
        + "\n"
    )


if __name__ == "__main__":
    main()
