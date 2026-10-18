"use strict";
const fs = require("node:fs/promises");
const path = require("node:path");
const { randomUUID } = require("node:crypto");
function createResearchStore(directory, options = {}) {
  const model = options.model || require("./research-model.cjs");
  const filename = path.join(
    directory,
    options.filename || "research-lists.json",
  );
  const label = options.label || "Research lists";
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
        `${label} could not be read. The existing file has been preserved; restore a private backup or inspect ${path.basename(filename)}.`,
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
            `${label} changed in another window. Reload before saving.`,
          );

        if (!options.filename && next.version === 2) {
          try {
            const original = await fs.readFile(filename, "utf8");
            if (JSON.parse(original).version === 1) {
              const temporaryBackup = filename + ".v1-" + randomUUID() + ".tmp";
              try {
                const backup = await fs.open(temporaryBackup, "wx", 0o600);
                try {
                  await backup.writeFile(original);
                  await backup.sync();
                } finally {
                  await backup.close();
                }
                try {
                  await fs.link(
                    temporaryBackup,
                    path.join(directory, "research-lists.v1-backup.json"),
                  );
                } catch (error) {
                  if (error.code !== "EEXIST") throw error;
                }
              } finally {
                await fs.rm(temporaryBackup, { force: true });
              }
            }
          } catch (error) {
            if (error.code !== "ENOENT") throw error;
          }
        }
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
  const stores = {
    research: createResearchStore(directory),
    workbench: createResearchStore(directory, {
      model: require("./workbench-model.cjs"),
      filename: "workbench.json",
      label: "Research workspace",
    }),
  };
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
  for (const [name, store] of Object.entries(stores)) {
    ipcMain.handle(`ukinfo:${name}-read`, (event) => {
      trusted(event);
      return store.read();
    });
    ipcMain.handle(
      `ukinfo:${name}-write`,
      (event, { value, revision } = {}) => {
        trusted(event);
        return store.write(value, revision);
      },
    );
  }
}
module.exports = { createResearchStore, installResearch };
