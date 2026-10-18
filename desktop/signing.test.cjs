"use strict";
const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs/promises"),
  os = require("node:os"),
  path = require("node:path");
const { shouldSign, signForDistribution } = require("./signing.cjs");
test("signing selects Mach-O/native code and bundles, leaving ordinary archive resources sealed by the outer app", async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), "ukinfo-sign-plan-"),
  );
  try {
    const binary = path.join(directory, "ukinfo-server");
    await fs.writeFile(binary, Buffer.from([0xcf, 0xfa, 0xed, 0xfe]));
    assert.equal(shouldSign(binary), true);
    const resource = path.join(directory, "app.asar");
    await fs.writeFile(resource, Buffer.from([0, 1, 0, 0, 4, 9]));
    assert.equal(shouldSign(resource), false);
    assert.equal(shouldSign(path.join(directory, "Example.app")), true);
    const framework = path.join(
      directory,
      "runtime",
      "notices",
      "CPython",
      "Tcl.framework",
    );
    await fs.mkdir(framework, { recursive: true });
    assert.equal(shouldSign(framework), false);
    const license = path.join(framework, "LICENSE");
    await fs.writeFile(license, "Redistribution licence");
    assert.equal(shouldSign(license), false);
    const native = path.join(framework, "native");
    await fs.copyFile(binary, native);
    assert.equal(shouldSign(native), true);
    await assert.rejects(
      signForDistribution("unused.app", "not-a-fingerprint", "unused"),
      /fingerprint/,
    );
  } finally {
    await fs.rm(directory, { recursive: true, force: true });
  }
});
