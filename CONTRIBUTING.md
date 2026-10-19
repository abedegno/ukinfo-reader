# Contributing

Contributions to format research, edition support, tests, documentation and the reader are
welcome. Please describe the concrete problem and the evidence behind a change.

## Find the right component

- `src/ukinfo_reader/` and `tests/`: Python decoding, validation and streaming ingestion.
- `server/`: Go API, PostgreSQL catalogue and embedded web UI.
- `desktop/`: Electron shell, native lifecycle and browser tests.
- `docs/formats/` and `docs/validation/`: format evidence and aggregate source reports.
- `tools/` and `.github/workflows/`: build, release and verification tooling.

Start with the [documentation index](docs/README.md). Report ordinary bugs through
the [issue chooser](https://github.com/abedegno/ukinfo-reader/issues/new/choose); use
[SECURITY.md](SECURITY.md) for vulnerabilities.

## Python development setup

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

For the 2004 and 2005 residential sources, follow the
[extraction and inspection instructions](docs/formats/modern-se.md). Use `ukinfo-inspect`
to reproduce aggregate structural reports, or `ukinfo-validate` with the complete query
profile to check joins and entry selection. Do not confuse source occurrences with
distinct people, or remove electoral names when applying telephone deletions.

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

## Search application

For the Go service, run `go test ./...` and `go vet ./...` from `server/`.
For desktop unit tests, run `npm ci` and `npm test` from `desktop/`. Database/browser
integration and native packaging need the additional setup below.

See [the application guide](docs/search-app.md#verification) for Go/PostgreSQL integration,
desktop lifecycle and browser test commands. Use invented fixtures for committed tests and
screenshots. Keep imports, PostgreSQL clusters, bundles and record-level test output ignored.
The hosted and desktop builds share the same SQL, API and embedded UI.

### Production policy checks

These checks use invented fixtures and never mount an imported catalogue:

```sh
python3 tools/test_monitor.py
python3 tools/check_serving_role.py
python3 tools/check_hosting_edge.py
```

The serving-role check uses Docker and a temporary PostgreSQL 18 container with no
network access or published ports; its data is in tmpfs and is removed afterward.
It checks fresh and existing roles, readable results, denied writes with read-only
transactions disabled, column grants, ownership, membership and PUBLIC permissions.
CI runs it on Linux. The edge check also needs Go and OpenSSL.
