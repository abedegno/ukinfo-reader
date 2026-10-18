"use strict";
const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const os = require("node:os");
const path = require("node:path");
const { collectDiagnostics, installDiagnostics } = require("./diagnostics.cjs");
test("diagnostics exclude private values, tolerate unavailable health, and never inspect external catalogues", async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), "ukinfo-diagnostics-"),
  );
  const secret = "PRIVATE-NAME-PATH-PASSWORD";
  const requests = [];
  try {
    await fs.writeFile(path.join(directory, "server.log"), secret);
    await fs.writeFile(path.join(directory, "research-lists.json"), secret);
    await fs.writeFile(
      path.join(directory, "setup-state.json"),
      JSON.stringify({
        version: 1,
        kind: "media",
        path: "/" + secret,
        title: secret,
        edition: secret,
        status: "stopped",
        limit: 10000,
      }),
    );
    const options = {
      app: { getVersion: () => "0.3.4", isPackaged: true },
      directory,
      runtime: directory,
      getLifecycle: () => "running",
      ownsCatalogue: true,
      getServer: () => ({
        address: "http://127.0.0.1:1",
        control: { url: "http://127.0.0.1:2", token: secret },
      }),
      env: {
        UKINFO_DATA_DIR: "/" + secret,
        UKINFO_DESKTOP_DATABASE_URL: "postgres://" + secret,
      },
    };
    const originalFetch = global.fetch;
    global.fetch = async (url) => {
      requests.push(url);
      if (url.endsWith("/api/health")) throw new Error(secret);
      return {
        ok: true,
        json: async () => ({
          database_bytes: 123,
          state: { running: false, error: secret },
          datasets: [
            {
              manifest: {
                edition: "se4-pcf94-1999-04",
                title: secret,
                sample: true,
                sha256: secret,
              },
              entries: 10000,
              bytes: 123,
              address_ready: true,
              record: secret,
            },
            { manifest: { edition: secret }, key: secret, entries: secret },
          ],
          geography: { source: secret },
        }),
      };
    };
    try {
      const report = await collectDiagnostics(options);
      assert.equal(report.health, "unreachable");
      assert.equal(report.catalogue.datasets[0].edition, "se4-pcf94-1999-04");
      assert.equal(report.catalogue.datasets[1].edition, "other");
      assert.equal(report.catalogue.datasets[1].entries, null);
      assert.equal(report.catalogue.operation_error_present, true);
      assert.equal(report.setup.status, "stopped");
      assert.equal(report.setup.sample_limit, 10000);
      assert.equal(JSON.stringify(report).includes(secret), false);
      assert.equal(JSON.stringify(report).includes(directory), false);
      requests.length = 0;
      const external = await collectDiagnostics({
        ...options,
        ownsCatalogue: false,
      });
      assert.equal(external.catalogue.status, "external-not-inspected");
      assert.equal(requests.length, 1);
      await fs.writeFile(path.join(directory, "setup-state.json"), secret);
      const broken = await collectDiagnostics({
        ...options,
        getServer: () => null,
        getLifecycle: () => "startup-failed",
      });
      assert.equal(broken.setup.status, "unavailable");
      assert.equal(broken.health, "not-running");
      assert.equal(
        await fs.readFile(path.join(directory, "setup-state.json"), "utf8"),
        secret,
      );
    } finally {
      global.fetch = originalFetch;
    }
  } finally {
    await fs.rm(directory, { recursive: true, force: true });
  }
});
test("diagnostics IPC rejects child frames; cancel writes nothing; native save produces private JSON without returning paths", async (t) => {
  t.mock.method(global, "fetch", async () => ({ ok: false }));
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), "ukinfo-diagnostics-save-"),
  );
  try {
    const handlers = new Map();
    const frame = { url: "http://127.0.0.1:5000/" };
    const window = { webContents: { mainFrame: frame } };
    const event = { sender: window.webContents, senderFrame: frame };
    const filename = path.join(directory, "report.json");
    let canceled = true,
      dialogs = 0;
    installDiagnostics({
      ipcMain: { handle: (name, fn) => handlers.set(name, fn) },
      dialog: {
        showSaveDialog: async () => {
          dialogs++;
          return { canceled, filePath: filename };
        },
      },
      getWindow: () => window,
      app: { getVersion: () => "0.3.4", isPackaged: false },
      directory,
      runtime: directory,
      ownsCatalogue: true,
      getServer: () => ({ address: frame.url.slice(0, -1) }),
      getLifecycle: () => "running",
      env: {},
    });
    const save = handlers.get("ukinfo:export-diagnostics");
    assert.throws(
      () => save({ ...event, senderFrame: { url: frame.url } }),
      /Archive window/,
    );
    assert.equal(dialogs, 0);
    assert.deepEqual(await save(event), { canceled: true });
    assert.deepEqual(await fs.readdir(directory), []);
    canceled = false;
    assert.deepEqual(await save(event), { canceled: false });
    const report = JSON.parse(await fs.readFile(filename, "utf8"));
    assert.equal(report.schema, "ukinfo-desktop-diagnostics");
    assert.equal(report.health, "unhealthy");
    if (process.platform !== "win32")
      assert.equal((await fs.stat(filename)).mode & 0o777, 0o600);
    assert.deepEqual(await fs.readdir(directory), ["report.json"]);
  } finally {
    await fs.rm(directory, { recursive: true, force: true });
  }
});
