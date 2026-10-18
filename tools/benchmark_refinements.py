"""Compare displayed-field refinements with exhaustive postcode pages; save aggregates only."""

import argparse
import json
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

from benchmark_premises import normalize


def matches(row, filters):
    for param, field in (("house", "houseno"), ("building", "building"), ("flat", "subbuilding")):
        if param not in filters:
            continue
        value = row.get(field)
        if not isinstance(value, str | int | float) or isinstance(value, bool):
            return False
        value = normalize(value)
        if field == "houseno" and value in ("0", "0.0"):
            return False
        wanted = filters[param]
        if not value or (
            not value.startswith(wanted[:-1]) if wanted.endswith("*") else value != wanted
        ):
            return False
    given = normalize(row.get("firstName"))
    if "first_name" in filters:
        wanted = filters["first_name"]
        mode = filters["first_match"]
        if mode == "given" and given != wanted and not given.startswith(wanted + " "):
            return False
        if mode == "exact" and given != wanted:
            return False
        if mode == "prefix" and not given.startswith(wanted):
            return False
    if "middle_initial" in filters:
        parts = given.replace(".", " ").split()
        if len(parts) < 2 or not parts[1].startswith(filters["middle_initial"]):
            return False
    return True


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", default="http://127.0.0.1:58320")
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()

    def fetch(path, params=None):
        started = time.monotonic()
        url = args.url + path + ("?" + urllib.parse.urlencode(params) if params else "")
        try:
            with urllib.request.urlopen(url, timeout=15) as response:
                data = json.load(response)
        except urllib.error.HTTPError as error:
            # Do not print the request URL: it contains original directory fields.
            message = json.load(error).get("error", "unavailable")
            raise RuntimeError(f"API {error.code}: {message}") from None
        return data, round((time.monotonic() - started) * 1000, 2)

    def walk(params):
        query = dict(params)
        found = {}
        timings = []
        while True:
            data, elapsed = fetch("/api/search", query)
            timings.append(elapsed)
            for item in data["items"]:
                key = (item["dataset"], item["source_id"])
                if key in found:
                    raise AssertionError("Duplicate occurrence in cursor traversal")
                found[key] = item["record"]
            if not data["next_cursor"]:
                return found, timings
            query["cursor"] = data["next_cursor"]

    datasets, _ = fetch("/api/datasets")
    real = [d for d in datasets if not d["manifest"].get("sample") and d["key"] != "synthetic-demo"]
    targets = [(d, "person") for d in real]
    targets += [(d, "business") for d in real if d["key"].startswith("se2005-")]
    reports = []
    for dataset, kind in targets:
        bounded, _ = fetch(
            "/api/search",
            {"dataset": dataset["key"], "kind": kind, "postcode": "WS13", "limit": 100},
        )
        anchor = next(
            item["record"]
            for item in bounded["items"]
            if normalize(item["record"].get("houseno")) not in ("", "0")
            and len("".join(item["record"].get("postcode", "").split())) >= 5
        )
        filters = {"house": normalize(anchor["houseno"])}
        for param, field in (("building", "building"), ("flat", "subbuilding")):
            if anchor.get(field):
                filters[param] = normalize(anchor[field])
        if kind == "person" and anchor.get("firstName"):
            given = normalize(anchor["firstName"])
            filters["first_name"], filters["first_match"] = given.split()[0], "given"
            parts = given.replace(".", " ").split()
            if len(parts) >= 2 and parts[1][0].isalpha():
                filters["middle_initial"] = parts[1][0]
        variants = [filters, {"house": filters["house"] + "*"}]
        if "first_name" in filters:
            variants += [
                {"first_name": normalize(anchor["firstName"]), "first_match": "exact"},
                {"first_name": filters["first_name"], "first_match": "prefix"},
            ]
        for scope in (dataset["key"], "all"):
            base = {"dataset": scope, "kind": kind, "postcode": anchor["postcode"], "limit": 100}
            reference, _ = walk(base)
            for variant in variants:
                expected = {key for key, row in reference.items() if matches(row, variant)}
                found, timings = walk({**base, **variant, "limit": 2})
                if set(found) != expected:
                    raise AssertionError("Refinement disagrees with exhaustive postcode reference")
                report = {
                    "edition": dataset["key"],
                    "kind": kind,
                    "scope": "all" if scope == "all" else "edition",
                    "filters": sorted(variant),
                    "reference_records": len(reference),
                    "matched_records": len(found),
                    "page_ms": timings,
                    "reference_match": True,
                }
                reports.append(report)
                print(json.dumps(report), flush=True)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(
        json.dumps(
            {
                "method": "One bounded person anchor per complete edition plus a 2005 business anchor. House/available premises/name refinements compared with exhaustive same-kind full-postcode results, edition/all scopes and page size two. Only aggregate counts and timings retained. Warm local HTTP; not a capacity test.",
                "cases": reports,
            },
            indent=2,
        )
        + "\n"
    )


if __name__ == "__main__":
    main()
