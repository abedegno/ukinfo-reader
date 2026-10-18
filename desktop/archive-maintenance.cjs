"use strict";
const path = require("node:path");
const { spawn } = require("node:child_process");
const { copyArchive, readManifest } = require("./archive-backup.cjs");
const { selectProfile, validateProfile } = require("./profile-location.cjs");
async function holdLock(executable, directory, abort) {
  let released = false;
  const child = spawn(
    executable,
    ["lock-archive", "--data-dir", path.join(directory, "catalogue")],
    { stdio: ["pipe", "pipe", "ignore"] },
  );
  child.stdin.on("error", () => {});
  child.once("exit", () => {
    if (!released) abort?.abort();
  });
  const exited = new Promise((resolve) => {
    child.once("exit", resolve);
    child.once("error", resolve);
  });
  try {
    await new Promise((resolve, reject) => {
      const timer = setTimeout(
        () => reject(new Error("Archive ownership check timed out.")),
        5000,
      );
      const fail = () => {
        clearTimeout(timer);
        reject(new Error("Another process owns the archive; files preserved."));
      };
      child.once("error", fail);
      child.once("exit", fail);
      let text = "";
      child.stdout.on("data", (chunk) => {
        text += chunk;
        if (text.length > 1024) return fail();
        if (!text.includes("\n")) return;
        try {
          if (JSON.parse(text.split("\n")[0]).locked !== true) return fail();
          clearTimeout(timer);
          resolve();
        } catch {
          fail();
        }
      });
    });
    return async () => {
      released = true;
      child.stdin.end();
      await exited;
    };
  } catch (e) {
    child.kill();
    await exited;
    throw e;
  }
}
function installMaintenance({
  app,
  ipcMain,
  dialog,
  getWindow,
  getServer,
  directory,
  profileRoot,
  executable,
  ownsCatalogue,
  stop,
  restart,
}) {
  let active = false,
    abort;
  const progress = (state) => {
    const w = getWindow();
    if (w && !w.isDestroyed())
      w.webContents.send("ukinfo:archive-progress", state);
  };
  async function choose(title) {
    const options = { title, properties: ["openDirectory", "createDirectory"] },
      w = getWindow();
    const result = await dialog.showOpenDialog(
      ...(w ? [w, options] : [options]),
    );
    return result.canceled ? null : result.filePaths[0];
  }
  async function savedOnly() {
    const w = getWindow();
    if (!w || w.isDestroyed()) return true;
    const dirty = await w.webContents
      .executeJavaScript(
        "Boolean(window.hasUnsavedResearch && window.hasUnsavedResearch())",
      )
      .catch(() => true);
    if (!dirty) return true;
    await dialog.showMessageBox(w, {
      type: "question",
      message: "Save your research notes first",
      detail:
        "Return to Research lists and save your notes before copying or switching archives.",
      buttons: ["Return to notes"],
      defaultId: 0,
      cancelId: 0,
    });
    return false;
  }
  async function run(action) {
    if (!ownsCatalogue)
      throw new Error(
        "Archive backup and switching require an app-owned local database.",
      );
    if (active) throw new Error("An archive operation is already running.");
    active = true;
    abort = new AbortController();
    let stopped = false,
      unlock;
    try {
      if (!(await savedOnly())) return { canceled: true };
      if (action === "open") {
        const target = await choose("Open an existing UK-Info archive folder");
        if (!target) return { canceled: true };
        validateProfile(target);
        await stop();
        stopped = true;
        selectProfile(profileRoot, target);
        return { restarting: true };
      }
      let source = directory;
      if (action === "restore") {
        source = await choose("Choose a UK-Info backup folder");
        if (!source) return { canceled: true };
        await readManifest(source);
      } else if (action !== "backup")
        throw new Error("Unknown archive action.");
      const parent = await choose(
        action === "backup"
          ? "Choose where to create a private backup folder"
          : "Choose where to create a restored archive",
      );
      if (!parent) return { canceled: true };
      const w = getWindow();
      const confirmation = await dialog.showMessageBox(
        ...(w
          ? [
              w,
              {
                type: "question",
                message:
                  action === "backup"
                    ? "Back up and restart the app?"
                    : "Restore into a separate archive and restart?",
                detail:
                  action === "backup"
                    ? "The database will stop while files are copied and verified. The backup contains private directory records, saved notes and local database credentials. Keep it private. Original media is not included."
                    : "Only restore backups you trust. The backup must match this operating system and CPU. Your current archive is kept; the app will open a new restored copy.",
                buttons: [
                  "Cancel",
                  action === "backup" ? "Create backup" : "Restore backup",
                ],
                defaultId: 0,
                cancelId: 0,
              },
            ]
          : [
              {
                type: "question",
                message: "Restore this trusted backup into a separate archive?",
                buttons: ["Cancel", "Restore backup"],
                defaultId: 0,
                cancelId: 0,
              },
            ]),
      );
      if (confirmation.response !== 1) return { canceled: true };
      progress({ phase: "stopping", done: 0, total: 0 });
      await stop();
      stopped = true;
      if (action === "backup")
        unlock = await holdLock(executable, directory, abort);
      const result = await copyArchive({
        source,
        parent,
        mode: action,
        appVersion: app.getVersion(),
        signal: abort.signal,
        onProgress: progress,
      });
      if (action === "restore") selectProfile(profileRoot, result);
      await dialog.showMessageBox({
        type: "info",
        message:
          action === "backup" ? "Backup verified" : "Restored archive verified",
        detail:
          action === "backup"
            ? "The app will restart with your original archive. Keep the backup folder private and intact."
            : "The app will restart with the restored archive. Your previous archive is unchanged. Use Open archive to switch back.",
        buttons: ["Restart app"],
      });
      return { restarting: true };
    } catch (e) {
      if (stopped)
        await dialog.showMessageBox({
          type: "error",
          message: abort.signal.aborted
            ? "Archive copy canceled"
            : "Archive copy stopped",
          detail: `${abort.signal.aborted ? "No completed copy was created." : e.message} Existing archives are preserved. The app will restart.`,
          buttons: ["Restart app"],
        });
      throw new Error(
        abort.signal.aborted ? "Archive copy canceled." : e.message,
      );
    } finally {
      await unlock?.();
      active = false;
      abort = null;
      progress({ phase: "idle", done: 0, total: 0 });
      if (stopped) restart();
    }
  }
  function trusted(event) {
    const w = getWindow();
    if (
      !w ||
      event.sender !== w.webContents ||
      event.senderFrame !== w.webContents.mainFrame ||
      !event.senderFrame ||
      new URL(event.senderFrame.url).origin !== getServer()?.address
    )
      throw new Error("Archive window required.");
  }
  ipcMain.handle("ukinfo:archive-maintenance", (event, action) => {
    trusted(event);
    return run(action);
  });
  ipcMain.handle("ukinfo:archive-cancel", (event) => {
    trusted(event);
    abort?.abort();
  });
  return {
    run,
    get active() {
      return active;
    },
  };
}
module.exports = { installMaintenance, holdLock };
