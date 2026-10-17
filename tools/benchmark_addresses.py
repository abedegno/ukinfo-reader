"""Validate and time local address searches; write source-free aggregate results.

No source names, addresses, IDs or response bodies are written. The checks recompute
address/name/postcode predicates from each returned original record. This establishes
returned-row correctness and paging uniqueness, not an independent completeness proof.
"""

import argparse
import concurrent.futures
import json
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path


def normalize(value):
    return " ".join((value or "").split()).lower()


def matches(value, query):
    value = normalize(value)
    query = normalize(query)
    return value.startswith(query[:-1]) if query.endswith("*") else value == query


def validate(record, query):
    components = {
        "street": [record.get("street") or record.get("street_1"), record.get("street_2")],
        "locality": [record.get("locality") or record.get("locality_1"), record.get("locality_2")],
        "town": [record.get("town")],
        "county": [record.get("county")],
    }
    for field, values in components.items():
        if query.get(field) and not any(matches(value, query[field]) for value in values):
            raise AssertionError(f"Returned record does not match {field}")
    for field, source in [("surname", "surname"), ("organisation", "organisation")]:
        if query.get(field) and not matches(record.get(source), query[field]):
            raise AssertionError(f"Returned record does not match {field}")
    if (record.get("entry_type") == "business") != (query.get("kind") == "business"):
        raise AssertionError("Wrong directory type")
    if query.get("postcode"):
        code = "".join((record.get("postcode") or "").upper().split())
        prefix = "".join(query["postcode"].upper().split())
        # This workload uses an unambiguous outward district, not W11/W1 1.
        if not code.startswith(prefix) or len(code) != len(prefix) + 3:
            raise AssertionError("Wrong postcode district")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", default="http://127.0.0.1:58320")
    parser.add_argument("--dataset", required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--pages", type=int, default=5)
    args = parser.parse_args()
    cases = [
        ("street-exact", {"street": "High Street"}),
        ("street-prefix", {"street": "High*"}),
        ("town", {"town": "Lichfield"}),
        ("county", {"county": "Staffordshire"}),
        ("large-town", {"town": "London"}),
        ("street-town", {"street": "High*", "town": "Lichfield"}),
        ("name-county", {"surname": "Smith", "county": "Staffordshire"}),
        ("name-prefix-county", {"surname": "Smi*", "county": "Stafford*"}),
        ("locality", {"locality": "Shenstone"}),
        ("street-postcode", {"street": "High*", "postcode": "WS13"}),
        ("business-county", {"kind": "business", "county": "Staffordshire"}),
        ("business-large-town", {"kind": "business", "town": "London"}),
        ("absent-town", {"town": "Invented Absent Place ZZZZZ"}),
    ]

    def request(query):
        started = time.monotonic()
        try:
            with urllib.request.urlopen(
                args.url + "/api/search?" + urllib.parse.urlencode(query), timeout=15
            ) as response:
                result = json.load(response)
            return result, round((time.monotonic() - started) * 1000, 2), None
        except urllib.error.HTTPError as error:
            return None, round((time.monotonic() - started) * 1000, 2), error.code

    results = []
    for label, filters in cases:
        query = {"dataset": args.dataset, "limit": 50, **filters}
        seen = set()
        timings = []
        error = None
        for _ in range(args.pages):
            data, elapsed, error = request(query)
            timings.append(elapsed)
            if error:
                break
            for item in data["items"]:
                identity = (item["dataset"], item["source_id"])
                if identity in seen:
                    raise AssertionError("Duplicate occurrence across pages")
                seen.add(identity)
                validate(item["record"], filters)
            if not data["next_cursor"]:
                break
            query["cursor"] = data["next_cursor"]
        result = {
            "case": label,
            "records_checked": len(seen),
            "page_ms": timings,
            "error_status": error,
        }
        results.append(result)
        print(json.dumps(result), flush=True)

    concurrent_query = {"dataset": args.dataset, "town": "Lichfield", "limit": 50}
    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
        parallel_results = list(pool.map(lambda _: request(concurrent_query), range(12)))
    for data, _, error in parallel_results:
        if not error:
            for item in data["items"]:
                validate(item["record"], concurrent_query)
    report = {
        "dataset": args.dataset,
        "method": "Local HTTP; sequential keyset pages of 50; recomputed filters on original records; per-case occurrence uniqueness. Four callers/12 requests to one repeated town query. No physical cold-cache claim or independent completeness proof.",
        "cases": results,
        "concurrency": {
            "callers": 4,
            "requests": 12,
            "ms": [x[1] for x in parallel_results],
            "errors": sum(x[2] is not None for x in parallel_results),
        },
    }
    args.output.write_text(json.dumps(report, indent=2) + "\n")
    if any(row["error_status"] for row in results) or report["concurrency"]["errors"]:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
