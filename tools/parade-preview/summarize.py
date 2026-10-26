"""Summarize complete private NAS trial reports without reading workload files."""

import argparse
import collections
import json
import math
from pathlib import Path


def statistics(rows):
    if not rows:
        return {"requests": 0}
    values = sorted(row["milliseconds"] for row in rows)
    errors = [row for row in rows if row["outcome"] not in {"ok", "unverified"}]
    return {
        "requests": len(rows),
        "errors": len(errors),
        "error_codes": dict(collections.Counter(row.get("code", row["outcome"]) for row in errors)),
        "p50_ms": round(values[math.ceil(len(values) * 0.5) - 1], 3),
        "p95_ms": round(values[math.ceil(len(values) * 0.95) - 1], 3),
        "p99_ms": round(values[math.ceil(len(values) * 0.99) - 1], 3),
        "max_ms": round(max(values), 3),
    }


def summarize(root):
    def read(name):
        return json.loads((root / name).read_text())

    verification = read("verification.json")
    metadata = read("case-metadata-v3.json")
    paired = read("compare.json")
    cursors = read("cursors.json")
    calibration = read("calibration.json")
    if (
        not verification["passed"]
        or len(verification["editions"]) != 5
        or sum(row["entries"] for row in verification["editions"]) != 248951107
    ):
        raise ValueError("Incomplete edition verification")
    if len({metadata["cases"], paired["cases"], cursors["cases"], calibration["cases"]}) != 1:
        raise ValueError("Different comparison cohorts")
    if paired["budget_exhausted"] or not cursors["complete"] or not calibration["complete"]:
        raise ValueError("Incomplete bounded comparison")
    if (
        paired["outcomes"].get("both_ok_mismatch", 0)
        or cursors["mismatches"]
        or calibration["mismatches"]
    ):
        raise ValueError("Content mismatches require investigation")
    by_case = {row["case"]: row for row in metadata["metadata"]}
    first = {(row["case"], row["round"]) for row in paired["pairs"] if row["page"] == 1}
    if len(first) != metadata["cases"] * paired["rounds"]:
        raise ValueError("Incomplete fixed first-page population")
    cohorts = []
    for side in ["before", "after"]:
        for scope in ["first", "later", "all"]:
            selected = [
                row[side]
                for row in paired["pairs"]
                if scope == "all" or (scope == "first") == (row["page"] == 1)
            ]
            cohorts.append(
                {
                    "engine": "native" if side == "before" else "paradedb",
                    "scope": scope,
                    **statistics(selected),
                }
            )
    capabilities = []
    for backend in ["paradedb", "native"]:
        for side in ["before", "after"]:
            selected = [
                row[side]
                for row in paired["pairs"]
                if by_case[row["case"]]["expected_backend"] == backend
            ]
            capabilities.append(
                {
                    "engine": "native" if side == "before" else "paradedb",
                    "selected_path": backend,
                    **statistics(selected),
                }
            )
    breakdowns = []
    for dimension in ["edition_ordinal", "kind", "route", "postcode", "address"]:
        for value in sorted({row[dimension] for row in by_case.values()}):
            for side in ["before", "after"]:
                selected = [
                    row[side]
                    for row in paired["pairs"]
                    if row["page"] == 1 and by_case[row["case"]][dimension] == value
                ]
                breakdowns.append(
                    {
                        "dimension": dimension,
                        "value": value,
                        "engine": "native" if side == "before" else "paradedb",
                        "scope": "first",
                        **statistics(selected),
                    }
                )
    runs = []
    for name in ["load-native", "load-candidate", "load-candidate-repeat", "load-native-repeat"]:
        run = read(name + ".json")
        if (
            run["budget_exhausted"]
            or run.get("mode") != "fixed_cohort"
            or run.get("fixed_rounds") != 2
        ):
            raise ValueError("Incomplete or duration-biased load run")
        if run["cases"] != calibration["matching_cases"]:
            raise ValueError("Different load reference subset")
        initial = run["initial_summary"]
        if (
            initial["requests"] != run["cases"]
            or len(initial["per_case"]) != run["cases"]
            or any(case["requests"] != 1 for case in initial["per_case"].values())
            or set(run["closed_loop"]) != {"1", "4"}
        ):
            raise ValueError("Incomplete initial or concurrency populations")
        if initial["outcomes"].get("hash_mismatch", 0):
            raise ValueError("Initial load content mismatches require investigation")
        for stage in run["closed_loop"].values():
            if stage["requests"] != run["cases"] * 2 or len(stage["per_case"]) != run["cases"]:
                raise ValueError("Different load case populations")
            if any(case["requests"] != 2 for case in stage["per_case"].values()):
                raise ValueError("Different load case frequencies")
            if stage["outcomes"].get("hash_mismatch", 0):
                raise ValueError("Load content mismatches require investigation")
        runs.append(
            {
                "name": name,
                "cases": run["cases"],
                "initial": {key: value for key, value in initial.items() if key != "per_case"},
                "closed_loop": {
                    level: {key: value for key, value in stage.items() if key != "per_case"}
                    for level, stage in run["closed_loop"].items()
                },
                "passed": run["passed"],
            }
        )
    environment = read("environment.json")
    storage = {
        "database_bytes": environment["database_bytes"],
        "table_bytes": sum(table["table_bytes"] for table in environment["tables"]),
        "index_bytes": sum(table["index_bytes"] for table in environment["tables"]),
    }
    rollback = read("rollback.json")
    completion = read("completion.json")
    if not rollback["passed"] or not completion["rollback_proved"]:
        raise ValueError("Offline-dependency rollback not verified")
    health_before, health_after = read("health-before.json"), read("health-after.json")
    if health_before != health_after or any(
        row["status"] != "running" or row["health"] != "healthy" for row in health_after.values()
    ):
        raise ValueError("Public service health changed")
    source_before, source_after = read("source-before.json"), read("source-after.json")
    if (
        source_before["serving"] != source_after["serving"]
        or not source_after["owner_metadata"]["rollback_exists"]
        or source_after["owner_metadata"]["rollback_entries"] != 59886841
    ):
        raise ValueError("Source visibility or rollback changed")
    pilots = {}
    for name in ["pilot-compare.json", "pilot-compare-v2.json"]:
        pilot = read(name)
        pilots[name] = {
            key: pilot[key]
            for key in ["cases", "rounds", "max_pages", "outcomes", "budget_exhausted"]
        }
        pilots[name]["engines"] = {
            side: statistics([row[side] for row in pilot["pairs"]]) for side in ["before", "after"]
        }
    for name in ["pilot-verification.json", "pilot-plans.json", "pilot-plans-v2.json"]:
        pilots[name] = read(name)
    return {
        "schema_version": 1,
        "method": {
            "cases": metadata["cases"],
            "rounds": paired["rounds"],
            "max_pages": paired["max_pages"],
            "original_record_retrieval": True,
            "includes_http": True,
            "includes_cloudflare": False,
            "cache_flushed": False,
            "candidate_additional_cpus": 2,
            "candidate_additional_memory_bytes": 6 * 1024**3,
            "mixed_engine_load": False,
            "load_order": [run["name"] for run in runs],
            "load_subset_matching_cases": calibration["matching_cases"],
            "load_subset_excluded_cases": metadata["cases"] - calibration["matching_cases"],
        },
        "verification": verification,
        "build": read("build-timing.json"),
        "paired_outcomes": paired["outcomes"],
        "cohorts": cohorts,
        "capabilities": capabilities,
        "breakdowns": breakdowns,
        "load_runs": runs,
        "cursor_checks": cursors,
        "storage": storage,
        "versions": {key: environment[key] for key in ["version", "extension"]},
        "plans": read("plans.json"),
        "resources": read("resources.json"),
        "source_before": source_before,
        "source_after": source_after,
        "health_before": health_before,
        "health_after": health_after,
        "rollback": {
            key: rollback[key]
            for key in ["cases", "requests_attempted", "passed", "budget_exhausted"]
        },
        "completion": completion,
        "pilots": pilots,
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    result = summarize(args.directory)
    args.output.write_text(json.dumps(result, indent=2) + "\n")
    for row in result["cohorts"]:
        print(row)


if __name__ == "__main__":
    main()
