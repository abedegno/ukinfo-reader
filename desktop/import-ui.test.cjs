"use strict";
const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs/promises"),
  os = require("node:os"),
  path = require("node:path");
const { installImporter } = require("./import-ui.cjs");
test("external catalogue connections cannot invoke management and child frames cannot use native capabilities", async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), "ukinfo-control-test-"),
  );
  try {
    const handlers = new Map();
    const frame = { url: "http://127.0.0.1:5000/" };
    const window = { webContents: { mainFrame: frame } };
    const event = { sender: window.webContents, senderFrame: frame };
    installImporter({
      ipcMain: { handle: (name, fn) => handlers.set(name, fn) },
      dialog: {},
      getWindow: () => window,
      server: {
        address: "http://127.0.0.1:5000",
        control: { url: "unused", token: "unused" },
      },
      runtime: directory,
      isPackaged: true,
      directory,
      ownsCatalogue: false,
      log: () => {},
    });
    assert.equal(
      handlers.get("ukinfo:capabilities")(event).canManageCatalogue,
      false,
    );
    await assert.rejects(
      handlers.get("ukinfo:catalogue")(event),
      /app-owned local/,
    );
    await assert.rejects(
      handlers.get("ukinfo:manage-dataset")(event, {
        selection: "invented",
        action: "remove",
      }),
      /app-owned local/,
    );
    assert.throws(
      () =>
        handlers.get("ukinfo:capabilities")({
          ...event,
          senderFrame: { url: frame.url },
        }),
      /Archive window/,
    );
  } finally {
    await fs.rm(directory, { recursive: true, force: true });
  }
});
