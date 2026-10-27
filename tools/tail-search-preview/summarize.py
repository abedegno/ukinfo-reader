"""Summarize fixed allowlisted numeric reports; never open query or golden files."""

import argparse
import json
from pathlib import Path


def build(root, prefix):
    def read(name):
        return json.loads((root / f"{prefix}-{name}.json").read_text())

    def comparison(name, cases):
        report = read(name)
        pairs = report["pairs"]
        positions = {(p["case"], p["round"], p["page"]) for p in pairs}
        assert report["cases"] == cases and report["rounds"] == 2
        assert report["max_pages"] == 10 and not report["budget_exhausted"]
        assert len(positions) == len(pairs)
        assert {(i, r) for i, r, p in positions if p == 1} == {
            (i, r) for i in range(cases) for r in (1, 2)
        }
        assert all(i in range(cases) and r in (1, 2) and p in range(1, 11) for i, r, p in positions)
        assert all(p == 1 or (i, r, p - 1) in positions for i, r, p in positions)
        allowed = {
            "both_ok_same",
            "both_ok_mismatch",
            "before_only_ok",
            "after_only_ok",
            "both_failed",
        }
        assert set(report["outcomes"]) <= allowed
        return {
            "cases": cases,
            "rounds": 2,
            "max_pages": 10,
            "pairs": len(pairs),
            "outcomes": report["outcomes"],
            "candidate_errors": sum(p["after"]["status"] != 200 for p in pairs),
            "baseline_errors": sum(p["before"]["status"] != 200 for p in pairs),
            "mismatches": report["outcomes"].get("both_ok_mismatch", 0),
        }

    references = {}
    for name, expected in [
        ("business-reference", 4),
        ("cold-reference", 16),
        ("exact-reference", 4),
        ("coverage-reference-0", 12),
        ("coverage-reference-1", 16),
        ("coverage-reference-2", 12),
    ]:
        report = read(name)
        assert report["requests"] == expected and report["errors"] == report["mismatches"] == 0
        references[name] = {k: report[k] for k in ("requests", "errors", "mismatches")}
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
        report = read(name)
        assert (
            report["cases"] == 292
            and report["requests_attempted"] == 876
            and not report["budget_exhausted"]
        )
        stats = report["closed_loop"][str(concurrency)]
        assert stats["requests"] == 584 and len(stats["per_case"]) == 292
        assert all(v["requests"] == 2 for v in stats["per_case"].values())
        assert not stats["outcomes"].get("hash_mismatch", 0)
        loads[name] = {
            k: stats[k]
            for k in ("requests", "errors", "median_ms", "p95_ms", "requests_per_second")
        }
        loads[name]["initial_errors"] = report["initial_summary"]["errors"]
    rates = {}
    for side, repeat in [
        ("baseline", False),
        ("candidate", False),
        ("candidate", True),
        ("baseline", True),
    ]:
        name = f"rate-{side}" + ("-repeat" if repeat else "")
        report = read(name)
        phase = report["scheduled_arrivals"]
        assert not report["budget_exhausted"] and phase["scheduled_requests"] == 584
        assert phase["stats"]["requests"] == 584
        rates[name] = {
            k: phase["stats"][k]
            for k in ("requests", "errors", "median_ms", "p95_ms", "requests_per_second")
        }
        rates[name].update({k: phase[k] for k in ("load_generator_drops", "budget_skips")})
        rates[name]["initial_errors"] = report["initial_summary"]["errors"]
    cold = []
    for coverage, cases in [(False, [56, 166, 9, 95, 232, 216]), (True, [0, 7, 9])]:
        family = "coverage-" if coverage else ""
        for case in cases:
            cache = read(f"{family}cache-{case}")
            assert cache["at_most_one_mib_resident"] and not cache["host_cache_flush"]
            report = read(f"{family}cold-{case}-report")
            gate = read(f"{family}source-check-{case}")
            assert report["complete"] and report["requests"] == 1 and not report["errors"]
            assert gate["requests"] == 1 and gate["errors"] == gate["mismatches"] == 0
            cold.append(
                {
                    "coverage": coverage,
                    **{k: report["observations"][0][k] for k in ("case", "status", "milliseconds")},
                }
            )
    cache = read("grouped-cache-91")
    report = read("grouped-cold-91-report")
    gate = read("grouped-source-check-91")
    assert cache["at_most_one_mib_resident"] and not cache["host_cache_flush"]
    assert report["complete"] and report["requests"] == 1 and not report["errors"]
    assert gate["complete"] and gate["requests_per_backend"] == 1
    assert gate["outcomes"] == {"both_ok_same": 1} and gate["mismatches"] == 0
    assert report["observations"][0]["case"] == 91
    cold.append(
        {
            "coverage": False,
            "grouped": True,
            **{k: report["observations"][0][k] for k in ("case", "status", "milliseconds")},
        }
    )
    completion = json.loads((root / "completion.json").read_text())
    assert (
        completion["source_and_target_unchanged"]
        and completion["public_unchanged"]
        and completion["private_services_stopped"]
    )
    result = json.loads((root / "result.json").read_text())
    original, coverage = comparison("compare", 294), comparison("coverage-compare", 12)
    passed = result["complete"] and result["candidate_failures"] == 0
    passed = passed and all(
        not r[k] for r in (original, coverage) for k in ("candidate_errors", "mismatches")
    )
    passed = passed and all(
        not s["errors"] and not s["initial_errors"] for n, s in loads.items() if "candidate" in n
    )
    passed = passed and all(
        s["p95_ms"] <= (500 if "-c1" in n else 1000) for n, s in loads.items() if "candidate" in n
    )
    passed = passed and all(s["milliseconds"] <= 3000 for s in cold)
    passed = passed and all(
        not s["errors"]
        and not s["initial_errors"]
        and not s["load_generator_drops"]
        and not s["budget_skips"]
        for n, s in rates.items()
        if "candidate" in n
    )
    return {
        "schema_version": 1,
        "passed": bool(passed),
        "baseline": "hosted-2026-10-27.1",
        "source_cache": "independently reset for ten cold probes",
        "target_cache": "shared/uncontrolled",
        "original": original,
        "coverage": coverage,
        "reference_checks": references,
        "grouped_reference": {
            "kind": "pre-change hybrid first-page snapshot",
            "baseline": "hosted-2026-10-27.1",
            "checks": 1,
            "errors": 0,
            "mismatches": 0,
            "full_native_reference_complete": False,
            "full_native_offline_deadline_seconds": 600,
        },
        "latency_budgets_ms": {"source_cold": 3000, "c1_p95": 500, "c4_p95": 1000},
        "closed_loop": loads,
        "scheduled_arrivals": rates,
        "source_cold": cold,
        "lifecycle": {
            k: completion[k]
            for k in ("source_and_target_unchanged", "public_unchanged", "private_services_stopped")
        },
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--final", required=True, type=Path)
    parser.add_argument("--prefix", required=True)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    report = build(args.final, args.prefix)
    with args.output.open("x") as output:
        json.dump(report, output, indent=2)
        output.write("\n")
    if not report["passed"]:
        raise SystemExit("Candidate failed; aggregate retained")


if __name__ == "__main__":
    main()
