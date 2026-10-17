"""Invented entries for a safe, offline first-run demonstration."""

import hashlib
import json
from types import SimpleNamespace


def records():
    for ordinal in range(125):
        yield dict(
            entry_id=f"invented:{ordinal}",
            entry_type="electoral",
            surname="Example",
            firstName=f"Person {ordinal:03d}",
            houseno=str(ordinal + 1),
            street="Invented Street",
            town="Fictional Town",
            postcode="ZZ1 1ZZ",
            source_warning="synthetic_demo_not_a_real_person" if ordinal == 0 else "",
            building=None,
        )
    yield dict(
        entry_id="invented:business",
        entry_type="business",
        organisation="Aster Archive Supplies",
        street="Invented Street",
        postcode="ZZ1 1ZZ",
        industry="Synthetic archive supplies",
    )


def database():
    rows = list(records())
    digest = hashlib.sha256(json.dumps(rows, sort_keys=True).encode()).hexdigest()
    return SimpleNamespace(
        format="demo",
        edition=SimpleNamespace(
            id="synthetic-demo",
            title="Synthetic demo — invented entries",
            verify=lambda _: {"invented-fixture": digest},
        ),
        records=lambda: iter(rows),
    )
