"use strict";
const { _electron: electron } = require("playwright");
const fs = require("node:fs/promises"),
  os = require("node:os"),
  path = require("node:path"),
  assert = require("node:assert/strict");
(async () => {
  const root = await fs.mkdtemp(
    path.join(os.tmpdir(), "ukinfo-backup-native-"),
  );
  const directory = path.join(root, "profile"),
    copies = path.join(root, "copies");
  await fs.mkdir(copies);
  const env = {
    ...process.env,
    UKINFO_DATA_DIR: directory,
    ELECTRON_RUN_AS_NODE: "",
  };
  delete env.DATABASE_URL;
  delete env.UKINFO_DESKTOP_DATABASE_URL;
  if (process.env.UKINFO_TEST_APP) {
    delete env.UKINFO_SERVER;
    delete env.UKINFO_PG_BIN;
  }
  let app;
  async function launch() {
    app = await electron.launch({
      ...(process.env.UKINFO_TEST_APP
        ? { executablePath: process.env.UKINFO_TEST_APP, args: [] }
        : { args: [path.resolve(__dirname, "../main.cjs")] }),
      env,
      timeout: 60000,
    });
    const window = await app.firstWindow();
    await window.locator("#import-open").waitFor();
    return window;
  }
  async function copy(window, action, choices) {
    await app.evaluate(({ app, dialog }, locations) => {
      app.relaunch = () => {};
      dialog.showOpenDialog = async () => ({
        canceled: false,
        filePaths: [locations.shift()],
      });
      dialog.showMessageBox = async (...args) => ({
        response: args.at(-1).buttons?.length === 2 ? 1 : 0,
      });
    }, choices);
    const process = app.process();
    const exited = new Promise((resolve, reject) => {
      const timer = setTimeout(
        () => reject(new Error("Archive copy did not finish")),
        90000,
      );
      process.once("exit", () => {
        clearTimeout(timer);
        resolve();
      });
    });
    await window.evaluate((action) => {
      window.ukinfoDesktop.archiveMaintenance(action).catch(() => {});
    }, action);
    await exited;
    app = null;
  }
  try {
    let window = await launch();
    await window.locator("#import-dialog[open]").waitFor();
    await window.getByRole("button", { name: "Try invented demo" }).click();
    await window
      .locator("#import-title")
      .filter({ hasText: "Synthetic demo" })
      .waitFor();
    await window
      .getByRole("button", { name: "Import edition", exact: true })
      .click();
    await window
      .locator("#import-message")
      .filter({ hasText: "Edition imported successfully" })
      .waitFor({ timeout: 60000 });
    await window
      .getByRole("button", { name: "Start searching", exact: true })
      .click();
    await window.evaluate(async () => {
      const next = await window.ukinfoDesktop.researchRead();
      next.revision++;
      next.lists = [
        { id: "backup-list", name: "Invented backup research", entries: [] },
      ];
      await window.ukinfoDesktop.researchWrite(next, next.revision - 1);
    });
    const before = await fs.readFile(
      path.join(directory, "research-lists.json"),
      "utf8",
    );
    await copy(window, "backup", [copies]);
    const backups = (await fs.readdir(copies)).filter((n) =>
      n.startsWith("UK-Info-backup-"),
    );
    assert.equal(backups.length, 1);
    const backup = path.join(copies, backups[0]);
    const manifest = JSON.parse(
      await fs.readFile(path.join(backup, "ukinfo-backup.json"), "utf8"),
    );
    assert(manifest.files.length > 100);
    assert.equal(
      manifest.files.some((f) => f.name.endsWith("postmaster.pid")),
      false,
    );
    assert.equal(
      await fs.readFile(path.join(directory, "research-lists.json"), "utf8"),
      before,
    );
    window = await launch();
    assert.equal(
      (await window.evaluate(() => window.ukinfoDesktop.researchRead()))
        .lists[0].name,
      "Invented backup research",
    );
    await copy(window, "restore", [backup, copies]);
    const location = JSON.parse(
      await fs.readFile(path.join(directory, "archive-location.json"), "utf8"),
    );
    assert.notEqual(location.directory, await fs.realpath(directory));
    assert.equal(
      await fs.readFile(path.join(directory, "research-lists.json"), "utf8"),
      before,
    );
    window = await launch();
    assert.equal(
      (await window.evaluate(() => window.ukinfoDesktop.researchRead()))
        .lists[0].name,
      "Invented backup research",
    );
    const catalogue = await window.evaluate(() =>
      window.ukinfoDesktop.catalogue(),
    );
    assert.equal(
      catalogue.datasets.some(
        (d) => d.manifest.edition === "synthetic-demo" && d.entries === 126,
      ),
      true,
    );
    await window.getByLabel("Surname", { exact: true }).fill("Example");
    await window.getByRole("button", { name: "Search archive" }).click();
    await window.locator("article").first().waitFor();
    assert.equal(await window.locator("article").count(), 50);
    await app.close();
    app = null;
    assert.equal(
      (await fs.readdir(copies)).some((n) => n.endsWith(".partial")),
      false,
    );
    console.log(
      "Native cold backup, verified restore, preserved original and restored searches/research passed.",
    );
  } finally {
    if (app) await app.close().catch(() => {});
    await fs.rm(root, { recursive: true, force: true });
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
