"use strict";
const fs = require("node:fs");
const path = require("node:path");
function shouldSign(file) {
  // Licence trees retain upstream directory names, including Tcl.framework.
  // They are resources, not executable bundles; still inspect any files for Mach-O.
  const notice = file.includes(
    `${path.sep}runtime${path.sep}notices${path.sep}`,
  );
  if ([".app", ".framework"].includes(path.extname(file)) && !notice)
    return true;
  if (notice && !fs.statSync(file).isFile()) return false;
  let descriptor;
  try {
    descriptor = fs.openSync(file, "r");
    const header = Buffer.alloc(4);
    if (fs.readSync(descriptor, header, 0, 4, 0) !== 4) return false;
    return new Set([
      0xfeedface, 0xcefaedfe, 0xfeedfacf, 0xcffaedfe, 0xcafebabe, 0xbebafeca,
      0xcafebabf, 0xbfbafeca,
    ]).has(header.readUInt32BE(0));
  } finally {
    if (descriptor !== undefined) fs.closeSync(descriptor);
  }
}
async function signForDistribution(app, identity, keychain) {
  if (
    process.platform !== "darwin" ||
    !/^[a-fA-F0-9]{40}$/.test(identity || "") ||
    !keychain
  )
    throw new Error(
      "Developer ID signing needs macOS, a certificate SHA-1 fingerprint and a temporary keychain.",
    );
  const { sign } = await import("@electron/osx-sign");
  await sign({
    app,
    identity,
    keychain,
    platform: "darwin",
    type: "distribution",
    version: require("./package.json").devDependencies.electron,
    preAutoEntitlements: false,
    ignore: [(file) => !shouldSign(file)],
    optionsForFile: (file) => ({
      hardenedRuntime: true,
      entitlements: path.join(
        __dirname,
        "entitlements",
        file.includes(`${path.sep}runtime${path.sep}`)
          ? "native.plist"
          : "electron.plist",
      ),
    }),
  });
  // The one-file decoder must already have its embedded Python libraries signed
  // by PyInstaller during staging. Signing only its outer file cannot do this.
  await fs.promises.access(
    path.join(app, "Contents/Resources/runtime/ukinfo-import"),
  );
}
module.exports = { signForDistribution, shouldSign };
