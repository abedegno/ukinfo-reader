"use strict";
const { spawn, execFile } = require("node:child_process");
const { promisify } = require("node:util");
const fs = require("node:fs");
const path = require("node:path");
const execute = promisify(execFile);

function importerCommand(runtime, isPackaged) {
  const bundled = path.join(runtime, "ukinfo-import");
  if (fs.existsSync(bundled)) return { executable: bundled, args: [] };
  if (isPackaged) return null;
  const root = path.resolve(__dirname, "..");
  const python =
    process.env.UKINFO_PYTHON ||
    path.join(
      root,
      ".venv",
      process.platform === "win32" ? "Scripts/python.exe" : "bin/python",
    );
  return fs.existsSync(python)
    ? { executable: python, args: ["-m", "ukinfo_reader.search_export"] }
    : null;
}
async function identify(command, directory) {
  const { stdout } = await execute(
    command.executable,
    [
      ...command.args,
      ...(directory === null ? ["--demo"] : ["--data", directory]),
      "--identify",
    ],
    { timeout: 120000, maxBuffer: 1 << 20, windowsHide: true },
  );
  const result = JSON.parse(stdout);
  if (
    typeof result.edition !== "string" ||
    typeof result.title !== "string" ||
    !Number.isSafeInteger(result.entries)
  )
    throw new Error("Invalid source identification.");
  return result;
}
function beginImport(command, directory, limit, control, onProgress, log) {
  const args = [
    ...command.args,
    ...(directory === null ? ["--demo"] : ["--data", directory]),
  ];
  if (limit != null) {
    if (!Number.isSafeInteger(limit) || limit < 1 || limit > 1000000)
      throw new Error("Sample size must be 1–1,000,000 entries.");
    args.push("--limit", String(limit));
  }
  const child = spawn(command.executable, args, {
    stdio: ["ignore", "pipe", "pipe"],
    windowsHide: true,
  });
  child.stderr.on("data", (chunk) => log(chunk.toString()));
  let exitCode;
  const exited = new Promise((resolve, reject) => {
    child.once("error", reject);
    child.once("exit", (code) => {
      exitCode = code;
      resolve();
    });
  });
  // Attach rejection handling immediately, including when spawn itself fails.
  const processResult = exited.then(
    () => ({ code: exitCode }),
    (error) => ({ error }),
  );
  const abort = new AbortController();
  const headers = { Authorization: `Bearer ${control.token}` };
  const stopChild = () => {
    if (child.exitCode === null && child.signalCode === null) {
      child.kill("SIGTERM");
      const timer = setTimeout(() => {
        if (child.exitCode === null && child.signalCode === null)
          child.kill("SIGKILL");
      }, 3000);
      timer.unref();
      child.once("exit", () => clearTimeout(timer));
    }
  };
  let polling = false;
  const timer = setInterval(async () => {
    if (polling) return;
    polling = true;
    try {
      const response = await fetch(`${control.url}/progress`, {
        headers,
        signal: AbortSignal.timeout(3000),
      });
      if (response.ok) onProgress(await response.json());
    } catch {
      /* A canceled stream or quitting app can close the control listener. */
    } finally {
      polling = false;
    }
  }, 300);
  const cancel = () => {
    abort.abort();
    stopChild();
    fetch(`${control.url}/cancel`, {
      method: "POST",
      headers,
      signal: AbortSignal.timeout(3000),
    }).catch(() => {});
  };
  const done = (async () => {
    try {
      const response = await fetch(`${control.url}/import`, {
        method: "POST",
        headers: { ...headers, "Content-Type": "application/x-ndjson" },
        body: child.stdout,
        duplex: "half",
        signal: abort.signal,
      });
      const data = await response.json();
      if (!response.ok) stopChild();
      const result = await processResult;
      if (result.error) throw result.error;
      if (!response.ok || result.code !== 0)
        throw new Error(data.error || "Source decoder failed. See server.log.");
      onProgress({
        running: false,
        progress: {
          phase: "complete",
          done: data.entries,
          total: data.entries,
        },
      });
      return data;
    } catch (error) {
      if (abort.signal.aborted)
        throw new Error("Import canceled; the dataset was not published.");
      throw error;
    } finally {
      clearInterval(timer);
      stopChild();
      await processResult;
    }
  })();
  return { done, cancel };
}
module.exports = { importerCommand, identify, beginImport };
