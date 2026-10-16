# Contributing

Contributions to format research, edition support, tests, documentation and the reader are
welcome. Please describe the concrete problem and the evidence behind a change.

## Development setup

Use Python 3.10 or later:

```sh
python -m venv .venv
source .venv/bin/activate  # Windows PowerShell: .venv\Scripts\Activate.ps1
python -m pip install -e '.[dev]'
python -m unittest discover -s tests -v
ruff check .
ruff format --check .
python -m build
```

The default tests require no disc, database, credentials or network access. They generate a
tiny fictional database in a temporary directory. CI exercises Python 3.10 and 3.14 on Linux,
macOS and Windows. The optional development tools are separate from runtime dependencies.

## Working with a source disc

Keep source material under `private/`, `data/` or `extracted/`; those directories are ignored.
To run the local regression tests against the SE 4.0 disc:

```sh
# macOS/Linux:
UKINFO_TEST_DATA=extracted python -m unittest discover -s tests -v
# PowerShell: $env:UKINFO_TEST_DATA='extracted'; python -m unittest discover -s tests -v
ukinfo-validate --data extracted --output validation.json
```

For the earlier release, use `UKINFO_TEST_DATA_78=private/pcf78` with the same test command,
and `ukinfo-validate --data private/pcf78 --output validation.json`.

For the experimental 2004 and 2005 sources, follow the
[extraction and inspection instructions](docs/formats/modern-se.md). Use `ukinfo-inspect`
to reproduce aggregate structural reports. Keep these profiles outside the supported
query registry until joins and result selection have also been validated.

Only publish a validation report after checking that it contains metadata, hashes and counts.
Do not contribute directory records, real personal information, original images or programs,
disassemblies, extracted strings, CSV/JSONL exports, or binary database fixtures. Use invented
examples to isolate new encoding behavior. Do not paste records into public issues or CI logs.
The ignore rules are a convenience; review staged files before committing.

## Adding editions

Read [the edition support guide](docs/adding-editions.md) before changing the registry.
Keep supported and investigated releases distinct. A release needs an exact source identity,
format evidence, synthetic regression coverage and a complete local validation report.
Preserve the tests and validation results for existing editions.

## Pull requests

Include a concise explanation of the resulting behavior, relevant tests and any remaining
uncertainties. Decoder fixes should cite the format section or reader routine they implement.
Tests should cover behavior that could regress, especially overflow, repeat boundaries,
string references and joins between record levels.

The new code and documentation use the MIT license. By contributing, you agree that your
contribution can be distributed under that license. The original software and data retain
their own rights. Be respectful and keep technical disagreement focused on evidence.
