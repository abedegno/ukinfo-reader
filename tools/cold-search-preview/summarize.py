"""Summarize allowlisted aggregate reports; never open workload or golden files."""

import argparse
import json
import math
from pathlib import Path


def read(root, name):
    return json.loads((root / (name + ".json")).read_text())


def measures(rows, key):
    values = sorted(r[key]["milliseconds"] for r in rows)
    return {
        "requests": len(rows),
        "errors": sum(r[key]["status"] != 200 for r in rows),
        "median_ms": values[len(values) // 2],
        "p95_ms": values[math.ceil(len(values) * 0.95) - 1],
    }


def observations(report):
    return [
        {k: row[k] for k in ("case", "status", "milliseconds", "success")}
        for row in report["observations"]
    ]


def build(final, cold, initial_load):
    comparison = read(final, "compare")
    assert comparison["cases"] == 294 and comparison["rounds"] == 2
    assert not comparison["budget_exhausted"]
    assert not comparison["outcomes"].get("both_ok_mismatch", 0)
    first = [p for p in comparison["pairs"] if p["page"] == 1]
    assert len(first) == 588
    assert all(sum(p["case"] == i for p in first) == 2 for i in range(294))
    references = {}
    for name, expected in [
        ("business-reference-compare", 4),
        ("cold-reference-compare", 16),
        ("native-fallback-reference-compare", 4),
    ]:
        r = read(final, name)
        references[name] = {k: r[k] for k in ("complete", "requests", "errors", "mismatches")}
        assert r["complete"] and r["requests"] == expected and r["errors"] == r["mismatches"] == 0
    loads = {}
    for side, concurrency, repeat in [
        ("baseline", 1, False),
        ("candidate", 1, False),
        ("baseline", 4, False),
        ("candidate", 4, False),
        ("candidate", 4, True),
        ("baseline", 4, True),
    ]:
        name = f"load-{side}-c{concurrency}" + ("-repeat" if repeat else "")
        r = read(final, name)
        assert r["cases"] == 292 and r["requests_attempted"] == 876 and not r["budget_exhausted"]
        s = r["closed_loop"][str(concurrency)]
        assert s["requests"] == 584 and not s["outcomes"].get("hash_mismatch", 0)
        if side == "candidate":
            assert s["errors"] == r["initial_summary"]["errors"] == 0
        loads[name] = {
            k: s[k]
            for k in ("requests", "errors", "median_ms", "p95_ms", "p99_ms", "requests_per_second")
        }
        loads[name]["initial_errors"] = r["initial_summary"]["errors"]
    resets = []
    for side, rounds in [("native", [1]), ("candidate", [1, 2])]:
        for round_ in rounds:
            for case in (56, 166, 9, 95):
                name = f"{round_}-{side}-{case}"
                cache = read(cold, "cache-" + name)
                assert cache["at_most_one_mib_resident"] and not cache["host_cache_flush"]
                r = read(cold, "cold-" + name + "-report")
                assert r["complete"] and r["requests"] == 1
                if side == "candidate":
                    gate = read(cold, "source-check-" + name)
                    assert gate["errors"] == gate["mismatches"] == 0
                resets.append({"side": side, "round": round_, **observations(r)[0]})
    final_cold = []
    for case in (56, 166, 9, 95, 232):
        cache = read(final, f"cache-{case}")
        assert cache["at_most_one_mib_resident"] and not cache["host_cache_flush"]
        r = read(final, f"cold-{case}-report")
        assert r["complete"] and r["requests"] == 1
        gate = read(final, f"source-check-{case}")
        assert gate["errors"] == gate["mismatches"] == 0
        final_cold.extend(observations(r))
    completion = read(final, "completion")
    assert completion["complete"] and completion["candidate_failures"] == 0
    assert (
        completion["public_unchanged"]
        and completion["target_lifecycle_unchanged"]
        and completion["private_source_stopped"]
    )
    mapping = read(initial_load, "load-metadata")["metadata"]
    failing = next(v for v in mapping if v["case"] == 230)
    assert failing["source_case"] == 232
    return {
        "schema_version": 1,
        "cohort": {
            "cases": 294,
            "rounds": 2,
            "maximum_pages": 10,
            "pairs": len(comparison["pairs"]),
            "outcomes": comparison["outcomes"],
            "first_pages": {s: measures(first, s) for s in ("before", "after")},
        },
        "source_reference_checks": references,
        "loads": loads,
        "repeated_source_cold": resets,
        "final_source_cold": final_cold,
        "concurrency_case_mapping": {"load_case": 230, "source_case": 232},
        "target_cache": "shared_uncontrolled",
        "source_cold_preparation": "per-file eviction of the stopped private source clone",
        "source_and_target_production_lifecycle_unchanged": True,
        "serving_limits_unchanged": True,
    }


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--final", required=True, type=Path)
    parser.add_argument("--cold", required=True, type=Path)
    parser.add_argument("--initial-load", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    args.output.write_text(
        json.dumps(build(args.final, args.cold, args.initial_load), indent=2) + "\n"
    )
