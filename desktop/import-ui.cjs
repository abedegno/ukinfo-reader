"use strict";
const { randomUUID } = require("node:crypto");
const path = require("node:path");
const { importerCommand, identify, beginImport } = require("./importer.cjs");

function installImporter({
  ipcMain,
  dialog,
  getWindow,
  server,
  runtime,
  isPackaged,
  log,
}) {
  const command = importerCommand(runtime, isPackaged);
  const control = server.control;
  const selections = new Map();
  let active = null;
  function trusted(event) {
    const window = getWindow();
    if (
      !window ||
      event.sender !== window.webContents ||
      !event.senderFrame ||
      new URL(event.senderFrame.url).origin !== server.address
    )
      throw new Error("Archive window required.");
    return window;
  }
  ipcMain.handle("ukinfo:capabilities", (event) => {
    trusted(event);
    return { canImport: Boolean(command && control), portable: isPackaged };
  });
  ipcMain.handle("ukinfo:choose-source", async (event) => {
    const window = trusted(event);
    if (!command || !control)
      throw new Error("The desktop importer is not installed.");
    if (active) throw new Error("Wait for the current import to finish.");
    const result = await dialog.showOpenDialog(window, {
      title: "Choose extracted UK-Info database files",
      properties: ["openDirectory"],
    });
    if (result.canceled || result.filePaths.length !== 1) return null;
    const directory = result.filePaths[0];
    let source;
    try {
      source = await identify(command, directory);
    } catch (error) {
      log(error.message + "\n");
      throw new Error(
        "Unable to verify that folder. Check that the required database files were extracted; see server.log for details.",
      );
    }
    const selection = randomUUID();
    selections.clear();
    selections.set(selection, directory);
    return {
      selection,
      label: path.basename(directory),
      title: source.title,
      edition: source.edition,
      entries: source.entries,
    };
  });
  ipcMain.handle("ukinfo:demo", async (event) => {
    trusted(event);
    if (!command || !control)
      throw new Error("The desktop importer is not installed.");
    if (active) throw new Error("An import is already running.");
    const source = await identify(command, null);
    const selection = randomUUID();
    selections.clear();
    selections.set(selection, null);
    return { selection, label: "Invented demo", ...source };
  });
  ipcMain.handle("ukinfo:import", async (event, { selection, limit } = {}) => {
    trusted(event);
    if (active) throw new Error("An import is already running.");
    if (!command || !control || !selections.has(selection))
      throw new Error("Choose a source folder first.");
    const job = beginImport(
      command,
      selections.get(selection),
      limit,
      control,
      (state) => {
        const window = getWindow();
        if (window && !window.isDestroyed())
          window.webContents.send("ukinfo:import-progress", state);
      },
      log,
    );
    active = job;
    try {
      return await job.done;
    } finally {
      if (active === job) active = null;
    }
  });
  ipcMain.handle("ukinfo:cancel-import", (event) => {
    trusted(event);
    if (active) active.cancel();
    return { canceling: Boolean(active) };
  });
  return {
    cancel: async () => {
      if (active) {
        active.cancel();
        try {
          await active.done;
        } catch {}
      }
    },
  };
}
module.exports = { installImporter };
