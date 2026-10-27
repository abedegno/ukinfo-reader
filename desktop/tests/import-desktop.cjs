"use strict";
const { _electron: electron } = require("playwright");
const { expect } = require("@playwright/test");
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
    assert.equal(
      await app.evaluate(({ Menu }) =>
        Menu.getApplicationMenu().items.some((item) =>
          item.submenu?.items.some(
            (child) => child.label === "Export diagnostics…",
          ),
        ),
      ),
      true,
      "native Help menu offers diagnostics even during setup",
    );
    const capabilities = await window.evaluate(() =>
      window.ukinfoDesktop.capabilities(),
    );
    assert.equal(capabilities.canImport, true);
    assert.equal("token" in capabilities, false);
    await window.locator("#import-dialog[open]").waitFor();
    await fs.promises.mkdir(path.join(root, "desktop/test-results"), {
      recursive: true,
    });
    await window.locator("#import-dialog").screenshot({
      path: path.join(root, "desktop/test-results/desktop-first-launch.png"),
    });
    assert.match(
      await window.locator("#import-heading").innerText(),
      /Set up your archive/,
    );
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
    await window.getByLabel("Surname", { exact: true }).fill("Example");
    await window.getByRole("button", { name: "Search archive" }).click();
    await window.locator("article").first().waitFor();
    assert.equal(await window.locator("article").count(), 50);
    const exportPath = path.join(directory, "invented-page.jsonl");
    await app.evaluate(({ session }, target) => {
      globalThis.ukinfoDownload = new Promise((resolve, reject) => {
        const timer = setTimeout(
          () => reject(new Error("Desktop export timed out")),
          10000,
        );
        session.defaultSession.once("will-download", (_event, item) => {
          item.setSavePath(target);
          item.once("done", (_done, state) => {
            clearTimeout(timer);
            if (state === "completed") resolve();
            else reject(new Error(`Desktop export ${state}`));
          });
        });
      });
    }, exportPath);
    await window.getByRole("button", { name: "Export this page" }).click();
    await app.evaluate(() => globalThis.ukinfoDownload);
    const exported = fs
      .readFileSync(exportPath, "utf8")
      .trim()
      .split("\n")
      .map(JSON.parse);
    assert.equal(exported.length, 50);
    assert.equal(exported[0].dataset, "synthetic-demo");
    assert.equal(exported[0].record.building, null);
    await window.getByLabel("Surname", { exact: true }).fill("");
    await window.getByText("More address filters", { exact: true }).click();
    assert.equal(
      await window.getByLabel("Street", { exact: true }).isEnabled(),
      true,
    );
    await window.getByLabel("Street", { exact: true }).fill("Invented Street");
    await window
      .getByLabel("Town or city", { exact: true })
      .fill("Fictional Town");
    await window.getByRole("button", { name: "Search archive" }).click();
    await window.locator("article").first().waitFor();
    assert.equal(await window.locator("article").count(), 50);
    await window.getByRole("button", { name: "Next page" }).click();
    await window.locator("#status").filter({ hasText: "Page 2" }).waitFor();
    assert.equal(await window.locator("article").count(), 50);
    await window
      .locator("article")
      .first()
      .getByRole("button", { name: "View record" })
      .click();
    await window.locator("#explore-address:enabled").waitFor();
    await window
      .getByRole("button", { name: "Explore this address", exact: true })
      .click();
    await window.locator("#address-items article").first().waitFor();
    assert.equal(await window.locator("#address-items article").count(), 1);
    await window
      .getByRole("button", { name: "Close address", exact: true })
      .click();
    await window
      .getByRole("button", { name: "Clear filters", exact: true })
      .click();
    await window.locator('[name="postcode"]').fill("ZZ1");
    if (
      !(await window.locator("#address-controls").evaluate((node) => node.open))
    )
      await window.locator("#address-controls summary").click();
    await window.getByLabel("House number", { exact: true }).fill("12");
    await window.locator('[name="first_name"]').fill("Person");
    if (!(await window.locator("#name-options").evaluate((node) => node.open)))
      await window.locator("#name-options summary").click();
    await window.locator('[name="first_match"]').selectOption("given");
    await window.getByRole("button", { name: "Search archive" }).click();
    await window
      .locator("#status")
      .filter({ hasText: "1 source entries" })
      .waitFor();
    assert.equal(await window.locator("#items article").count(), 1);
    assert.match(
      await window.locator("#items article").innerText(),
      /Person 011/,
    );
    await window
      .getByRole("button", { name: "Open saved link", exact: true })
      .click();
    await window
      .getByLabel("Saved link", { exact: true })
      .fill(
        "http://127.0.0.1:1/?view=record&record_dataset=synthetic-demo&record_id=1&record_source=invented%3A0",
      );
    await window
      .getByRole("button", { name: "Open link", exact: true })
      .click();
    await window
      .locator("#record-title")
      .filter({ hasText: "Person 000 Example" })
      .waitFor();
    await window
      .getByRole("button", { name: "Save to list", exact: true })
      .click();
    await window
      .locator("#research-new-name")
      .fill("Invented desktop research");
    await window
      .locator("#research-save-note")
      .fill("Saved across desktop restarts.");
    await window
      .getByRole("button", { name: "Save entry", exact: true })
      .click();
    await window
      .locator("#record-feedback")
      .filter({ hasText: "Saved to research list" })
      .waitFor();
    await window
      .getByRole("button", { name: "Close details", exact: true })
      .click();
    await app.evaluate(({ shell }) => {
      shell.openExternal = async (url) => {
        globalThis.ukinfoOpenedURL = url;
      };
    });
    await window.getByRole("link", { name: "About the project" }).click();
    assert.equal(
      await app.evaluate(() => globalThis.ukinfoOpenedURL),
      "https://github.com/abedegno/ukinfo-reader",
    );
    await window.evaluate(() => window.open("https://untrusted.invalid/"));
    assert.equal(app.windows().length, 1);
    assert.equal(
      await app.evaluate(() => globalThis.ukinfoOpenedURL),
      "https://github.com/abedegno/ukinfo-reader",
    );
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
    // The chooser returns an opaque selection; a wrong ZIP reaches the owned
    // server and fails its pinned hash without changing the current reference.
    await window.locator("#import-dialog").evaluate((node) => node.close());
    const wrongZip = path.join(directory, "wrong-reference.zip");
    fs.writeFileSync(wrongZip, "Invented ZIP fixture");
    await app.evaluate(({ dialog }, file) => {
      dialog.showOpenDialog = async () => ({
        canceled: false,
        filePaths: [file],
      });
    }, wrongZip);
    await window.locator("#postcode-setup-open").click();
    await window.locator("#postcode-choose").click();
    await window.locator("#postcode-start").click();
    await window
      .locator("#postcode-message")
      .filter({ hasText: "SHA-256 mismatch" })
      .waitFor({ timeout: 30000 });
    if (process.env.UKINFO_TEST_ONSPD) {
      await app.evaluate(({ dialog }, file) => {
        dialog.showOpenDialog = async () => ({
          canceled: false,
          filePaths: [file],
        });
      }, process.env.UKINFO_TEST_ONSPD);
      await window.locator("#postcode-choose").click();
      await window.locator("#postcode-start").click();
      await window
        .locator("#postcode-message")
        .filter({ hasText: "Postcode locations are ready" })
        .waitFor({ timeout: 240000 });
      const map = await window.evaluate(() =>
        fetch("/api/map?postcode=WS137SW").then((r) => r.json()),
      );
      assert.equal(map.points.length, 1);
      await window.locator("#postcode-setup-dialog").screenshot({
        path: path.join(
          root,
          "desktop/test-results/desktop-postcode-setup.png",
        ),
      });
    }
    await window.locator("#postcode-setup-close").click();
    if (process.env.UKINFO_TEST_MEDIA) {
      await app.evaluate(({ dialog }, file) => {
        dialog.showOpenDialog = async () => ({
          canceled: false,
          filePaths: [file],
        });
      }, process.env.UKINFO_TEST_MEDIA);
      await window.locator("#import-open").click();
      await window.locator("#import-media").click();
      await window
        .locator("#import-message")
        .filter({ hasText: "Source fingerprints match" })
        .waitFor({ timeout: 240000 });
      await window.locator("#import-scope").selectOption("10000");
      await window.locator("#import-start").click();
      await window
        .locator("#import-message")
        .filter({ hasText: "Edition imported successfully" })
        .waitFor({ timeout: 120000 });
      await window.locator("#import-search").click();
    }
    await app.close();
    app = null;
    assert.equal(
      fs.existsSync(path.join(directory, "catalogue/pg18/postmaster.pid")),
      false,
    );
    const saved = JSON.parse(
      fs.readFileSync(path.join(directory, "research-lists.json"), "utf8"),
    );
    assert.equal(
      saved.lists[0].entries[0].note,
      "Saved across desktop restarts.",
    );
    app = await electron.launch({
      ...(process.env.UKINFO_TEST_APP
        ? { executablePath: process.env.UKINFO_TEST_APP, args: [] }
        : { args: [path.join(root, "desktop/main.cjs")] }),
      env,
      timeout: 60000,
    });
    const reopened = await app.firstWindow();
    await reopened
      .locator('#dataset option[value="synthetic-demo"]')
      .waitFor({ state: "attached" });
    assert.equal(
      await reopened.locator("#import-dialog").evaluate((node) => node.open),
      false,
      "existing catalogues do not show the first-run wizard",
    );
    await reopened
      .getByRole("button", { name: "Research lists", exact: true })
      .click();
    await reopened.locator("#research-entries textarea").waitFor();
    assert.equal(
      await reopened.locator("#research-entries textarea").inputValue(),
      "Saved across desktop restarts.",
    );
    await reopened
      .getByRole("button", { name: "Open source", exact: true })
      .click();
    await reopened
      .locator("#record-title")
      .filter({ hasText: "Person 000" })
      .waitFor();
    await reopened
      .getByRole("button", { name: "Close details", exact: true })
      .click();
    await reopened
      .getByRole("button", { name: "Research lists", exact: true })
      .click();
    await reopened
      .locator("#research-entries textarea")
      .fill("Unsaved quit check");
    await app.evaluate(({ app, dialog }) => {
      dialog.showMessageBox = async () => ({ response: 0 });
      app.quit();
    });
    await reopened.waitForTimeout(250);
    assert.equal(
      await reopened.evaluate(() => fetch("/api/health").then((r) => r.ok)),
      true,
      "cancelling quit keeps the server alive",
    );
    assert.equal(
      await reopened.locator("#research-entries textarea").inputValue(),
      "Unsaved quit check",
    );
    await reopened
      .locator("#research-entries textarea")
      .fill("Saved across desktop restarts.");
    await reopened.locator("#research-dialog").evaluate((node) => node.close());
    await reopened.locator("#catalogue-open").click();
    await reopened
      .locator(".catalogue-edition")
      .filter({ hasText: "Synthetic demo" })
      .waitFor();
    await reopened
      .locator(".catalogue-edition")
      .filter({ hasText: "Synthetic demo" })
      .getByRole("button", { name: "Build research index", exact: true })
      .click();
    await reopened
      .locator("#catalogue-message")
      .filter({ hasText: "Research index is ready" })
      .waitFor({ timeout: 30000 });
    const researchInventory = await reopened.evaluate(() =>
      window.ukinfoDesktop.catalogue(),
    );
    assert.equal(
      researchInventory.datasets.find((item) => item.key === "synthetic-demo")
        .research_ready,
      true,
    );
    const diagnosticPath = path.join(directory, "diagnostics.json");
    await app.evaluate(({ dialog }, filePath) => {
      dialog.showSaveDialog = async () => ({ canceled: false, filePath });
    }, diagnosticPath);
    await reopened
      .getByRole("button", { name: "Export diagnostics…", exact: true })
      .click();
    await reopened
      .locator("#diagnostics-message")
      .filter({ hasText: "Diagnostics saved" })
      .waitFor();
    const diagnosticText = fs.readFileSync(diagnosticPath, "utf8");
    const diagnostic = JSON.parse(diagnosticText);
    assert.equal(diagnostic.health, "healthy");
    assert.equal(diagnostic.catalogue.status, "available");
    assert.equal(
      diagnostic.catalogue.datasets.some((d) => d.edition === "synthetic-demo"),
      true,
    );
    assert.equal(diagnosticText.includes(directory), false);
    assert.equal(
      diagnosticText.includes("Saved across desktop restarts."),
      false,
    );
    assert.equal(diagnosticText.includes("127.0.0.1"), false);
    await reopened.locator("#catalogue-dialog").screenshot({
      path: path.join(root, "desktop/test-results/desktop-catalogue.png"),
    });
    await app.evaluate(({ dialog }) => {
      dialog.showMessageBox = async () => ({ response: 0 });
    });
    const demoCard = reopened
      .locator(".catalogue-edition")
      .filter({ hasText: "Synthetic demo" });
    await demoCard
      .getByRole("button", { name: "Remove edition…", exact: true })
      .click();
    await reopened
      .locator("#catalogue-message")
      .filter({ hasText: "Edition kept" })
      .waitFor();
    await app.evaluate(({ dialog }) => {
      dialog.showMessageBox = async () => ({ response: 1 });
    });
    await demoCard
      .getByRole("button", { name: "Remove edition…", exact: true })
      .click();
    await reopened
      .locator("#catalogue-message")
      .filter({ hasText: "Research lists and notes are preserved" })
      .waitFor();
    // Success feedback precedes the asynchronous catalogue refresh.
    await expect(demoCard).toHaveCount(0);
    const preserved = JSON.parse(
      fs.readFileSync(path.join(directory, "research-lists.json"), "utf8"),
    );
    assert.equal(
      preserved.lists[0].entries[0].note,
      "Saved across desktop restarts.",
    );
    const setup = await reopened.evaluate(() =>
      window.ukinfoDesktop.catalogue(),
    );
    if (!process.env.UKINFO_TEST_MEDIA && !process.env.UKINFO_TEST_REAL_SOURCE)
      assert.equal(setup.setup.kind, "demo");
    assert.equal("path" in setup.setup, false);
    await reopened.locator("#catalogue-close").click();
    await app.close();
    app = null;
    assert.equal(
      fs.existsSync(path.join(directory, "catalogue/pg18/postmaster.pid")),
      false,
    );
    // Simulate a process interruption after removal, then verify that the next
    // launch rechecks the retained original source instead of losing setup.
    if (
      !process.env.UKINFO_TEST_MEDIA &&
      !process.env.UKINFO_TEST_REAL_SOURCE
    ) {
      const setupPath = path.join(directory, "setup-state.json");
      const previousSetup = JSON.parse(fs.readFileSync(setupPath, "utf8"));
      fs.writeFileSync(
        setupPath,
        JSON.stringify({ ...previousSetup, status: "running" }),
      );
      app = await electron.launch({
        ...(process.env.UKINFO_TEST_APP
          ? { executablePath: process.env.UKINFO_TEST_APP, args: [] }
          : { args: [path.join(root, "desktop/main.cjs")] }),
        env,
        timeout: 60000,
      });
      const resumed = await app.firstWindow();
      await resumed.locator("#import-dialog[open]").waitFor();
      await resumed.locator("#import-resume").click();
      await resumed
        .locator("#import-title")
        .filter({ hasText: "Synthetic demo" })
        .waitFor();
      await resumed.locator("#import-start").click();
      await resumed
        .locator("#import-message")
        .filter({ hasText: "Edition imported successfully" })
        .waitFor({ timeout: 60000 });
      assert.equal(
        JSON.parse(
          fs.readFileSync(path.join(directory, "research-lists.json"), "utf8"),
        ).lists[0].entries[0].note,
        "Saved across desktop restarts.",
      );
      await app.close();
      app = null;
      assert.equal(
        fs.existsSync(path.join(directory, "catalogue/pg18/postmaster.pid")),
        false,
      );
    }
    console.log(
      "Desktop wizard/import/search/export/lists/relaunch/navigation/shutdown passed" +
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
