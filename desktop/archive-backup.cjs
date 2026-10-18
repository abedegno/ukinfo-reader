"use strict";
const fs = require("node:fs/promises");
const path = require("node:path");
const { createHash, randomUUID } = require("node:crypto");
const ROOTS = new Set([
  "catalogue",
  "research-lists.json",
  "research-lists.v1-backup.json",
  "setup-state.json",
  "workbench.json",
  "research-drafts.json",
]);
const RESERVE = 5 * 2 ** 30;
const metadataName = "ukinfo-backup.json";
function validName(name) {
  return (
    typeof name === "string" &&
    name.length < 1024 &&
    !name.includes("\\") &&
    !name.includes("\0") &&
    !path.posix.isAbsolute(name) &&
    name.split("/").every((p) => p && p !== "." && p !== "..") &&
    ROOTS.has(name.split("/")[0])
  );
}
async function freeSpace(directory, required = 0) {
  const stat = await fs.statfs(directory);
  if (stat.bavail * stat.bsize < required + RESERVE)
    throw new Error(
      "Not enough free space. Keep at least 5 GiB free after the copy.",
    );
}
async function inventory(directory, relative = "", directories = []) {
  const files = [];
  for (const entry of await fs.readdir(path.join(directory, relative), {
    withFileTypes: true,
  })) {
    if (
      (!relative && !ROOTS.has(entry.name)) ||
      entry.name === "owner.lock" ||
      /^(server|postgres)\.log(?:\.\d+)?$/.test(entry.name)
    )
      continue;
    const name = relative ? relative + "/" + entry.name : entry.name;
    if (!validName(name)) throw new Error("Unsupported backup filename.");
    if (entry.isDirectory()) {
      directories.push(name);
      files.push(...(await inventory(directory, name, directories)));
    } else if (entry.isFile())
      files.push({
        name,
        bytes: (await fs.lstat(path.join(directory, name))).size,
      });
    else
      throw new Error(
        "External tablespaces, symbolic links and special files cannot be backed up by this app.",
      );
  }
  if (files.length > 100000) throw new Error("Backup contains too many files.");
  return files;
}
async function cold(directory) {
  try {
    await fs.lstat(path.join(directory, "catalogue/pg18/postmaster.pid"));
  } catch (e) {
    if (e.code === "ENOENT") return;
    throw e;
  }
  throw new Error(
    "Database shutdown is not confirmed. Backup stopped; existing files are preserved.",
  );
}
async function readManifest(directory) {
  const file = path.join(directory, metadataName),
    stat = await fs.lstat(file);
  if (!stat.isFile() || stat.size > 32 * 1024 * 1024)
    throw new Error("Invalid backup manifest.");
  const data = JSON.parse(await fs.readFile(file, "utf8"));
  if (
    data.format !== "ukinfo-cold-backup" ||
    data.version !== 1 ||
    data.postgres_major !== 18 ||
    data.platform !== process.platform ||
    data.architecture !== process.arch ||
    !Array.isArray(data.files) ||
    !data.files.length ||
    data.files.length > 100000 ||
    !Array.isArray(data.directories) ||
    data.directories.length > 100000
  )
    throw new Error(
      "Use a complete backup from the same operating system, CPU architecture and PostgreSQL major version.",
    );
  const directories = new Set();
  for (const name of data.directories) {
    if (!validName(name) || directories.has(name))
      throw new Error("Invalid backup directory inventory.");
    directories.add(name);
  }
  const names = new Set();
  for (const item of data.files) {
    if (
      !validName(item.name) ||
      names.has(item.name) ||
      directories.has(item.name) ||
      !Number.isSafeInteger(item.bytes) ||
      item.bytes < 0 ||
      !/^[a-f0-9]{64}$/.test(item.sha256)
    )
      throw new Error("Invalid backup file inventory.");
    names.add(item.name);
  }
  if (
    !names.has("catalogue/pg18/PG_VERSION") ||
    !names.has("catalogue/password") ||
    names.has("catalogue/pg18/postmaster.pid")
  )
    throw new Error(
      "Backup is missing required files or was not taken after database shutdown.",
    );
  return data;
}
async function copyFile(
  source,
  destination,
  item,
  { signal, progress = () => {} },
) {
  signal?.throwIfAborted();
  const sourceFile = path.join(source, item.name),
    stat = await fs.lstat(sourceFile);
  if (!stat.isFile() || stat.size !== item.bytes)
    throw new Error("Backup source changed or contains an unsafe file.");
  // Reject symlinked parents as well as leaves when restoring an edited backup.
  for (
    let parent = path.dirname(item.name);
    parent !== ".";
    parent = path.dirname(parent)
  ) {
    if (!(await fs.lstat(path.join(source, parent))).isDirectory())
      throw new Error("Unsafe backup directory.");
  }
  const input = await fs.open(sourceFile, "r");
  let output;
  try {
    const opened = await input.stat();
    if (!opened.isFile() || opened.ino !== stat.ino || opened.dev !== stat.dev)
      throw new Error("Backup source changed.");
    if (destination) {
      const target = path.join(destination, item.name);
      await fs.mkdir(path.dirname(target), { recursive: true, mode: 0o700 });
      output = await fs.open(target, "wx", 0o600);
    }
    const hash = createHash("sha256");
    let bytes = 0,
      checked = 0;
    for await (const chunk of input.createReadStream({
      autoClose: false,
      highWaterMark: 1024 * 1024,
    })) {
      signal?.throwIfAborted();
      if (destination && bytes >= checked) {
        await freeSpace(destination);
        checked = bytes + 16 * 1024 * 1024;
      }
      hash.update(chunk);
      if (output) await output.writeFile(chunk);
      bytes += chunk.length;
      progress(chunk.length);
    }
    const final = await input.stat();
    if (
      bytes !== item.bytes ||
      final.size !== opened.size ||
      final.mtimeMs !== opened.mtimeMs
    )
      throw new Error("Backup source changed while being read.");
    const sha256 = hash.digest("hex");
    if (item.sha256 && item.sha256 !== sha256)
      throw new Error(
        "Backup checksum mismatch. Existing archives have not been changed.",
      );
    if (output) await output.sync();
    return { name: item.name, bytes, sha256 };
  } finally {
    await output?.close();
    await input.close();
  }
}
async function copyArchive({
  source,
  parent,
  mode,
  appVersion,
  signal,
  onProgress = () => {},
}) {
  source = await fs.realpath(source);
  parent = await fs.realpath(parent);
  if (parent === source || parent.startsWith(source + path.sep))
    throw new Error("Choose a destination outside the source archive.");
  let manifest;
  if (mode === "backup") {
    await cold(source);
    const version = (
      await fs.readFile(path.join(source, "catalogue/pg18/PG_VERSION"), "utf8")
    ).trim();
    if (version !== "18") throw new Error("PostgreSQL 18 archive required.");
    const directories = [];
    manifest = {
      format: "ukinfo-cold-backup",
      version: 1,
      postgres_major: 18,
      platform: process.platform,
      architecture: process.arch,
      app_version: appVersion,
      created: new Date().toISOString(),
      files: await inventory(source, "", directories),
      directories,
    };
    if (!manifest.files.some((f) => f.name === "catalogue/password"))
      throw new Error("Archive credentials are missing; files preserved.");
  } else if (mode === "restore") manifest = await readManifest(source);
  else throw new Error("Unknown archive copy operation.");
  const total = manifest.files.reduce((n, f) => n + f.bytes, 0);
  if (!Number.isSafeInteger(total)) throw new Error("Invalid backup size.");
  await freeSpace(parent, total);
  const label = `UK-Info-${mode}-${new Date().toISOString().replace(/[:.]/g, "-")}-${randomUUID().slice(0, 8)}`;
  const staging = path.join(parent, "." + label + ".partial"),
    destination = path.join(parent, label);
  await fs.mkdir(staging, { mode: 0o700 });
  let done = 0;
  try {
    for (const name of manifest.directories)
      await fs.mkdir(path.join(staging, name), {
        recursive: true,
        mode: 0o700,
      });
    const files = [];
    for (const item of manifest.files)
      files.push(
        await copyFile(source, staging, item, {
          signal,
          progress: (bytes) => {
            done += bytes;
            onProgress({ phase: "copying", done, total });
          },
        }),
      );
    if (mode === "backup") {
      let verified = 0;
      for (const item of files)
        await copyFile(staging, null, item, {
          signal,
          progress: (bytes) => {
            verified += bytes;
            onProgress({ phase: "verifying", done: verified, total });
          },
        });
    }
    await cold(staging);
    if (
      (
        await fs.readFile(
          path.join(staging, "catalogue/pg18/PG_VERSION"),
          "utf8",
        )
      ).trim() !== "18"
    )
      throw new Error("Invalid restored database version.");
    const metadata =
      JSON.stringify(
        mode === "backup"
          ? { ...manifest, files }
          : {
              format: "ukinfo-profile",
              version: 1,
              platform: process.platform,
              architecture: process.arch,
              postgres_major: 18,
            },
        null,
        2,
      ) + "\n";
    if (Buffer.byteLength(metadata) > 32 * 1024 * 1024)
      throw new Error("Backup manifest exceeds the supported size.");
    await fs.writeFile(
      path.join(
        staging,
        mode === "backup" ? metadataName : "ukinfo-profile.json",
      ),
      metadata,
      { mode: 0o600, flag: "wx" },
    );
    signal?.throwIfAborted();
    await fs.rename(staging, destination);
    onProgress({ phase: "complete", done: total, total });
    return destination;
  } catch (e) {
    await fs.rm(staging, { recursive: true, force: true });
    throw e;
  }
}
module.exports = { copyArchive, readManifest, metadataName };
