# Hosted release workflow

Hosted UK-Info uses reviewed application code and configuration as code in
[homelab](https://github.com/abedegno/homelab), under
`docker/ukinfo/hosted-release/`. This is separate from the
[desktop packaging/signing workflow](desktop-releases.md); desktop releases remain drafts.
No NAS credential or private directory fixture is supplied to GitHub runners.

## Release contract

A versioned JSON manifest selects an immutable `hosted-YYYY-MM-DD.N` application
tag, its full 40-character commit, application/coordinator image tags, the accepted
runtime-source digest, numeric benchmark evidence and the frozen NAS acceptance
client's code digest. `releases/current.json` selects the desired manifest; live
status remains authoritative for what is actually deployed. Old manifests and
failed measurements remain available.

Before promotion, the operator command requires successful latest main-branch
workflow runs at the exact application commit (tests, search, observability and
desktop packaging) and exact homelab commit (regression/release policy). A green PR,
older SHA or older successful rerun does not satisfy that gate. Homelab release
configuration must be committed on main with no local changes in its deployment
scope. Unrelated homelab work is outside that scope. There is no skip-CI switch.

The numeric acceptance must cover the frozen 294-case comparison, separate twelve
expanded cases, 64 native references, fixed load gates and all ten source-cold
probes. The three-second cold gate is unchanged. See
[the latest measured implementation and retained failures](research/grouped-search-2026-10-27.md).
Source provenance does not make these short tests a public-capacity guarantee.

## Operator commands

Run from a trusted homelab checkout with Python 3.10+, Git, authenticated `gh` and
Docker Compose. Portainer settings stay in its local ignored `.env` or process
environment. Build contexts contain application source only; images build on the
NAS with two CPUs/two GiB, without local image tar files or database copies.

```sh
python3 docker/ukinfo/hosted-release/release.py validate  # offline contract check
python3 docker/ukinfo/hosted-release/release.py status    # read-only actual state
python3 docker/ukinfo/hosted-release/release.py plan      # exact source/CI gates
python3 docker/ukinfo/hosted-release/release.py apply     # build, stage, verify
```

An explicit tracked manifest path can replace the default. Prepare a new manifest
and accepted benchmark evidence in a reviewed PR; do not retag an existing image or
overwrite a previous release's evidence. Merge application/configuration changes
and let their exact main checks finish before applying. If unrelated homelab
changes did not trigger UK-Info policy checks, manually dispatch both policy
workflows on the selected main SHA. The workflow-dispatch job validates manifests
and rollback tests; it does not receive a production credential or deploy the NAS.

The deployment command holds a NAS-wide lease, waits for an idle fixture, checks
image provenance and saves the previous stack privately. It permits only app,
monitor and coordinator image changes plus the search-backend setting; database,
network, volume, quota and other infrastructure differences need separate review.
It stages native then hybrid mode, checking twelve internal references each, then
all 64 native references and the separately labeled pre-change hybrid grouped
snapshot. Real searches stay on private Docker networks. Public smoke uses only
health, the UI and invented input. Serving database identities must stay unchanged.

## Recovery and verification

Any failure after an attempted stack update restores the prior stack/environment
and retained image identities, then verifies readiness. A concurrent external
stack edit is not overwritten; failed recovery is reported explicitly. Previous
configuration may contain credentials: local state is ignored, directories are
0700 and files 0600. Private hashes and query fixtures remain on the NAS.

```sh
python3 docker/ukinfo/hosted-release/release.py verify       # deployed references
python3 docker/ukinfo/hosted-release/release.py native       # same images, native routing
python3 docker/ukinfo/hosted-release/release.py rollback --run RUN_ID
```

Recovery commands reuse already-deployed/retained code and remain available during
GitHub outages; they do not build or promote unreviewed images. They retain public
controls and serving data. Keep previous images until their recovery window ends.
Never delete a data volume or flush global/serving-target caches to recover an app.
A crashed operator can leave the NAS release lease: inspect its run label and
confirm no operator/build remains before removing only that stopped lease record.

As of 27 October 2026, homelab's GitHub jobs cannot start because of account
billing/spending limits. Release planning reports this missing/failed gate and
production promotion stays blocked. Resolve billing and pass/merge configuration
checks; a privileged NAS GitHub runner or disabled checks is not the workaround.
