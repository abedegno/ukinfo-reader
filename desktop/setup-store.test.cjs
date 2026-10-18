"use strict";
const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs/promises"),
  os = require("node:os"),
  path = require("node:path");
const { createSetupStore } = require("./setup-store.cjs");
test("setup source survives restart, drops unexpected fields and preserves malformed history", async () => {
  const dir = await fs.mkdtemp(path.join(os.tmpdir(), "ukinfo-setup-test-"));
  try {
    const store = createSetupStore(dir);
    assert.equal(await store.read(), null);
    await store.write({
      kind: "media",
      path: path.join(dir, "source.iso"),
      status: "running",
      edition: "invented",
      title: "Invented",
      limit: 10000,
      untrusted: "drop",
    });
    const value = await createSetupStore(dir).read();
    assert.equal(value.status, "running");
    assert.equal(value.limit, 10000);
    assert.equal(value.untrusted, undefined);
    await assert.rejects(
      store.write({ kind: "folder", path: "relative", status: "ready" }),
      /Invalid/,
    );
    assert.equal((await store.read()).status, "running");
    await fs.writeFile(path.join(dir, "setup-state.json"), "{broken");
    await assert.rejects(store.write({ kind: "demo", status: "complete" }));
    assert.equal(
      await fs.readFile(path.join(dir, "setup-state.json"), "utf8"),
      "{broken",
    );
  } finally {
    await fs.rm(dir, { recursive: true, force: true });
  }
});
