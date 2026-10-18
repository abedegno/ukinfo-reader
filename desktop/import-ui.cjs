"use strict";
const { randomUUID } = require("node:crypto");
const path = require("node:path");
const fs = require("node:fs/promises");
const os = require("node:os");
const { createSetupStore } = require("./setup-store.cjs");
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
  directory,
  ownsCatalogue,
  shell,
}) {
  const command = importerCommand(runtime, isPackaged);
  const control = server.control;
  const selections = new Map();
  const catalogueSelections = new Map();
  const setup = createSetupStore(directory);
  const saveSetup = async (value) => {
    try {
      await setup.write(value);
    } catch (error) {
      log(`Setup history: ${error.message}\n`);
    }
  };
  async function controlRequest(
    endpoint,
    body,
    signal = AbortSignal.timeout(5000),
  ) {
    if (!control) throw new Error("Local catalogue management is unavailable.");
    const response = await fetch(`${control.url}${endpoint}`, {
      method: body === undefined ? "GET" : "POST",
      headers: {
        Authorization: `Bearer ${control.token}`,
        ...(body === undefined ? {} : { "Content-Type": "application/json" }),
      },
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
      signal,
    });
    const result = await response.json();
    if (!response.ok)
      throw new Error(result.error || "Catalogue operation failed.");
    return result;
  }

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
      canManageCatalogue: Boolean(control && ownsCatalogue),
    };
  });
  ipcMain.handle("ukinfo:show-archive-folder", async (event) => {
    trusted(event);
    const error = await shell.openPath(directory);
    if (error)
      throw new Error("Unable to open the archive folder in the file manager.");
  });
  ipcMain.handle("ukinfo:catalogue", async (event) => {
    trusted(event);
    if (!ownsCatalogue)
      throw new Error(
        "Archive management requires an app-owned local catalogue.",
      );
    const report = await controlRequest("/catalogue");
    const previousSelections = new Map(catalogueSelections);
    catalogueSelections.clear();
    report.datasets = report.datasets.map((item) => {
      const selection =
        [...previousSelections].find(
          ([, old]) => old.id === item.id && old.key === item.key,
        )?.[0] || randomUUID();
      catalogueSelections.set(selection, item);
      const { id, ...visible } = item;
      return { ...visible, selection };
    });
    try {
      const saved = await setup.read();
      if (saved) {
        const { path: original, ...visible } = saved;
        report.setup = {
          ...visible,
          label: original ? path.basename(original) : "Invented demo",
        };
      }
    } catch {
      report.setupWarning =
        "Setup history could not be read; the existing file is preserved. Choose your source again.";
    }
    try {
      const disk = await fs.statfs(directory);
      report.free_bytes = disk.bavail * disk.bsize;
    } catch {}
    return report;
  });
  ipcMain.handle("ukinfo:manage-dataset", (event, { selection, action } = {}) =>
    select(async () => {
      const window = trusted(event);
      if (!ownsCatalogue)
        throw new Error(
          "Archive management requires an app-owned local catalogue.",
        );
      const item = catalogueSelections.get(selection);
      if (!item || !["remove", "index"].includes(action))
        throw new Error("Refresh the catalogue and choose an edition.");
      if (action === "remove") {
        const result = await dialog.showMessageBox(window, {
          type: "warning",
          title: "Remove imported edition?",
          message: `Remove ${item.manifest.title}?`,
          detail: `${item.entries.toLocaleString()} imported entries will be removed from this catalogue. Original discs, research lists, saved notes and other editions are kept. Saved links to this import will stop opening. To search it again you must reimport its source. Shared dictionaries remain for reuse.`,
          buttons: ["Keep edition", "Remove edition"],
          defaultId: 0,
          cancelId: 0,
          noLink: true,
        });
        if (result.response !== 1) return { canceled: true };
      }
      const abort = new AbortController();
      let polling = false;
      const timer = setInterval(async () => {
        if (polling) return;
        polling = true;
        try {
          const state = await controlRequest("/progress");
          if (!window.isDestroyed())
            window.webContents.send("ukinfo:catalogue-progress", state);
        } catch {
        } finally {
          polling = false;
        }
      }, 500);
      const job = {
        cancel: () => {
          abort.abort();
          controlRequest("/cancel", {}).catch(() => {});
        },
        done: controlRequest(
          action === "remove" ? "/remove-dataset" : "/index-dataset",
          { key: item.key, id: item.id },
          abort.signal,
        ),
      };
      active = job;
      try {
        return await job.done;
      } finally {
        clearInterval(timer);
        if (active === job) active = null;
      }
    }),
  );
  ipcMain.handle("ukinfo:resume-setup", (event) =>
    select(async () => {
      trusted(event);
      const previous = await setup.read();
      if (!previous) throw new Error("Choose your original source first.");
      return prepareSource(previous.path, previous.kind, previous.limit);
    }),
  );
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
    return prepareSource(result.filePaths[0], media ? "media" : "folder", null);
  }
  async function prepareSource(file, kind, limit = null) {
    if (!command || !control)
      throw new Error("The desktop importer is not installed.");
    await saveSetup({ kind, path: file, status: "preparing", limit });
    await clearSelections();

    let temporary = null,
      directory = file,
      source;
    try {
      if (kind === "media") {
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
      } else
        source = await identify(command, kind === "demo" ? null : directory);
    } catch (error) {
      if (temporary) await fs.rm(temporary, { recursive: true, force: true });
      log(error.message + "\n");
      await saveSetup({ kind, path: file, status: "stopped", limit });
      throw new Error(
        kind === "media"
          ? error.message
          : "Unable to verify that folder. Check that the required database files were extracted; see server.log for details.",
      );
    }
    const selection = randomUUID();
    const origin = {
      kind,
      path: file,
      edition: source.edition,
      title: source.title,
      limit,
    };
    selections.set(selection, {
      directory: kind === "demo" ? null : directory,
      temporary,
      origin,
    });
    await saveSetup({ ...origin, status: "ready" });
    return {
      selection,
      label: file ? path.basename(file) : "Invented demo",
      limit,
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
      return prepareSource(null, "demo");
    }),
  );
  ipcMain.handle("ukinfo:import", (event, { selection, limit } = {}) =>
    select(async () => {
      trusted(event);
      if (active)
        throw new Error("An import or source selection is already running.");
      if (!command || !control || !selections.has(selection))
        throw new Error("Choose a source folder first.");
      const chosen = selections.get(selection);
      await saveSetup({ ...chosen.origin, limit, status: "running" });
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
        const result = await job.done;
        await saveSetup({ ...chosen.origin, limit, status: "complete" });
        return result;
      } catch (error) {
        await saveSetup({ ...chosen.origin, limit, status: "stopped" });
        throw error;
      } finally {
        if (active === job) active = null;
      }
    }),
  );
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
