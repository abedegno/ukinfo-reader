"use strict";
const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const os = require("node:os");
const path = require("node:path");
const { beginPrepareMedia } = require("./importer.cjs");
test("disc preparation cancels the owned process group and settles before cleanup", async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), "ukinfo-prepare-test-"),
  );
  try {
    const script = path.join(directory, "decoder.cjs");
    await fs.writeFile(
      script,
      `console.error('UKINFO_PROGRESS '+JSON.stringify({phase:'extracting disc files',done:1,total:2})); setInterval(()=>{},1000);`,
    );
    let resolve;
    const progress = new Promise((done) => {
      resolve = done;
    });
    const job = beginPrepareMedia(
      { executable: process.execPath, args: [script] },
      "unused.iso",
      directory,
      resolve,
      () => {},
    );
    const done = assert.rejects(job.done, /canceled/);
    const state = await progress;
    assert.equal(state.progress.done, 1);
    job.cancel();
    await done;
  } finally {
    await fs.rm(directory, { recursive: true, force: true });
  }
});
