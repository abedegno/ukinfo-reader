"""Validate research groups/orders against exhaustive bounded occurrence searches.

Writes aggregates only. Original fields/queries stay in memory; optional load cases
contain private query values and must be written under private/. This is end-to-end
completeness relative to the mature occurrence endpoint, not a new source-file audit.
"""

import argparse
import hashlib
import json
import math
import re
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

from benchmark_premises import identity, normalize


def grouping(item):
    row = item["record"]
    premises = identity(row)
    business = row.get("entry_type") == "business"
    name = normalize(row.get("organisation") if business else row.get("surname"))
    first = "" if business else normalize(row.get("firstName"))
    if not premises or not name or (not business and not first):
        return "occurrence", item["dataset"], item["source_id"]
    return (
        "listing",
        row.get("entry_type", ""),
        name,
        first,
        normalize(row.get("initial2")),
        premises,
        normalize(row.get("phone")),
        normalize(row.get("industry")),
        normalize(row.get("sic_code")),
    )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", default="http://127.0.0.1:58321")
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--scope", default="all")
    parser.add_argument("--private-cases", type=Path)
    args = parser.parse_args()
    if args.private_cases and "private" not in args.private_cases.resolve().parts:
        parser.error("load cases include query values; choose a path under private/")
    load_cases = []

    def fetch(path, query=None, save=False):
        encoded = urllib.parse.urlencode(query or {})
        started = time.monotonic()
        try:
            with urllib.request.urlopen(
                args.url + path + ("?" + encoded if encoded else ""), timeout=12
            ) as response:
                raw = response.read()
        except urllib.error.HTTPError as error:
            # Hide the request URL/values in the exception and in chained traces.
            raise RuntimeError(
                f"API {error.code}: {json.load(error).get('error', 'unavailable')}"
            ) from None
        elapsed = round((time.monotonic() - started) * 1000, 2)
        if save:
            load_cases.append(
                {"path": path, "query": encoded, "sha256": hashlib.sha256(raw).hexdigest()}
            )
        if path == "/api/export" or (query and query.get("format") == "jsonl"):
            return [json.loads(line) for line in raw.splitlines()], elapsed
        return json.loads(raw), elapsed

    def walk(path, query, group=False):
        query = dict(query)
        found, timings, geography = {}, [], None
        while True:
            page, elapsed = fetch(path, query)
            timings.append(elapsed)
            geography = page.get("geography") or geography
            for item in page["items"]:
                key = item["group_key"] if group else (item["dataset"], item["source_id"])
                if key in found:
                    raise AssertionError("Repeated occurrence/group across keyset pages")
                found[key] = item
            if not page["next_cursor"]:
                return found, timings, geography
            query["cursor"] = page["next_cursor"]

    datasets, _ = fetch("/api/datasets")
    metadata = {item["key"]: item for item in datasets}
    complete = [
        item
        for item in datasets
        if not item["manifest"].get("sample") and (args.scope == "all" or item["key"] == args.scope)
    ]
    anchors, reports = [], []
    for dataset in complete:
        page, _ = fetch(
            "/api/search",
            {"dataset": dataset["key"], "surname": "Smith", "first_name": "John", "limit": 100},
        )
        added = 0
        for item in page["items"]:
            if identity(item["record"]) and item["record"]["postcode"] not in anchors:
                anchors.append(item["record"]["postcode"])
                added += 1
                if added == 2:
                    break
    if len(anchors) < 2:
        raise AssertionError("Not enough independently chosen complete source addresses")
    for number, code in enumerate(anchors):
        query = {"dataset": args.scope, "kind": "person", "postcode": code, "limit": 100}
        occurrences, source_ms, _ = walk("/api/search", query)
        expected = {}
        for ref, item in occurrences.items():
            expected.setdefault(grouping(item), set()).add(ref)
        grouped, group_ms, _ = walk(
            "/api/search", {**query, "group": "listing", "limit": 7}, group=True
        )
        expected_sets = {frozenset(refs) for refs in expected.values()}
        actual_sets = set()
        for key, item in grouped.items():
            members, _, _ = walk("/api/group", {"dataset": args.scope, "group_key": key})
            refs = frozenset(members)
            if refs not in expected_sets or len(refs) != item["occurrence_count"]:
                raise AssertionError(
                    "Group membership differs from independent original-field grouping"
                )
            for ref, source in members.items():
                if source["record"] != occurrences[ref]["record"]:
                    raise AssertionError("Source fields changed in group view")
            actual_sets.add(refs)
        if actual_sets != expected_sets:
            raise AssertionError("Grouped pages omitted an occurrence partition")
        order_ms = {}
        for mode in ["name", "edition"]:
            ordered, timings, _ = walk("/api/search", {**query, "sort": mode, "limit": 11})
            if set(ordered) != set(occurrences):
                raise AssertionError("Ordered traversal differs from exhaustive source occurrences")
            keys = []
            for item in ordered.values():
                row = item["record"]
                name = normalize(row.get("surname")), normalize(row.get("firstName"))
                if mode == "edition":
                    match = re.search(
                        r"([12][0-9]{3})-([01][0-9])",
                        metadata[item["dataset"]]["manifest"]["edition"],
                    )
                    date = int(match[1]) * 12 + int(match[2]) if match else 0
                    keys.append((-date, *name))
                else:
                    keys.append(name)
            if keys != sorted(keys):
                raise AssertionError("Semantic ordering changed across a page boundary")
            order_ms[mode] = timings
        fetch("/api/search", {**query, "limit": 50}, save=True)
        fetch("/api/search", {**query, "limit": 50, "group": "listing"}, save=True)
        fetch("/api/search", {**query, "limit": 50, "sort": "name"}, save=True)
        fetch("/api/export", {**query, "limit": 50}, save=True)
        report = {
            "case": number,
            "occurrences": len(occurrences),
            "independent_groups": len(expected),
            "source_page_ms": source_ms,
            "group_page_ms": group_ms,
            "order_page_ms": order_ms,
            "membership_exact": True,
        }
        reports.append(report)
        print(json.dumps(report), flush=True)
    # Nearby validation uses a small radius, exhaustively comparing both orders.
    nearby_report = None
    for code in anchors:
        points, _ = fetch("/api/map", {"postcode": code})
        if not points.get("points"):
            continue
        query = {
            "dataset": args.scope,
            "kind": "person",
            "near": code,
            "radius_m": 100,
            "limit": 100,
        }
        source, _, geography = walk("/api/search", query)
        ordered, timings, _ = walk("/api/search", {**query, "sort": "nearest", "limit": 13})
        if set(source) != set(ordered):
            raise AssertionError("Nearest ordering lost or added source occurrences")
        distances = [item["distance_m"] for item in ordered.values()]
        if distances != sorted(distances):
            raise AssertionError("Nearest ordering changes across page boundary")
        coordinate_points = {}
        codes = sorted({item["record"]["postcode"] for item in ordered.values()})
        for start in range(0, len(codes), 100):
            encoded = urllib.parse.urlencode(
                [("postcode", item) for item in codes[start : start + 100]]
            )
            points, _ = fetch("/api/map?" + encoded)
            coordinate_points.update(
                {point["postcode"].replace(" ", "").upper(): point for point in points["points"]}
            )
        for item in ordered.values():
            point = coordinate_points[item["record"]["postcode"].replace(" ", "").upper()]
            latitude, longitude = math.radians(point["latitude"]), math.radians(point["longitude"])
            centre_lat, centre_lon = (
                math.radians(geography["latitude"]),
                math.radians(geography["longitude"]),
            )
            haversine = (
                math.sin((latitude - centre_lat) / 2) ** 2
                + math.cos(latitude)
                * math.cos(centre_lat)
                * math.sin((longitude - centre_lon) / 2) ** 2
            )
            expected_distance = 2 * 6371008.8 * math.asin(min(1, math.sqrt(haversine)))
            if abs(expected_distance - item["distance_m"]) > 0.1:
                raise AssertionError("Returned distance disagrees with coordinate reference")
        fetch("/api/search", {**query, "sort": "nearest", "limit": 50}, save=True)
        nearby_report = {"occurrences": len(source), "page_ms": timings, "coordinate_check": True}
        break
    if nearby_report is None:
        raise AssertionError("No usable postcode centre for nearest-order validation")
    business_reports = []
    business_source = next((item for item in complete if item["key"].startswith("se2005-")), None)
    if business_source:
        business_anchors = []
        for code in anchors:
            page, _ = fetch(
                "/api/search",
                {
                    "dataset": business_source["key"],
                    "kind": "business",
                    "postcode": code.split()[0],
                    "limit": 100,
                },
            )
            for item in page["items"]:
                row = item["record"]
                if (
                    isinstance(row.get("industry"), str)
                    and isinstance(row.get("sic_code"), str)
                    and row["industry"]
                    and row["sic_code"]
                    and identity(row)
                ):
                    if row["postcode"] not in business_anchors:
                        business_anchors.append(row["postcode"])
                    break
            if len(business_anchors) == 3:
                break
        if not business_anchors:
            raise AssertionError("No resolved business classification anchor")
        for number, code in enumerate(business_anchors):
            # A district exercises real classification page boundaries; a single
            # full postcode often contains only one business in each category.
            query = {
                "dataset": args.scope,
                "kind": "business",
                "postcode": code.split()[0],
                "limit": 100,
            }
            reference, _, _ = walk("/api/search", query)
            anchor = next(
                item["record"]
                for item in reference.values()
                if isinstance(item["record"].get("industry"), str)
                and isinstance(item["record"].get("sic_code"), str)
                and item["record"]["industry"]
                and item["record"]["sic_code"]
            )
            industry, sic = normalize(anchor["industry"]), normalize(anchor["sic_code"])
            filters = [
                {"industry": industry},
                {"sic": sic},
                {"industry": industry[:2] + "*"},
                {"sic": sic[:2] + "*"},
                {"industry": industry, "sic": sic},
                {"industry": industry, "organisation": anchor["organisation"]},
            ]
            sizes, timings = [], []
            for values in filters:
                expected = set()
                for ref, item in reference.items():
                    for param, wanted in values.items():
                        field = {"sic": "sic_code"}.get(param, param)
                        raw_value = item["record"].get(field)
                        value = normalize(raw_value) if isinstance(raw_value, str) else ""
                        wanted = normalize(wanted)
                        if not value or (
                            not value.startswith(wanted[:-1])
                            if wanted.endswith("*")
                            else value != wanted
                        ):
                            break
                    else:
                        expected.add(ref)
                actual, elapsed, _ = walk("/api/search", {**query, **values, "limit": 2})
                if set(actual) != expected:
                    raise AssertionError("Business filter traversal differs from source fields")
                sizes.append(len(actual))
                timings.extend(elapsed)
            for values in (filters[2], filters[3], filters[-1]):
                fetch("/api/search", {**query, **values, "limit": 50}, save=True)
            business_reports.append(
                {
                    "case": number,
                    "source_occurrences": len(reference),
                    "filter_result_counts": sizes,
                    "classification_pages_exercised": any(size > 2 for size in sizes),
                    "page_ms": timings,
                    "membership_exact": True,
                }
            )
        if not any(item["classification_pages_exercised"] for item in business_reports):
            raise AssertionError("Business validation did not exercise a real page boundary")
    if args.private_cases:
        args.private_cases.parent.mkdir(parents=True, exist_ok=True)
        args.private_cases.write_text(json.dumps(load_cases, indent=2) + "\n")
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(
        json.dumps(
            {
                "method": "Exhaustive bounded occurrence queries, independently recomputed grouping tuples, exact source-set comparison for group/source/order traversals; no new source-file completeness proof. Query values and source records excluded.",
                "cases": reports,
                "nearby": nearby_report,
                "business": business_reports,
                "private_load_cases": len(load_cases),
            },
            indent=2,
        )
        + "\n"
    )


if __name__ == "__main__":
    main()
