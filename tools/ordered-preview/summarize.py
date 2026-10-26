"""Export aggregate ordered-search evidence; never read private queries or responses."""

import argparse
import json
from pathlib import Path


def load(root, name):
    return json.loads((root / (name + ".json")).read_text())


def summary(row):
    fields = ("requests", "errors", "median_ms", "p95_ms", "p99_ms", "requests_per_second")
    return {k: row[k] for k in fields if k in row}


def build(warm, cold):
    comparison = load(warm, "compare")
    assert comparison["cases"] == 294 and comparison["rounds"] == 2
    assert not comparison["budget_exhausted"]
    assert not comparison["outcomes"].get("both_ok_mismatch", 0)
    pairs = comparison["pairs"]
    first = [p for p in pairs if p["page"] == 1]
    assert len(first) == 588
    oracle = load(warm, "bounded-native-reference-report")
    assert oracle["complete"] and oracle["pages"] == 4 and oracle["errors"] == 0
    reference = load(warm, "reference-compare")
    assert reference["requests"] == 8 and reference["errors"] == reference["mismatches"] == 0
    cursors = load(warm, "cursors")
    assert cursors["complete"] and not cursors["mismatches"]
    completion = load(warm, "full-completion")
    assert completion["complete"] and not completion["public_changed"]
    assert completion["target_lifecycle_unchanged"] and completion["private_source_stopped"]
    calibration = load(warm, "calibration")
    assert calibration["complete"] and not calibration["mismatches"]
    runs = {}
    for name in ("load-native", "load-candidate", "load-candidate-repeat", "load-native-repeat"):
        run = load(warm, name)
        assert not run["budget_exhausted"] and run["cases"] == 292
        assert run["requests_attempted"] == 1460
        selected = {}
        for concurrency in ("1", "4"):
            phase = run["closed_loop"][concurrency]
            assert phase["requests"] == 584 and not phase["outcomes"].get("hash_mismatch", 0)
            selected[concurrency] = summary(phase)
        runs[name] = selected
    cold_completion = load(cold, "cold-completion")
    assert cold_completion["complete"] and not cold_completion["public_changed"]
    assert (
        cold_completion["target_lifecycle_unchanged"] and cold_completion["private_source_stopped"]
    )
    cold_runs = []
    for ordinal in (91, 92, 56, 166, 95, 9, 230):
        for side in ("native", "candidate"):
            cache = load(cold, f"cache-{ordinal}-{side}")
            assert cache["at_most_one_mib_resident"] and not cache["host_cache_flush"]
            snapshot = load(cold, f"cold-{ordinal}-{side}-report")
            assert snapshot["complete"] and snapshot["requests"] == 1
            cold_runs.append(
                {
                    "case": ordinal,
                    "side": side,
                    "source_residency_gate": True,
                    "observations": snapshot["observations"],
                }
            )
        assert not load(cold, f"cold-{ordinal}-pair")["mismatches"]
    # Ordinals, timings and fixed status codes only. No labels, queries, IDs,
    # response hashes, cursor tokens or record values enter this public artifact.
    out = {
        "schema_version": 1,
        "cases": 294,
        "rounds": 2,
        "pair_outcomes": comparison["outcomes"],
        "pairs": len(pairs),
        "first_page_requests_per_backend": len(first),
        "first_page_errors": {
            side: sum(p[side]["status"] != 200 for p in first) for side in ("before", "after")
        },
        "targeted_first_pages": [
            {
                "case": p["case"],
                "round": p["round"],
                "before_ms": p["before"]["milliseconds"],
                "after_ms": p["after"]["milliseconds"],
                "before_status": p["before"]["status"],
                "after_status": p["after"]["status"],
            }
            for p in first
            if p["case"] in (91, 92)
        ],
        "source_reference_pages": 4,
        "source_reference_checks": 8,
        "cursor_mismatches": 0,
        "loads": runs,
        "source_cold": cold_runs,
        "target_cache": "shared_uncontrolled",
        "public_unchanged_during_trial": True,
    }
    return out


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--warm", required=True, type=Path)
    parser.add_argument("--cold", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    args.output.write_text(json.dumps(build(args.warm, args.cold), indent=2) + "\n")
