#!/usr/bin/env python3
"""Publish aggregate-only fixed-cohort comparisons from operator reports."""

import argparse
import collections
import json
import math
from pathlib import Path


def stats(values):
    if not values:
        return None
    times = sorted(x["total_ms"] for x in values)

    def p(f):
        return round(times[math.ceil(len(times) * f) - 1], 3)

    return dict(
        requests=len(values),
        errors=sum(x["outcome"] != "ok" for x in values),
        error_codes=dict(
            collections.Counter(
                x.get("error_code", "unclassified") for x in values if x["outcome"] != "ok"
            )
        ),
        reference_matches=sum(x["reference"] == "match" for x in values),
        reference_unavailable=sum(x["reference"] == "unavailable" for x in values),
        p50_ms=p(0.5),
        p95_ms=p(0.95),
        p99_ms=p(0.99),
        max_ms=round(max(times), 3),
        query_p95_ms=round(
            sorted(x["query_ms"] for x in values)[math.ceil(len(values) * 0.95) - 1], 3
        ),
        records_p95_ms=round(
            sorted(x["records_ms"] for x in values)[math.ceil(len(values) * 0.95) - 1], 3
        ),
    )


def summarize(report):
    if not report["complete"] or report["mismatches"]:
        raise ValueError(
            "Do not publish an incomplete or mismatched comparison as accepted evidence"
        )
    obs = report["observations"]
    if not obs:
        raise ValueError("Empty report")
    cases = {o["case"] for o in obs}
    first = {
        e: {(x["case"], x["round"]) for x in obs if x["engine"] == e and x["page"] == 1}
        for e in ["native", "parade"]
    }
    if first["native"] != first["parade"] or len(first["native"]) != len(cases) * 2:
        raise ValueError("Fixed two-round first-page cohort is incomplete")
    result = dict(
        plan_mode=obs[0]["plan_mode"],
        clients=obs[0]["clients"],
        cases=len(cases),
        wall_seconds=round(report["wall_seconds"], 3),
        failures=report["failures"],
        mismatches=0,
        reference_cached=report.get("reference_cached", False),
        independent_reference_pages=sum(r["outcome"] == "ok" for r in report.get("references", [])),
        failed_source_reference_pages=sum(
            r["outcome"] != "ok" for r in report.get("references", [])
        ),
        cohorts=[],
        families=[],
    )
    for e in ["native", "parade"]:
        for scope in ["first", "later", "all"]:
            xs = [
                o
                for o in obs
                if o["engine"] == e and (scope == "all" or (scope == "first") == (o["page"] == 1))
            ]
            result["cohorts"].append(dict(engine=e, scope=scope, **(stats(xs) or {})))
        for f in sorted({o["family"] for o in obs}):
            xs = [o for o in obs if o["engine"] == e and o["family"] == f and o["page"] == 1]
            if xs:
                result["families"].append(dict(engine=e, family=f, **stats(xs)))
    pairs = collections.defaultdict(dict)
    for o in obs:
        pairs[(o["case"], o["round"], o["page"])][o["engine"]] = o["outcome"]
    result["pairs"] = dict(
        mutual_success=sum(
            p.get("native") == "ok" and p.get("parade") == "ok" for p in pairs.values()
        ),
        native_only_success=sum(
            p.get("native") == "ok" and p.get("parade") not in (None, "ok") for p in pairs.values()
        ),
        parade_only_success=sum(
            p.get("parade") == "ok" and p.get("native") not in (None, "ok") for p in pairs.values()
        ),
        both_failed=sum(
            p.get("native") not in (None, "ok") and p.get("parade") not in (None, "ok")
            for p in pairs.values()
        ),
        unpaired=sum(len(p) == 1 for p in pairs.values()),
    )
    return result


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("directory", type=Path)
    p.add_argument("--output", required=True, type=Path)
    a = p.parse_args()
    runs = []
    for name in ["auto-c1", "custom-c1", "auto-c4", "custom-c4"]:
        runs.append(summarize(json.loads((a.directory / (name + ".json")).read_text())))
    if len({r["cases"] for r in runs}) != 1:
        raise ValueError("Different case cohorts")
    result = dict(
        schema_version=1,
        method=dict(
            dataset="se2003-dpcs0104",
            cases=runs[0]["cases"],
            rounds=2,
            max_pages=3,
            page_size=50,
            sql_timeout_ms=4000,
            request_timeout_ms=5000,
            database_cpus=2,
            database_memory_bytes=6 * 1024**3,
            client_cpus=1,
            client_memory_bytes=512 * 1024**2,
            shared_database=True,
            independent_engine_throughput=False,
            cache_flushed=False,
            includes_original_record_retrieval=True,
            includes_http=False,
            plans_analyzed=True,
            preliminary_plans_analyzed=False,
            corrected_nonexecuting_plans_analyzed=False,
        ),
        build=json.loads((a.directory / "build-metadata.json").read_text()),
        environment=json.loads((a.directory / "environment.json").read_text()),
        runs=runs,
        plans=json.loads((a.directory / "plans.json").read_text()),
        preliminary=summarize(json.loads((a.directory / "preliminary-auto-c1.json").read_text())),
        preliminary_plans=json.loads((a.directory / "preliminary-plans.json").read_text()),
        corrected_nonexecuting_plans=json.loads((a.directory / "corrected-plans.json").read_text()),
    )
    a.output.write_text(json.dumps(result, indent=2) + "\n")
    for r in runs:
        print(r["plan_mode"], r["clients"], "clients", r["cases"], "cases")
        for row in r["cohorts"]:
            if row["scope"] == "first":
                print(row)


if __name__ == "__main__":
    main()
