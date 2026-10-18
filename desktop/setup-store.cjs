"use strict";
const fs = require("node:fs/promises");
const path = require("node:path");
function createSetupStore(directory) {
  const filename = path.join(directory, "setup-state.json");
  let queue = Promise.resolve();
  const validate = (value) => {
    if (
      value?.version !== 1 ||
      !["media", "folder", "demo"].includes(value.kind) ||
      !["preparing", "ready", "running", "complete", "stopped"].includes(
        value.status,
      ) ||
      (value.kind !== "demo" &&
        (typeof value.path !== "string" ||
          value.path.length > 8192 ||
          !path.isAbsolute(value.path))) ||
      (value.limit != null &&
        (!Number.isSafeInteger(value.limit) ||
          value.limit < 1 ||
          value.limit > 1000000))
    )
      throw new Error(
        "Invalid setup history. Existing setup-state.json has been preserved.",
      );
    return {
      version: 1,
      kind: value.kind,
      path: value.kind === "demo" ? null : value.path,
      status: value.status,
      limit: value.limit ?? null,
      edition:
        typeof value.edition === "string" ? value.edition.slice(0, 256) : "",
      title: typeof value.title === "string" ? value.title.slice(0, 512) : "",
      updated:
        typeof value.updated === "string" ? value.updated.slice(0, 64) : "",
    };
  };
  async function read() {
    try {
      if ((await fs.stat(filename)).size > 16384)
        throw new Error("Setup history is oversized; existing file preserved.");
      return validate(JSON.parse(await fs.readFile(filename, "utf8")));
    } catch (error) {
      if (error.code === "ENOENT") return null;
      throw error;
    }
  }
  return {
    read,
    write(value) {
      const pending = queue.then(async () => {
        await read(); // preserve malformed existing files
        const next = validate({
          ...value,
          version: 1,
          updated: new Date().toISOString(),
        });
        await fs.mkdir(directory, { recursive: true, mode: 0o700 });
        const temporary = filename + ".tmp";
        try {
          const handle = await fs.open(temporary, "w", 0o600);
          try {
            await handle.writeFile(JSON.stringify(next));
            await handle.sync();
          } finally {
            await handle.close();
          }
          await fs.rename(temporary, filename);
        } finally {
          await fs.rm(temporary, { force: true });
        }
        return next;
      });
      queue = pending.catch(() => {});
      return pending;
    },
  };
}
module.exports = { createSetupStore };
