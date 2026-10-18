const { test } = require("node:test"),
  assert = require("node:assert/strict"),
  fs = require("node:fs/promises"),
  os = require("node:os"),
  path = require("node:path");
const { createBoundedLog } = require("./bounded-log.cjs");
test("log retention is bounded across writes and restarts", async () => {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), "ukinfo-log-")),
    filename = path.join(directory, "server.log");
  try {
    await fs.writeFile(filename, "a".repeat(100));
    const log = await createBoundedLog(filename, 16, 2);
    for (let n = 0; n < 10; n++) log.write("event".repeat(5));
    await log.end();
    const files = await fs.readdir(directory);
    assert.equal(files.length, 3);
    for (const file of files)
      assert((await fs.stat(path.join(directory, file))).size <= 16);
    const reopened = await createBoundedLog(filename, 16, 2);
    reopened.write("new");
    await reopened.end();
    assert((await fs.readFile(filename, "utf8")).endsWith("new"));
  } finally {
    await fs.rm(directory, { recursive: true, force: true });
  }
});
