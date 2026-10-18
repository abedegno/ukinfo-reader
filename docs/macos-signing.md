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
| `MAC_CERT_P12_BASE64` | Base64 export of a **Developer ID Application** signing certificate and its private key, in password-protected PKCS#12 format |
| `MAC_CERT_PASSWORD` | Password for that PKCS#12 export |
| `MAC_CODESIGN_IDENTITY` | The certificate's 40-character SHA-1 fingerprint (not a certificate filename) |
| `APPLE_ID` | Apple ID authorized to notarize for the team |
| `APPLE_TEAM_ID` | Developer team ID |
| `APPLE_APP_PASSWORD` | Apple app-specific password for notarization |

Configure environment reviewers/restrictions appropriate to the repository. Dispatch
only a reviewed tag containing this workflow and its supporting tools (0.3.0 or later).
The job checks that the tag matches the desktop package version. Missing credentials
fail before building a candidate; there is no unsigned fallback in this workflow.

## What the workflow does

1. Imports the certificate into a temporary runner keychain, stores notarization
   credentials there, and adds only that temporary keychain to the existing search list.
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
7. Deletes the temporary certificate file and keychain and restores the runner's
   original keychain search list. Candidate artifacts expire after seven days.

The signing path needs a real certificate/Apple service run before it can be described
as validated. Without configured credentials, only ordinary ad-hoc builds have been
executed. A signed candidate should also be downloaded and opened on clean customer
machines before promoting it to a public release. Linux builds remain unsigned.

## Primary references

- [Electron code signing](https://www.electronjs.org/docs/latest/tutorial/code-signing)
- [Electron osx-sign options](https://packages.electronjs.org/osx-sign/v2.6.0/interfaces/SignOptions.html)
- [PyInstaller macOS signing and embedded binaries](https://pyinstaller.org/en/stable/feature-notes.html#macos-binary-code-signing)
- [Apple custom notarization workflow](https://developer.apple.com/documentation/security/customizing-the-notarization-workflow)
