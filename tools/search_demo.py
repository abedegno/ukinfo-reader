"""Emit an invented catalogue for UI development; contains no directory records."""

import hashlib
import json
import sys


def main():
    rows = []
    for ordinal in range(125):
        rows.append(
            dict(
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
        )
    rows.append(
        dict(
            entry_id="invented:business",
            entry_type="business",
            organisation="Aster Archive Supplies",
            street="Invented Street",
            postcode="ZZ1 1ZZ",
            industry="Synthetic archive supplies",
        )
    )
    payload = json.dumps(rows, sort_keys=True).encode()
    frames = [
        dict(
            type="manifest",
            protocol=1,
            dataset="synthetic-demo",
            edition="synthetic-demo",
            title="Synthetic demo — invented entries",
            reader_version="synthetic-1",
            sha256={"invented-fixture": hashlib.sha256(payload).hexdigest()},
            sample=True,
            expected=len(rows),
        )
    ]
    frames.extend(dict(type="record", source_id=r["entry_id"], record=r) for r in rows)
    frames.append(dict(type="end", count=len(rows)))
    for frame in frames:
        print(json.dumps(frame, separators=(",", ":")), file=sys.stdout)


if __name__ == "__main__":
    main()
