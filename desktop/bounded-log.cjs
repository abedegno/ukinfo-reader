"use strict";
const fs = require("node:fs/promises");
async function createBoundedLog(filename, max = 8 * 1024 * 1024, keep = 3) {
  let file = await fs.open(filename, "a+", 0o600),
    size = (await file.stat()).size,
    queue = Promise.resolve(),
    pending = 0;
  async function rotate() {
    await file.close();
    for (let n = keep; n >= 1; n--) {
      await fs.rm(filename + "." + n, { force: true });
      try {
        await fs.rename(
          n === 1 ? filename : filename + "." + (n - 1),
          filename + "." + n,
        );
      } catch (e) {
        if (e.code !== "ENOENT") throw e;
      }
    }
    file = await fs.open(filename, "a+", 0o600);
    size = 0;
  }
  if (size > max) {
    const tail = Buffer.alloc(max);
    await file.read(tail, 0, max, size - max);
    await file.truncate(0);
    await file.writeFile(tail);
    size = max;
    await rotate();
  }
  return {
    write(text) {
      const bytes = Buffer.from(text);
      if (pending > max) return;
      const chunk =
        bytes.length > max ? bytes.subarray(bytes.length - max) : bytes;
      pending += chunk.length;
      queue = queue
        .then(async () => {
          try {
            if (size + chunk.length > max) await rotate();
            await file.writeFile(chunk);
            size += chunk.length;
          } finally {
            pending -= chunk.length;
          }
        })
        .catch(() => {
          /* A full/unavailable volume must not crash the app while it reports the failure. */
        });
    },
    async end() {
      await queue;
      await file.close();
    },
  };
}
module.exports = { createBoundedLog };
