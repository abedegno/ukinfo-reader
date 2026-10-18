"use strict";
const { randomUUID } = require("node:crypto");
const path = require("node:path");
const fs = require("node:fs/promises");
const os = require("node:os");
const {
  importerCommand,
  identify,
  beginImport,
  beginGeographyImport,
  beginPrepareMedia,
} = require("./importer.cjs");

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
  let selecting = false;
  async function select(action) {
    if (active || selecting)
      throw new Error("Wait for the current import or selection to finish.");
    selecting = true;
    try {
      return await action();
    } finally {
      selecting = false;
    }
  }
  let geographySelection = null;
  function trusted(event) {
    const window = getWindow();
    if (
      !window ||
      event.sender !== window.webContents ||
      event.senderFrame !== window.webContents.mainFrame ||
      !event.senderFrame ||
      new URL(event.senderFrame.url).origin !== server.address
    )
      throw new Error("Archive window required.");
    return window;
  }
  ipcMain.handle("ukinfo:capabilities", (event) => {
    trusted(event);
    return {
      canImport: Boolean(command && control),
      canImportGeography: Boolean(control),
      portable: isPackaged,
    };
  });
  ipcMain.handle("ukinfo:geography-source", async (event) => {
    trusted(event);
    if (!control) throw new Error("Local postcode setup is unavailable.");
    const response = await fetch(`${control.url}/geography-source`, {
      headers: { Authorization: `Bearer ${control.token}` },
      signal: AbortSignal.timeout(3000),
    });
    if (!response.ok) throw new Error("Postcode source details unavailable.");
    return response.json();
  });
  ipcMain.handle("ukinfo:choose-geography", async (event) => {
    const window = trusted(event);
    if (!control) throw new Error("Local postcode setup is unavailable.");
    if (active) throw new Error("Wait for the current import to finish.");
    const result = await dialog.showOpenDialog(window, {
      title: "Choose the original ONSPD ZIP",
      properties: ["openFile"],
      filters: [{ name: "ONSPD ZIP archive", extensions: ["zip"] }],
    });
    if (result.canceled || result.filePaths.length !== 1) return null;
    geographySelection = { id: randomUUID(), file: result.filePaths[0] };
    return {
      selection: geographySelection.id,
      label: path.basename(geographySelection.file),
    };
  });
  ipcMain.handle("ukinfo:import-geography", async (event, selection) => {
    trusted(event);
    if (active || selecting)
      throw new Error("An import or source selection is already running.");
    if (!control || !geographySelection || selection !== geographySelection.id)
      throw new Error("Choose an ONSPD ZIP first.");
    const job = beginGeographyImport(
      geographySelection.file,
      control,
      (state) => {
        const window = getWindow();
        if (window && !window.isDestroyed())
          window.webContents.send("ukinfo:geography-progress", state);
      },
    );
    active = job;
    try {
      return await job.done;
    } finally {
      if (active === job) active = null;
    }
  });
  async function clearSelections() {
    for (const value of selections.values())
      if (value.temporary)
        await fs.rm(value.temporary, { recursive: true, force: true });
    selections.clear();
  }
  const sendProgress = (state) => {
    const window = getWindow();
    if (window && !window.isDestroyed())
      window.webContents.send("ukinfo:import-progress", state);
  };
  async function chooseSource(event, media) {
    const window = trusted(event);
    if (!command || !control)
      throw new Error("The desktop importer is not installed.");
    if (active) throw new Error("Wait for the current import to finish.");
    const result = await dialog.showOpenDialog(window, {
      title: media
        ? "Choose a UK-Info ISO or 7z cover disc"
        : "Choose extracted UK-Info database files",
      properties: [media ? "openFile" : "openDirectory"],
      ...(media
        ? {
            filters: [
              { name: "Disc images and archives", extensions: ["iso", "7z"] },
            ],
          }
        : {}),
    });
    if (result.canceled || result.filePaths.length !== 1) return null;
    if (active) throw new Error("Wait for the current import to finish.");
    await clearSelections();
    const file = result.filePaths[0];
    let temporary = null,
      directory = file,
      source;
    try {
      if (media) {
        temporary = await fs.mkdtemp(path.join(os.tmpdir(), "ukinfo-disc-"));
        directory = path.join(temporary, "database");
        const job = beginPrepareMedia(
          command,
          file,
          directory,
          sendProgress,
          log,
        );
        job.done = job.done.catch(async (error) => {
          await fs.rm(temporary, { recursive: true, force: true });
          throw error;
        });
        active = job;
        try {
          source = await job.done;
        } finally {
          if (active === job) active = null;
        }
      } else source = await identify(command, directory);
    } catch (error) {
      if (temporary) await fs.rm(temporary, { recursive: true, force: true });
      log(error.message + "\n");
      throw new Error(
        media
          ? error.message
          : "Unable to verify that folder. Check that the required database files were extracted; see server.log for details.",
      );
    }
    const selection = randomUUID();
    selections.set(selection, { directory, temporary });
    return {
      selection,
      label: path.basename(file),
      title: source.title,
      edition: source.edition,
      entries: source.entries,
    };
  }
  ipcMain.handle("ukinfo:choose-source", (event) =>
    select(() => chooseSource(event, false)),
  );
  ipcMain.handle("ukinfo:choose-media", (event) =>
    select(() => chooseSource(event, true)),
  );
  ipcMain.handle("ukinfo:demo", (event) =>
    select(async () => {
      trusted(event);
      if (!command || !control)
        throw new Error("The desktop importer is not installed.");
      if (active)
        throw new Error("An import or source selection is already running.");
      const source = await identify(command, null);
      const selection = randomUUID();
      await clearSelections();
      selections.set(selection, { directory: null, temporary: null });
      return { selection, label: "Invented demo", ...source };
    }),
  );
  ipcMain.handle("ukinfo:import", async (event, { selection, limit } = {}) => {
    trusted(event);
    if (active || selecting)
      throw new Error("An import or source selection is already running.");
    if (!command || !control || !selections.has(selection))
      throw new Error("Choose a source folder first.");
    const job = beginImport(
      command,
      selections.get(selection).directory,
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
      await clearSelections();
    },
  };
}
module.exports = { installImporter };
