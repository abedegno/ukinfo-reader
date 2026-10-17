"""Emit invented catalogue frames; this script also works from a plain checkout."""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from ukinfo_reader.demo import database  # noqa: E402
from ukinfo_reader.search_export import emit  # noqa: E402

if __name__ == "__main__":
    emit(database(), None, sys.stdout)
