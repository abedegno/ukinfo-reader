"use strict";
const { spawn } = require("node:child_process");

// Independent of Electron so process ownership and failed startup can be tested.
async function startServer(
  executable,
  args,
  { env = process.env, timeout = 120000, log = () => {} } = {},
) {
  const child = spawn(executable, args, {
    env,
    stdio: ["pipe", "pipe", "pipe"],
    windowsHide: true,
  });
  let stopped = false;
  let exited = false;
  const exit = new Promise((resolve) =>
    child.once("exit", () => {
      exited = true;
      resolve();
    }),
  );
  child.stderr.on("data", (chunk) => log(chunk.toString()));
  const stop = async () => {
    if (stopped) return exit;
    stopped = true;
    child.stdin.end(); // Go supervises EOF, including unexpected Electron death.
    let timer;
    await Promise.race([
      exit,
      new Promise((resolve) => {
        timer = setTimeout(() => {
          child.kill("SIGKILL");
          resolve();
        }, 22000);
      }),
    ]);
    clearTimeout(timer);
  };
  try {
    let control = null;
    const address = await new Promise((resolve, reject) => {
      let text = "";
      const timer = setTimeout(
        () => reject(new Error("Archive server startup timed out.")),
        timeout,
      );
      const done = (error, value) => {
        clearTimeout(timer);
        error ? reject(error) : resolve(value);
      };
      child.once("error", (error) => done(error));
      child.once("exit", (code) =>
        done(new Error(`Archive server exited during startup (${code}).`)),
      );
      child.stdout.on("data", (chunk) => {
        text += chunk.toString();
        if (text.length > 16384)
          return done(new Error("Invalid server startup output."));
        if (!text.includes("\n")) return;
        try {
          const data = JSON.parse(text.split("\n")[0]);
          const url = new URL(data.url);
          if (
            url.protocol !== "http:" ||
            url.hostname !== "127.0.0.1" ||
            !url.port ||
            url.username ||
            url.password ||
            url.pathname !== "/"
          )
            throw new Error("Invalid local server address.");
          if (data.control_url || data.control_token) {
            const c = new URL(data.control_url);
            if (
              c.protocol !== "http:" ||
              c.hostname !== "127.0.0.1" ||
              !c.port ||
              c.pathname !== "/" ||
              c.username ||
              c.password ||
              !/^[a-f0-9]{64}$/.test(data.control_token)
            )
              throw new Error("Invalid desktop control capability.");
            control = { url: c.origin, token: data.control_token };
          }
          done(null, url.origin);
        } catch (error) {
          done(error);
        }
      });
    });
    const response = await fetch(`${address}/api/health`, {
      signal: AbortSignal.timeout(5000),
    });
    if (!response.ok) throw new Error("Archive database is unavailable.");
    if (exited)
      throw new Error("Archive server stopped before the window opened.");
    return { address, stop, child, exit, control };
  } catch (error) {
    // spawn errors do not emit exit; avoid waiting on a process that never existed.
    if (child.pid) await stop();
    throw error;
  }
}
module.exports = { startServer };
