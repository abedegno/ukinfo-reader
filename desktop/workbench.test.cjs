"use strict";
const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs/promises"),
  os = require("node:os"),
  path = require("node:path");
const model = require("./workbench-model.cjs");
const { createResearchStore } = require("./research-store.cjs");
test("workspace rejects unsafe query recipes and preserves drafts with serialized native writes", async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), "ukinfo-workbench-"),
  );
  const options = { model, filename: "workbench.json", label: "Workspace" };
  try {
    const store = createResearchStore(directory, options),
      value = model.empty();
    value.revision = 1;
    value.searches.push({
      id: "search",
      name: "Invented search",
      query: { dataset: "synthetic-demo", surname: "Person*", flat: "1" },
      created: new Date().toISOString(),
    });
    value.drafts.push({
      id: "entry",
      base: "saved",
      note: "recover me",
      updated: new Date().toISOString(),
    });
    await store.write(value, 0);
    assert.deepEqual(
      await createResearchStore(directory, options).read(),
      value,
    );
    assert.equal(
      (await fs.stat(path.join(directory, "workbench.json"))).mode & 0o777,
      0o600,
    );
    const stale = structuredClone(value);
    stale.revision++;
    const results = await Promise.allSettled([
      store.write(stale, 1),
      store.write(stale, 1),
    ]);
    assert.equal(
      results.filter((result) => result.status === "fulfilled").length,
      1,
    );
    for (const property of ["cursor", "view", "url", "__proto__"]) {
      const bad = structuredClone(value);
      bad.searches[0].query = JSON.parse(`{"${property}":"untrusted"}`);
      assert.throws(() => model.validate(bad), /Invalid research workspace/);
    }
    assert.throws(
      () => model.parse(" ".repeat(model.MAX_BYTES + 1)),
      /Invalid research workspace/,
    );
    await fs.writeFile(path.join(directory, "workbench.json"), "broken");
    await assert.rejects(store.write(stale, 1), /preserved/);
    assert.equal(
      await fs.readFile(path.join(directory, "workbench.json"), "utf8"),
      "broken",
    );
  } finally {
    await fs.rm(directory, { recursive: true, force: true });
  }
});
