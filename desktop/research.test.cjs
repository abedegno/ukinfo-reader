"use strict";
const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const os = require("node:os");
const path = require("node:path");
const model = require("./research-model.cjs");
const {
  createResearchStore,
  installResearch,
} = require("./research-store.cjs");
function fixture() {
  const data = model.empty();
  data.revision = 1;
  data.lists = [
    {
      id: "list-1",
      name: "Invented research",
      entries: [
        {
          id: "entry-1",
          mode: "record",
          scope: "edition",
          ref: { dataset: "synthetic-demo", id: 1, source_id: "invented:0" },
          summary: {
            title: "Invented name",
            address: "Invented Street",
            postcode: "ZZ11ZZ",
            edition: "Invented edition",
            listing: "Invented",
            phone: "",
            citation: "Invented citation",
            warnings: "",
          },
          note: "=malicious spreadsheet formula",
          created: "2026-10-18T00:00:00.000Z",
          updated: "2026-10-18T00:00:00.000Z",
        },
      ],
    },
  ];
  return data;
}
test("research schema limits, safe CSV and unknown-field removal", () => {
  const input = fixture();
  input.lists[0].entries[0].untrustedHTML = "<script>";
  const output = model.parse(JSON.stringify(input));
  assert.equal(output.lists[0].entries[0].untrustedHTML, undefined);
  assert.match(model.csv(output.lists[0]), /"'=malicious spreadsheet formula"/);
  for (const change of [
    (d) => (d.version = 2),
    (d) => (d.lists[0].entries[0].ref.id = 0),
    (d) => d.lists[0].entries.push(d.lists[0].entries[0]),
    (d) => (d.lists[0].entries[0].note = "a".repeat(4001)),
  ]) {
    const bad = fixture();
    change(bad);
    assert.throws(() => model.validate(bad), /Invalid research/);
  }
  assert.throws(
    () => model.parse(" ".repeat(model.MAX_BYTES + 1)),
    /Invalid research/,
  );
});
test("native lists survive a new store, reject stale writes and preserve corrupt files", async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), "ukinfo-research-test-"),
  );
  try {
    const store = createResearchStore(directory);
    assert.deepEqual(await store.read(), model.empty());
    const next = fixture();
    await store.write(next, 0);
    assert.deepEqual(await createResearchStore(directory).read(), next);
    if (process.platform !== "win32")
      assert.equal(
        (await fs.stat(path.join(directory, "research-lists.json"))).mode &
          0o777,
        0o600,
      );
    const newer = structuredClone(next);
    newer.revision = 2;
    newer.lists[0].entries[0].note = "Saved note";
    const results = await Promise.allSettled([
      store.write(newer, 1),
      store.write(newer, 1),
    ]);
    assert.equal(results.filter((r) => r.status === "fulfilled").length, 1);
    assert.equal((await store.read()).revision, 2);
    await fs.writeFile(
      path.join(directory, "research-lists.json"),
      "broken research data",
    );
    await assert.rejects(store.read(), /preserved/);
    await assert.rejects(store.write(newer, 1), /preserved/);
    assert.equal(
      await fs.readFile(path.join(directory, "research-lists.json"), "utf8"),
      "broken research data",
    );
  } finally {
    await fs.rm(directory, { recursive: true, force: true });
  }
});
test("research IPC accepts only the owning window's main frame", async () => {
  const handlers = new Map(),
    frame = { url: "http://127.0.0.1:1234/" },
    contents = { mainFrame: frame };
  const window = { webContents: contents };
  installResearch({
    ipcMain: { handle: (name, handler) => handlers.set(name, handler) },
    getWindow: () => window,
    origin: "http://127.0.0.1:1234",
    directory: "unused",
  });
  assert.throws(
    () =>
      handlers.get("ukinfo:research-read")({
        sender: contents,
        senderFrame: { url: "http://127.0.0.1:1234/" },
      }),
    /Archive window/,
  );
  assert.throws(
    () =>
      handlers.get("ukinfo:research-read")({ sender: {}, senderFrame: frame }),
    /Archive window/,
  );
});
