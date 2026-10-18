"use strict";
const fs = require("node:fs/promises");
const path = require("node:path");
const model = require("./research-model.cjs");
function createResearchStore(directory) {
  const filename = path.join(directory, "research-lists.json");
  let queue = Promise.resolve();
  async function read() {
    try {
      const stat = await fs.stat(filename);
      if (stat.size > model.MAX_BYTES)
        throw new Error("Research file exceeds the supported size.");
      return model.parse(await fs.readFile(filename, "utf8"));
    } catch (error) {
      if (error.code === "ENOENT") return model.empty();
      throw new Error(
        "Research lists could not be read. The existing file has been preserved; restore a JSON backup or inspect research-lists.json.",
      );
    }
  }
  return {
    read,
    write(value, expected) {
      const pending = queue.then(async () => {
        const next = model.validate(value);
        const current = await read();
        if (current.revision !== expected || next.revision !== expected + 1)
          throw new Error(
            "Research lists changed in another window. Reload lists before saving.",
          );
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
function installResearch({ ipcMain, getWindow, origin, directory }) {
  const store = createResearchStore(directory);
  function trusted(event) {
    const window = getWindow();
    if (
      !window ||
      event.sender !== window.webContents ||
      !event.senderFrame ||
      new URL(event.senderFrame.url).origin !== origin ||
      event.senderFrame !== window.webContents.mainFrame
    )
      throw new Error("Archive window required.");
  }
  ipcMain.handle("ukinfo:research-read", (event) => {
    trusted(event);
    return store.read();
  });
  ipcMain.handle("ukinfo:research-write", (event, { value, revision } = {}) => {
    trusted(event);
    return store.write(value, revision);
  });
}
module.exports = { createResearchStore, installResearch };
