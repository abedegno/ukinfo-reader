"use strict";
// Native end-to-end smoke: fresh local cluster, import, app startup/search, shutdown.
// Explicit opt-in via npm run test:electron and UKINFO_PG_BIN; no source data is used.
const { _electron: electron } = require("playwright");
const { spawnSync } = require("node:child_process");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const assert = require("node:assert/strict");
(async () => {
  if (!process.env.UKINFO_PG_BIN)
    throw new Error("Set UKINFO_PG_BIN to native PostgreSQL 18 binaries.");
  const root = path.resolve(__dirname, "../..");
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "ukinfo-electron-"));
  const server =
    process.env.UKINFO_SERVER || path.join(root, "build/desktop/ukinfo-server");
  const env = {
    ...process.env,
    UKINFO_DATA_DIR: directory,
    ELECTRON_RUN_AS_NODE: "",
  };
  if (!process.env.UKINFO_TEST_APP) env.UKINFO_SERVER = server;
  else {
    delete env.UKINFO_SERVER;
    delete env.UKINFO_PG_BIN;
  }
  delete env.DATABASE_URL;
  delete env.UKINFO_DESKTOP_DATABASE_URL;
  let app;
  try {
    const demo = spawnSync(
      process.env.PYTHON || "python3",
      [path.join(root, "tools/search_demo.py")],
      { encoding: "utf8" },
    );
    assert.equal(demo.status, 0, demo.stderr);
    const imported = spawnSync(
      server,
      [
        "import",
        "--data-dir",
        path.join(directory, "catalogue"),
        "--pg-bin",
        process.env.UKINFO_PG_BIN,
      ],
      { env, input: demo.stdout, encoding: "utf8", timeout: 60000 },
    );
    assert.equal(imported.status, 0, imported.stderr);
    app = await electron.launch({
      ...(process.env.UKINFO_TEST_APP
        ? { executablePath: process.env.UKINFO_TEST_APP, args: [] }
        : { args: [path.join(root, "desktop/main.cjs")] }),
      env,
      timeout: 60000,
    });
    const window = await app.firstWindow();
    await window.locator("#dataset option").waitFor({ state: "attached" });
    await window.getByLabel("Surname", { exact: true }).fill("Example");
    await window.getByRole("button", { name: "Search archive" }).click();
    await window.locator("article").first().waitFor();
    assert.equal(await window.locator("article").count(), 50);
    assert.match(
      await window.locator("article").first().innerText(),
      /Person 000 Example/,
    );
    await fs.promises.mkdir(path.join(root, "desktop/test-results"), {
      recursive: true,
    });
    await window.screenshot({
      path: path.join(root, "desktop/test-results/electron-native.png"),
    });
    await app.close();
    app = null;
    assert.equal(
      fs.existsSync(path.join(directory, "catalogue/pg18/postmaster.pid")),
      false,
      "owned PostgreSQL process must stop with the app",
    );
    console.log(
      "Native Electron + Go + PostgreSQL import/search/shutdown passed.",
    );
  } finally {
    if (app) await app.close();
    // Leave a failed, still-running cluster intact for inspection.
    if (!fs.existsSync(path.join(directory, "catalogue/pg18/postmaster.pid")))
      fs.rmSync(directory, { recursive: true, force: true });
    else console.error(`Preserving cluster for diagnosis: ${directory}`);
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
