# macOS signing and notarization

Ordinary **Desktop packages** builds remain ad-hoc signed previews. **Signed macOS
candidates** is a separate, manually triggered workflow. It builds Apple Silicon and
Intel packages from an existing `desktop-v` tag, requires the `desktop-signing`
GitHub environment and produces reviewable signed artifacts. It does **not** publish
a release or replace existing downloads automatically.

## Credentials to configure

In the repository's `desktop-signing` environment, add these secrets through GitHub's
settings. Do not put keys, certificates or passwords in the repo or chat.

| Secret | Value |
| --- | --- |
| `CSC_LINK` | Base64 content of a password-protected PKCS#12 export containing a **Developer ID Application** certificate and its private key (base64 data URLs are also accepted; filesystem paths and certificate-download URLs are not) |
| `CSC_KEY_PASSWORD` | Password for that PKCS#12 export |
| `APPLE_API_KEY` | Base64 content of the App Store Connect team API key `.p8` file |
| `APPLE_API_KEY_ID` | App Store Connect API key ID |
| `APPLE_API_ISSUER` | App Store Connect team API issuer ID |
| `APPLE_TEAM_ID` | Developer team ID; the signing identity is discovered and checked against this team |

These names and the API-key authentication scheme match Muesli's desktop-release
workflow. GitHub secret values are write-only: their presence in the Muesli repo does
not make them available here. Add the same credential material to this repository or
the `desktop-signing` environment through GitHub settings. No Apple ID or app-specific
password is needed. The API key may be reused only with authorization for that team.

Configure environment reviewers/restrictions appropriate to the repository. Dispatch
only a reviewed tag containing this workflow and its supporting tools (0.3.1 or later; `desktop-v0.3.0` still uses the previous Apple-ID helper).
The job checks that the tag matches the desktop package version. Missing credentials
fail before building a candidate; there is no unsigned fallback in this workflow.

## What the workflow does

1. Imports the certificate into a temporary runner keychain, discovers its valid
   Developer ID identity for the configured team, stores API-key notarization credentials there, and adds only that temporary keychain to the existing search list.
   Credential-bearing command arguments are not printed, including on subprocess failure.
2. Signs the standalone decoder **during PyInstaller staging**, so embedded Python
   libraries carry the same Developer ID. They cannot be signed correctly by merely
   replacing the outer one-file executable's signature later.
3. Uses pinned `@electron/osx-sign` to sign the app and nested native code with hardened
   runtime. Electron receives the JIT entitlement; runtime executables receive the
   empty native entitlement profile. No app-sandbox, get-task-allow, unsigned executable
   memory or blanket library-validation exceptions are requested.
4. Runs the packaged-app import/search/research/management/shutdown checks. These
   are clean runner/profile checks; they do not simulate every customer OS installation.
5. Submits the app ZIP with Apple's `notarytool`, requires **Accepted**, staples and
   validates the ticket, verifies nested signatures, and checks Gatekeeper assessment.
6. Creates source-data-free ZIPs, SHA-256 files and manifests identifying the result
   as Developer ID signed, notarized and stapled. The manifest checks the actual app
   signature/ticket, rather than inferring success from an environment variable.
7. Deletes the temporary certificate/API-key files and keychain and restores the runner's
   original keychain search list. Candidate artifacts expire after seven days.

The signing path needs a real certificate/Apple service run before it can be described
as validated. Without configured credentials, only ordinary ad-hoc builds have been
executed. A signed candidate should also be downloaded and opened on clean customer
machines before promoting it to a public release. Linux builds remain unsigned.

## Attach candidates to a draft

After **Desktop packages** succeeds for a tag and **Signed macOS candidates** succeeds
for the same commit, dispatch **Attach signed desktop candidates to draft** with:

- `tag`: the existing `desktop-v` tag.
- `package_run`: the successful tag-triggered Desktop packages run ID.
- `signing_run`: the successful Signed macOS candidates run ID.

The workflow checks both run identities, events and commits, requires an existing
**draft**, downloads Linux and both signed Mac packages, and verifies all checksums,
versions and source-data exclusion flags. Both Mac manifests must record Developer ID,
notarization and stapling. It replaces only the six Mac files, updates the signing
paragraph in the draft notes, and checks the size and SHA-256 digest of all nine
uploaded files. It never publishes a release. Failed uploads can leave a partially
updated draft; rerun only after inspecting the failure. Ordinary package builds must
finish before promotion so they cannot overwrite the signed assets afterwards.

## Primary references

- [Electron code signing](https://www.electronjs.org/docs/latest/tutorial/code-signing)
- [Electron osx-sign options](https://packages.electronjs.org/osx-sign/v2.6.0/interfaces/SignOptions.html)
- [PyInstaller macOS signing and embedded binaries](https://pyinstaller.org/en/stable/feature-notes.html#macos-binary-code-signing)
- [Apple custom notarization workflow](https://developer.apple.com/documentation/security/customizing-the-notarization-workflow)
