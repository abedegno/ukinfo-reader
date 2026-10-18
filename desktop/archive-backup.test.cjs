"use strict";
const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const os = require("node:os"),
  path = require("node:path");
const { copyArchive, readManifest } = require("./archive-backup.cjs");
const { readProfile, selectProfile } = require("./profile-location.cjs");
async function fixture(root) {
  const source = path.join(root, "original"),
    copies = path.join(root, "copies");
  await fs.mkdir(path.join(source, "catalogue/pg18/pg_tblspc"), {
    recursive: true,
  });
  await fs.mkdir(copies);
  await fs.writeFile(path.join(source, "catalogue/pg18/PG_VERSION"), "18\n");
  await fs.writeFile(path.join(source, "catalogue/password"), "a".repeat(64));
  await fs.writeFile(
    path.join(source, "research-lists.json"),
    '{"note":"invented saved work"}',
  );
  await fs.writeFile(
    path.join(source, "server.log"),
    "private log not part of backup",
  );
  return { source, copies };
}
test("verified cold backup/restore preserves notes, empty PG directories, credentials and the original profile", async () => {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), "ukinfo-backup-"));
  try {
    const { source, copies } = await fixture(root);
    const backup = await copyArchive({
      source,
      parent: copies,
      mode: "backup",
      appVersion: "0.3.4",
    });
    const manifest = await readManifest(backup);
    assert.equal(manifest.files.length, 3);
    assert.equal(
      manifest.directories.includes("catalogue/pg18/pg_tblspc"),
      true,
    );
    assert.equal(
      manifest.files.some((f) => f.name.endsWith(".log")),
      false,
    );
    const restored = await copyArchive({
      source: backup,
      parent: copies,
      mode: "restore",
    });
    for (const name of [
      "research-lists.json",
      "catalogue/password",
      "catalogue/pg18/PG_VERSION",
    ])
      assert.equal(
        await fs.readFile(path.join(restored, name), "utf8"),
        await fs.readFile(path.join(source, name), "utf8"),
      );
    assert.equal(
      (
        await fs.stat(path.join(restored, "catalogue/pg18/pg_tblspc"))
      ).isDirectory(),
      true,
    );
    if (process.platform !== "win32")
      assert.equal(
        (await fs.stat(path.join(restored, "research-lists.json"))).mode &
          0o777,
        0o600,
      );
    assert.equal(readProfile(source), source);
    selectProfile(source, restored);
    assert.equal(readProfile(source), restored);
    assert.throws(() => selectProfile(source, backup), /Restore this backup/);
    selectProfile(source, source);
    assert.equal(readProfile(source), await fs.realpath(source));
    const altered = (beforeCopy) => beforeCopy.replace("invented", "changed!");
    const noteFile = path.join(backup, "research-lists.json");
    await fs.writeFile(noteFile, altered(await fs.readFile(noteFile, "utf8")));
    await assert.rejects(
      copyArchive({ source: backup, parent: copies, mode: "restore" }),
      /checksum/,
    );
    assert.equal(
      (await fs.readdir(copies)).some((n) => n.endsWith(".partial")),
      false,
    );
    assert.equal(
      await fs.readFile(path.join(source, "research-lists.json"), "utf8"),
      '{"note":"invented saved work"}',
    );
  } finally {
    await fs.rm(root, { recursive: true, force: true });
  }
});
test("cold copies reject live databases, traversal, symlinks, incompatible backups and insufficient disk", async (t) => {
  const root = await fs.mkdtemp(
    path.join(os.tmpdir(), "ukinfo-backup-reject-"),
  );
  try {
    const { source, copies } = await fixture(root);
    await fs.writeFile(
      path.join(source, "catalogue/pg18/postmaster.pid"),
      "123",
    );
    await assert.rejects(
      copyArchive({ source, parent: copies, mode: "backup" }),
      /shutdown/,
    );
    await fs.rm(path.join(source, "catalogue/pg18/postmaster.pid"));
    const backup = await copyArchive({
      source,
      parent: copies,
      mode: "backup",
    });
    const filename = path.join(backup, "ukinfo-backup.json"),
      original = JSON.parse(await fs.readFile(filename, "utf8"));
    for (const mutate of [
      (d) => (d.files[0].name = "catalogue/../../escape"),
      (d) => (d.architecture = "incompatible"),
      (d) => d.directories.push("../outside"),
    ]) {
      const value = structuredClone(original);
      mutate(value);
      await fs.writeFile(filename, JSON.stringify(value));
      await assert.rejects(
        copyArchive({ source: backup, parent: copies, mode: "restore" }),
      );
    }
    await fs.writeFile(filename, JSON.stringify(original));
    if (process.platform !== "win32") {
      await fs.symlink(source, path.join(source, "catalogue/linked"));
      await assert.rejects(
        copyArchive({ source, parent: copies, mode: "backup" }),
        /symbolic links/,
      );
      await fs.rm(path.join(source, "catalogue/linked"));
    }
    t.mock.method(fs, "statfs", async () => ({ bavail: 1, bsize: 1 }));
    await assert.rejects(
      copyArchive({ source, parent: copies, mode: "backup" }),
      /free space/,
    );
    assert.equal(
      (await fs.readdir(copies)).some((n) => n.endsWith(".partial")),
      false,
    );
  } finally {
    await fs.rm(root, { recursive: true, force: true });
  }
});
test("canceling a copy cleans its staging directory and leaves the source untouched", async () => {
  const root = await fs.mkdtemp(
    path.join(os.tmpdir(), "ukinfo-backup-cancel-"),
  );
  try {
    const { source, copies } = await fixture(root),
      controller = new AbortController();
    await assert.rejects(
      copyArchive({
        source,
        parent: copies,
        mode: "backup",
        signal: controller.signal,
        onProgress: () => controller.abort(),
      }),
    );
    assert.deepEqual(await fs.readdir(copies), []);
    assert.equal(
      await fs.readFile(path.join(source, "catalogue/pg18/PG_VERSION"), "utf8"),
      "18\n",
    );
  } finally {
    await fs.rm(root, { recursive: true, force: true });
  }
});
