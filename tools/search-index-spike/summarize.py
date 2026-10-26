#!/usr/bin/env python3
"""Summarize fixed first-page cohorts; preserve failed requests in percentiles."""

import argparse
import collections
import json
import math
from pathlib import Path


def quantile(values, fraction):
    return round(sorted(values)[math.ceil(len(values) * fraction) - 1], 3)


def summarize(report):
    groups = collections.defaultdict(list)
    cases = collections.defaultdict(list)
    for sample in report["Samples"]:
        if sample["Page"] == 1:
            groups[(sample["Engine"], sample["Clients"])].append(sample)
            cases[(sample["Engine"], sample["Clients"], sample["Case"])].append(sample)

    def stats(samples):
        values = [s["MS"] for s in samples]
        return dict(
            requests=len(values),
            errors=sum(bool(s.get("Error")) for s in samples),
            p50_ms=quantile(values, 0.5),
            p95_ms=quantile(values, 0.95),
            p99_ms=quantile(values, 0.99),
        )

    settings = dict(report["Settings"])
    settings.setdefault("plan_cache_mode", "auto")
    return dict(
        rows=report["Rows"],
        version=report["Version"],
        extensions=report["Extensions"],
        settings=settings,
        builds=report["Builds"],
        verified_pages=report["VerifiedPages"],
        complete_walks=report["CompleteWalks"],
        timed_requests=len(report["Samples"]),
        timed_errors=report["Errors"],
        first_pages=[dict(engine=e, clients=c, **stats(v)) for (e, c), v in sorted(groups.items())],
        per_case_first_pages=[
            dict(engine=e, clients=c, case=k, **stats(v)) for (e, c, k), v in sorted(cases.items())
        ],
        plans=report["Plans"],
    )


if __name__ == "__main__":
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("reports", nargs="+", type=Path)
    p.add_argument("--output", required=True, type=Path)
    a = p.parse_args()
    reports = [summarize(json.loads(path.read_text())) for path in a.reports]
    a.output.write_text(json.dumps({"runs": reports}, indent=2) + "\n")
    for r in reports:
        print(r["settings"].get("plan_cache_mode", "auto (initial client default)"))
        for row in r["first_pages"]:
            print(row)
