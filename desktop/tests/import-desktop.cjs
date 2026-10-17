"use strict";
const { _electron: electron } = require("playwright");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const assert = require("node:assert/strict");

(async () => {
  if (!process.env.UKINFO_PG_BIN)
    throw new Error("Set UKINFO_PG_BIN to native PostgreSQL 18 binaries.");
  const root = path.resolve(__dirname, "../..");
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "ukinfo-import-ui-"));
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
  // The bundled decoder must work without a Python interpreter on PATH.
  if (process.platform !== "win32") env.PATH = "/usr/bin:/bin";
  let app;
  try {
    app = await electron.launch({
      ...(process.env.UKINFO_TEST_APP
        ? { executablePath: process.env.UKINFO_TEST_APP, args: [] }
        : { args: [path.join(root, "desktop/main.cjs")] }),
      env,
      timeout: 60000,
    });
    const window = await app.firstWindow();
    await window.locator("#import-open").waitFor();
    assert.equal(await window.evaluate(() => typeof require), "undefined");
    const capabilities = await window.evaluate(() =>
      window.ukinfoDesktop.capabilities(),
    );
    assert.equal(capabilities.canImport, true);
    assert.equal("token" in capabilities, false);
    await window.getByRole("button", { name: "Add edition" }).click();
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
    await window.getByRole("button", { name: "Close import" }).click();
    await window.getByLabel("Surname", { exact: true }).fill("Example");
    await window.getByRole("button", { name: "Search archive" }).click();
    await window.locator("article").first().waitFor();
    assert.equal(await window.locator("article").count(), 50);
    await fs.promises.mkdir(path.join(root, "desktop/test-results"), {
      recursive: true,
    });
    await window.screenshot({
      path: path.join(root, "desktop/test-results/desktop-import-demo.png"),
    });
    if (process.env.UKINFO_TEST_REAL_SOURCE) {
      await app.evaluate(({ dialog }, source) => {
        dialog.showOpenDialog = async () => ({
          canceled: false,
          filePaths: [source],
        });
      }, process.env.UKINFO_TEST_REAL_SOURCE);
      await window.getByRole("button", { name: "Add edition" }).click();
      await window
        .getByRole("button", { name: "Choose source folder" })
        .click();
      await window
        .locator("#import-title")
        .filter({ hasText: "2005" })
        .waitFor({ timeout: 120000 });
      await window.locator("#import-scope").selectOption("full");
      await window
        .getByRole("button", { name: "Import edition", exact: true })
        .click();
      await window.waitForFunction(
        () =>
          document.getElementById("import-bar").value > 0 &&
          document.getElementById("import-bar").value < 100 &&
          /Importing entries/.test(
            document.getElementById("import-stage").textContent,
          ),
        null,
        { timeout: 120000 },
      );
      await window
        .getByRole("button", { name: "Cancel import", exact: true })
        .click();
      await window
        .locator("#import-message")
        .filter({ hasText: "Import canceled" })
        .waitFor({ timeout: 30000 });
      const catalog = await window.evaluate(() =>
        fetch("/api/datasets").then((r) => r.json()),
      );
      assert.deepEqual(
        catalog.map((d) => d.key),
        ["synthetic-demo"],
        "canceled source import must not publish a dataset",
      );
    }
    await app.close();
    app = null;
    assert.equal(
      fs.existsSync(path.join(directory, "catalogue/pg18/postmaster.pid")),
      false,
    );
    console.log(
      "Desktop GUI demo import/search/shutdown passed" +
        (process.env.UKINFO_TEST_REAL_SOURCE
          ? "; real source cancellation rolled back."
          : "."),
    );
  } finally {
    if (app) await app.close();
    if (!fs.existsSync(path.join(directory, "catalogue/pg18/postmaster.pid")))
      fs.rmSync(directory, { recursive: true, force: true });
    else console.error(`Preserving cluster for diagnosis: ${directory}`);
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
