"use strict";
const fs = require("node:fs/promises");
const path = require("node:path");
const os = require("node:os");
const { randomUUID } = require("node:crypto");
const { createSetupStore } = require("./setup-store.cjs");
const editions = new Set([
  "synthetic-demo",
  "se-pcf78-1998-01",
  "se4-pcf94-1999-04",
  "se2004-pcp218-2004-07",
  "se2005-pcf175-2005-06",
]);
const count = (n) => (Number.isSafeInteger(n) && n >= 0 ? n : null);
const version = (s) =>
  typeof s === "string" && /^\d+(?:\.\d+){0,3}(?:-[a-z0-9.]+)?$/i.test(s)
    ? s
    : null;
async function readJSON(filename) {
  if ((await fs.stat(filename)).size > 65536)
    throw new Error("Metadata too large");
  return JSON.parse(await fs.readFile(filename, "utf8"));
}
function summarizeCatalogue(value) {
  return {
    status: "available",
    database_bytes: count(value.database_bytes),
    shared_bytes: count(value.shared_bytes),
    geography_bytes: count(value.geography_bytes),
    postcode_reference_installed: Boolean(value.geography),
    operation_running: value.state?.running === true,
    operation_error_present: Boolean(value.state?.error),
    datasets: (Array.isArray(value.datasets) ? value.datasets : [])
      .slice(0, 100)
      .map((d) => ({
        edition: editions.has(d.manifest?.edition)
          ? d.manifest.edition
          : "other",
        sample: d.manifest?.sample === true,
        entries: count(d.entries),
        bytes: count(d.bytes),
        address_ready: d.address_ready === true,
      })),
  };
}
async function collectDiagnostics({
  app,
  directory,
  runtime,
  getServer,
  getLifecycle,
  ownsCatalogue,
  env = process.env,
}) {
  // Build a new allowlisted object. Never spread metadata, errors, URLs or environment.
  const report = {
    schema: "ukinfo-desktop-diagnostics",
    version: 1,
    generated_at: new Date().toISOString(),
    app: {
      version: version(
        app.isPackaged ? app.getVersion() : require("./package.json").version,
      ),
      packaged: app.isPackaged,
      commit: null,
      source_modified: null,
    },
    system: {
      platform: process.platform,
      architecture: process.arch,
      release: version(os.release().match(/^\d+(?:\.\d+){0,3}/)?.[0]),
    },
    runtimes: {
      electron: version(process.versions.electron),
      chromium: version(process.versions.chrome),
      node: version(process.versions.node),
    },
    configuration: {
      database: ownsCatalogue ? "app-owned" : "external",
      custom_data_directory: Boolean(env.UKINFO_DATA_DIR),
      custom_server: Boolean(env.UKINFO_SERVER),
      custom_postgres: Boolean(env.UKINFO_PG_BIN),
    },
    lifecycle: [
      "starting",
      "running",
      "startup-failed",
      "server-stopped",
    ].includes(getLifecycle())
      ? getLifecycle()
      : "unknown",
    health: "not-running",
    free_disk_gib: null,
    setup: { status: "unavailable" },
    catalogue: {
      status: ownsCatalogue ? "unavailable" : "external-not-inspected",
    },
  };
  try {
    const build = await readJSON(path.join(__dirname, "build-info.json"));
    if (/^[a-f0-9]{40}$/.test(build.commit)) report.app.commit = build.commit;
    if (typeof build.source_modified === "boolean")
      report.app.source_modified = build.source_modified;
  } catch {}
  for (const [name, file] of [
    ["postgres", "pg/BUILD.json"],
    ["unshield", "cabinet-notices/BUILD.json"],
  ]) {
    try {
      report.runtimes[name] = version(
        (await readJSON(path.join(runtime, file))).version,
      );
    } catch {
      report.runtimes[name] = null;
    }
  }
  try {
    const disk = await fs.statfs(directory);
    report.free_disk_gib = Math.floor((disk.bavail * disk.bsize) / 2 ** 30);
  } catch {}
  try {
    const saved = await createSetupStore(directory).read();
    report.setup = saved
      ? {
          status: saved.status,
          kind: saved.kind,
          sample_limit: count(saved.limit),
        }
      : { status: "none" };
  } catch {}
  const server = getServer();
  if (server) {
    try {
      const response = await fetch(`${server.address}/api/health`, {
        signal: AbortSignal.timeout(3000),
      });
      report.health = response.ok ? "healthy" : "unhealthy";
      await response.body?.cancel();
    } catch {
      report.health = "unreachable";
    }
    if (ownsCatalogue && server.control) {
      try {
        const response = await fetch(`${server.control.url}/catalogue`, {
          headers: { Authorization: `Bearer ${server.control.token}` },
          signal: AbortSignal.timeout(5000),
        });
        if (response.ok)
          report.catalogue = summarizeCatalogue(await response.json());
        else await response.body?.cancel();
      } catch {}
    }
  }
  return report;
}
function installDiagnostics({ ipcMain, dialog, getWindow, ...options }) {
  let busy = false;
  async function exportReport() {
    if (busy) return { busy: true };
    busy = true;
    try {
      const window = getWindow();
      const settings = {
        title: "Export diagnostics for an issue",
        message:
          "Includes versions, health and archive status. Excludes records, notes, searches, logs, credentials and file paths. Review the JSON before attaching it to an issue.",
        defaultPath: `ukinfo-diagnostics-${new Date().toISOString().slice(0, 10)}.json`,
        filters: [{ name: "Diagnostics JSON", extensions: ["json"] }],
      };
      const result = await dialog.showSaveDialog(
        ...(window ? [window, settings] : [settings]),
      );
      if (result.canceled || !result.filePath) return { canceled: true };
      const report = await collectDiagnostics(options);
      const temporary = result.filePath + "." + randomUUID() + ".tmp";
      try {
        await fs.writeFile(temporary, JSON.stringify(report, null, 2) + "\n", {
          mode: 0o600,
          flag: "wx",
        });
        await fs.rename(temporary, result.filePath);
      } finally {
        await fs.rm(temporary, { force: true });
      }
      return { canceled: false };
    } catch {
      throw new Error(
        "Unable to save diagnostics. Choose a writable location and try again.",
      );
    } finally {
      busy = false;
    }
  }
  ipcMain.handle("ukinfo:export-diagnostics", (event) => {
    const window = getWindow();
    if (
      !window ||
      event.sender !== window.webContents ||
      event.senderFrame !== window.webContents.mainFrame ||
      !event.senderFrame ||
      new URL(event.senderFrame.url).origin !== options.getServer()?.address
    )
      throw new Error("Archive window required.");
    return exportReport();
  });
  return { exportReport };
}
module.exports = { installDiagnostics, collectDiagnostics, summarizeCatalogue };
