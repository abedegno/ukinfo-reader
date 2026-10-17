"use strict";
const { app, BrowserWindow, dialog, ipcMain, shell } = require("electron");
const fs = require("node:fs");
const path = require("node:path");
const { startServer } = require("./supervisor.cjs");
const { installImporter } = require("./import-ui.cjs");

// An explicit data directory makes an unpacked build portable, including imports.
// Run only one app against a given directory; PostgreSQL owns its physical cluster.
if (process.env.UKINFO_DATA_DIR)
  app.setPath("userData", path.resolve(process.env.UKINFO_DATA_DIR));
let server,
  window,
  importer,
  quitting = false;
if (!app.requestSingleInstanceLock()) {
  app.quit();
} else {
  app.on("second-instance", () => {
    if (window) {
      if (window.isMinimized()) window.restore();
      window.focus();
    }
  });
  app.whenReady().then(async () => {
    try {
      const runtime = app.isPackaged
        ? path.join(process.resourcesPath, "runtime")
        : path.resolve(__dirname, "../build/desktop");
      const executable =
        process.env.UKINFO_SERVER || path.join(runtime, "ukinfo-server");
      const args = [
        "serve",
        "--listen",
        "127.0.0.1:0",
        "--parent-stdin",
        "--desktop-control",
      ];
      const env = { ...process.env };
      if (process.env.UKINFO_DESKTOP_DATABASE_URL)
        env.DATABASE_URL = process.env.UKINFO_DESKTOP_DATABASE_URL;
      else {
        delete env.DATABASE_URL;
        args.push(
          "--data-dir",
          path.join(app.getPath("userData"), "catalogue"),
          "--pg-bin",
          process.env.UKINFO_PG_BIN || path.join(runtime, "pg/bin"),
        );
      }
      fs.mkdirSync(app.getPath("userData"), { recursive: true, mode: 0o700 });
      const log = fs.createWriteStream(
        path.join(app.getPath("userData"), "server.log"),
        { flags: "a", mode: 0o600 },
      );
      server = await startServer(executable, args, {
        env,
        log: (text) => log.write(text),
      });
      server.exit.then(() => {
        if (!quitting) {
          dialog.showErrorBox(
            "Archive server stopped",
            "Close the app and inspect server.log before restarting.",
          );
          app.quit();
        }
        log.end();
      });
      window = new BrowserWindow({
        width: 1240,
        height: 900,
        minWidth: 650,
        minHeight: 600,
        backgroundColor: "#f6f5ef",
        show: false,
        webPreferences: {
          preload: path.join(__dirname, "preload.cjs"),
          contextIsolation: true,
          sandbox: true,
          nodeIntegration: false,
          devTools: !app.isPackaged,
        },
      });
      window.webContents.setWindowOpenHandler(({ url }) => {
        if (url === "https://github.com/abedegno/ukinfo-reader")
          shell.openExternal(url).catch(() => {});
        return { action: "deny" };
      });
      window.webContents.on("will-navigate", (event, target) => {
        if (new URL(target).origin !== server.address) event.preventDefault();
      });
      window.webContents.session.setPermissionRequestHandler(
        (_contents, _permission, callback) => callback(false),
      );
      importer = installImporter({
        ipcMain,
        dialog,
        getWindow: () => window,
        server,
        runtime,
        isPackaged: app.isPackaged,
        log: (text) => log.write(text),
      });
      await window.loadURL(server.address);
      window.show();
    } catch (error) {
      dialog.showErrorBox("Unable to open the archive", error.message);
      app.quit();
    }
  });
  app.on("window-all-closed", () => app.quit());
  app.on("before-quit", (event) => {
    if (server && !quitting) {
      event.preventDefault();
      quitting = true;
      (async () => {
        if (importer) await importer.cancel();
        await server.stop();
      })().finally(() => app.quit());
    }
  });
}
