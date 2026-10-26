"""Summarize aggregate hybrid/cold reports; never read private inputs or digests."""

import argparse
import collections
import json
from pathlib import Path

from summarize import statistics


def summarize(root):
    def read(name):
        return json.loads((root / name).read_text())

    verification = read("verification.json")
    if (
        not verification["passed"]
        or len(verification["editions"]) != 5
        or sum(x["entries"] for x in verification["editions"]) != 248951107
        or any(
            x["fields_matched"] != 256 or x["independent_matched"] != 256
            for x in verification["editions"]
        )
    ):
        raise ValueError("Full projection/clone verification required")
    metadata = read("metadata.json")
    identity = read("workload-identity.json")
    if (
        not identity["byte_identical_to_previous_corpus"]
        or identity["query_values_or_hashes_exported"]
    ):
        raise ValueError("Frozen workload identity not verified")
    if metadata["cases"] != 294 or metadata["editions"] != 5:
        raise ValueError("Frozen workload changed")
    meta = {x["case"]: x for x in metadata["metadata"]}
    if set(meta) != set(range(294)) or any(
        x["expected_backend"] == "paradedb" and not x["address"] for x in meta.values()
    ):
        raise ValueError("Invalid hybrid routing metadata")
    paired = read("compare.json")
    if (
        paired["budget_exhausted"]
        or paired["cases"] != 294
        or paired["rounds"] != 2
        or paired["outcomes"].get("both_ok_mismatch", 0)
    ):
        raise ValueError("Incomplete or mismatched full comparison")
    first = {(x["case"], x["round"]) for x in paired["pairs"] if x["page"] == 1}
    if first != {(i, r) for i in range(294) for r in [1, 2]}:
        raise ValueError("Missing fixed first pages")
    if paired["requests_attempted"] != len(paired["pairs"]) * 2:
        raise ValueError("Incomplete pairs")
    cohorts = []
    for side in ["before", "after"]:
        for scope in ["first", "later", "all"]:
            selected = [
                x[side]
                for x in paired["pairs"]
                if scope == "all" or (scope == "first") == (x["page"] == 1)
            ]
            cohorts.append(
                {
                    "engine": "native" if side == "before" else "hybrid",
                    "scope": scope,
                    **statistics(selected),
                }
            )
    paths = []
    for backend in ["paradedb", "native"]:
        rows = [x for x in paired["pairs"] if meta[x["case"]]["expected_backend"] == backend]
        paths.append(
            {
                "eligible_path": backend,
                "pairs": len(rows),
                "outcomes": dict(collections.Counter(x["outcome"] for x in rows)),
                "native": statistics([x["before"] for x in rows]),
                "hybrid": statistics([x["after"] for x in rows]),
            }
        )
    calibration = read("calibration.json")
    cursors = read("cursors.json")
    if not calibration["complete"] or calibration["cases"] != 294 or calibration["mismatches"]:
        raise ValueError("Calibration incomplete or mismatched")
    if [x["case"] for x in calibration["observations"]] != list(range(294)):
        raise ValueError("Calibration order changed")
    selected_ids = [x["case"] for x in calibration["observations"] if x["outcome"] == "match"]
    if len(selected_ids) != calibration["matching_cases"]:
        raise ValueError("Load subset mapping incomplete")
    if (
        not cursors["complete"]
        or cursors["cases"] != 294
        or cursors["mismatches"]
        or not cursors["continuations"]
        or cursors["binding_checks"] != cursors["binding_rejected"]
    ):
        raise ValueError("Cursor verification failed")
    loads = []
    for name in ["load-native", "load-candidate", "load-candidate-repeat", "load-native-repeat"]:
        r = read(name + ".json")
        if (
            r["budget_exhausted"]
            or r["mode"] != "fixed_cohort"
            or r["fixed_rounds"] != 2
            or r["cases"] != calibration["matching_cases"]
            or set(r["closed_loop"]) != {"1", "4"}
        ):
            raise ValueError("Invalid fixed load population")
        for stage, frequency in [
            (r["initial_summary"], 1),
            *((x, 2) for x in r["closed_loop"].values()),
        ]:
            if (
                stage["requests"] != r["cases"] * frequency
                or len(stage["per_case"]) != r["cases"]
                or any(x["requests"] != frequency for x in stage["per_case"].values())
                or stage["outcomes"].get("hash_mismatch", 0)
            ):
                raise ValueError("Incomplete or mismatched load frequency")

        def compact(stage):
            result = {k: v for k, v in stage.items() if k != "per_case"}
            counts = collections.Counter()
            for index, values in stage["per_case"].items():
                backend = meta[selected_ids[int(index)]]["expected_backend"]
                counts[backend] += values["errors"]
            result["errors_by_hybrid_eligible_path"] = dict(counts)
            return result

        loads.append(
            {
                "name": name,
                "cases": r["cases"],
                "passed": r["passed"],
                "initial": compact(r["initial_summary"]),
                "closed_loop": {k: compact(v) for k, v in r["closed_loop"].items()},
            }
        )
    selection = read("cold-case-selection.json")
    ordinals = [8, 10, 55, 58, 89, 98, 112, 138, 165, 167, 209, 212]
    if selection["ordinals"] != ordinals or not selection["selected_before_new_timing"]:
        raise ValueError("Cold regression selection changed")
    labels = ["native-a", "candidate-a", "candidate-b", "native-b"] + [
        f"case-{i}-{side}" for i in ordinals for side in ["native", "candidate"]
    ]
    cache = []
    for label in labels:
        for data in ["source", "parade"]:
            r = read(f"cache-{label}-{data}.json")
            if (
                not r["eviction_requested"]
                or r["host_cache_flush"]
                or not r["at_most_one_mib_resident"]
                or r["resident_bytes_after"] > 1 << 20
                or r["resident_bytes_after"] != r["after"]["resident_pages"] * r["page_size"]
                or r["before"]["bytes"] != r["after"]["bytes"]
                or r["before"]["pages"] != r["after"]["pages"]
            ):
                raise ValueError("Unproved cold residency")
            cache.append({"run": label, "data": data, **r})
    cold_runs = []
    for label in labels:
        r = read(f"cold-{label}-report.json")
        expected = 294 if label in labels[:4] else 1
        if (
            not r["complete"]
            or r["cases"] != 294
            or r["requests"] != expected
            or len(r["observations"]) != expected
        ):
            raise ValueError("Incomplete cold run")
        if expected == 294 and {x["case"] for x in r["observations"]} != set(range(294)):
            raise ValueError("Missing cold batch cases")
        rows = [
            {
                "milliseconds": x["milliseconds"],
                "outcome": "ok" if x["success"] else "error",
                "code": x.get("code") or "transport_or_content_error",
            }
            for x in r["observations"]
        ]
        cold_runs.append(
            {"name": label, "side": r["side"], "reverse": r["reverse"], **statistics(rows)}
        )
    cold_pairs = []
    for label in ["pair-a", "pair-b", *(f"case-{i}-pair" for i in ordinals)]:
        r = read(f"cold-{label}.json")
        expected = 294 if label.startswith("pair-") else 1
        if (
            not r["complete"]
            or r["cases"] != 294
            or r["requests_per_backend"] != expected
            or len(r["pairs"]) != expected
            or r["mismatches"]
        ):
            raise ValueError("Incomplete or mismatched cold pair")
        cold_pairs.append({"name": label, **r})
    before, after = read("health-before.json"), read("health-after.json")
    completion = read("completion.json")
    if (
        before != after
        or any(x["health"] != "healthy" or x["status"] != "running" for x in after.values())
        or not completion["complete"]
        or completion["public_changed"]
        or not completion["private_services_stopped"]
        or completion["host_cache_flush"]
    ):
        raise ValueError("Public health changed or private cleanup incomplete")
    original = read("original-source-after.json")
    final_state = read("final-state.json")
    if (
        original["serving"]
        != {
            "visible_editions": 5,
            "visible_entries": 248951107,
            "hidden_rollback_visible": 0,
            "can_write_entries": False,
        }
        or not original["rollback"]["rollback_exists"]
        or original["rollback"]["rollback_entries"] != 59886841
        or final_state["private_running"] != 0
        or not final_state["original_source_policy_verified"]
    ):
        raise ValueError("Original source policy or final private state changed")
    return {
        "schema_version": 1,
        "method": {
            "cases": 294,
            "full_paired_rounds": 2,
            "max_pages": paired["max_pages"],
            "load_matching_cases": calibration["matching_cases"],
            "load_excluded_cases": 294 - calibration["matching_cases"],
            "source_copy": True,
            "source_limits": {
                "cpus": 2,
                "memory_gib": 6,
                "work_mem_mb": 8,
                "max_parallel_workers_per_gather": 1,
            },
            "additional_derived_limits": {"cpus": 2, "memory_gib": 6},
            "cold_start_batches": 4,
            "independently_reset_cases_per_backend": 12,
            "hardware_caches_controlled": False,
            "global_cache_flush": False,
        },
        "verification": verification,
        "metadata": metadata,
        "workload_identity": identity,
        "paired_outcomes": paired["outcomes"],
        "cohorts": cohorts,
        "paths": paths,
        "calibration": calibration,
        "cursors": cursors,
        "load_runs": loads,
        "cache_evictions": cache,
        "cold_runs": cold_runs,
        "cold_pairs": cold_pairs,
        "source_copy": read("source-copy.json"),
        "cache_unit": read("cache-unit.json"),
        "health_before": before,
        "health_after": after,
        "completion": completion,
        "original_source_after": original,
        "final_state": final_state,
    }


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("directory", type=Path)
    p.add_argument("--output", required=True, type=Path)
    args = p.parse_args()
    result = summarize(args.directory)
    args.output.write_text(json.dumps(result, indent=2) + "\n")
    for row in result["cohorts"]:
        print(row)


if __name__ == "__main__":
    main()
