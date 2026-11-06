"""Explicit hosted selections; archival reader output remains unchanged."""

from collections import Counter
from dataclasses import dataclass

KINDS = ("electoral", "telephone", "business")


@dataclass(frozen=True)
class Selection:
    edition: str
    policy: str
    suffix: str
    source_counts: tuple[int, int, int]
    excluded_counts: tuple[int, int, int]

    @property
    def source_expected(self):
        return sum(self.source_counts)

    @property
    def excluded_expected(self):
        return sum(self.excluded_counts)

    @property
    def expected(self):
        return self.source_expected - self.excluded_expected

    @property
    def type_counts(self):
        return {k: n - e for k, n, e in zip(KINDS, self.source_counts, self.excluded_counts)}

    def manifest(self):
        return dict(
            policy=self.policy,
            source_expected=self.source_expected,
            excluded_expected=self.excluded_expected,
            expected=self.expected,
            type_counts=self.type_counts,
            source_field="source_co1",
        )

    def accept(self, row):
        kind, flag = row.get("entry_type"), row.get("source_co1")
        if kind not in KINDS:
            raise ValueError("Hosted selection requires a known occurrence type")
        if kind == "business":
            if flag is not None and flag != "" and flag is not False:
                raise ValueError("Unexpected CO1 value on a business occurrence")
            return True
        if type(flag) is not bool:
            raise ValueError("Hosted v14 selection requires a decoded boolean CO1 flag")
        return not flag

    def records(self, rows, seen, excluded):
        for row in rows:
            accepted = self.accept(row)
            seen[row["entry_type"]] += 1
            if accepted:
                yield row
            else:
                excluded[row["entry_type"]] += 1

    def verify_counts(self, seen, excluded):
        if seen != Counter(dict(zip(KINDS, self.source_counts))) or excluded != Counter(
            dict(zip(KINDS, self.excluded_counts))
        ):
            raise ValueError("Hosted source/CO1 selection counts differ; import rejected")


V14_CO1 = Selection(
    edition="v14-2008-03",
    policy="exclude-source-co1-v1",
    suffix=":co1-excluded",
    source_counts=(26963431, 10744063, 6092008),
    excluded_counts=(40694, 14175, 0),
)


def get_selection(edition, name):
    if name is None:
        return None
    if name != "exclude-source-co1" or edition != V14_CO1.edition:
        raise ValueError("The CO1 hosted selection applies only to the verified v14 source")
    return V14_CO1
