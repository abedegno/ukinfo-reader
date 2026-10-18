"use strict";
const fs = require("node:fs");
const path = require("node:path");
const { randomUUID } = require("node:crypto");
function readProfile(root) {
  const filename = path.join(root, "archive-location.json");
  try {
    const stat = fs.lstatSync(filename);
    if (!stat.isFile() || stat.size > 16384) throw new Error();
    const info = JSON.parse(fs.readFileSync(filename, "utf8"));
    if (
      info.version !== 1 ||
      typeof info.directory !== "string" ||
      !path.isAbsolute(info.directory) ||
      info.directory.length > 8192
    )
      throw new Error();
    const selected = fs.realpathSync(info.directory);
    if (!fs.statSync(selected).isDirectory()) throw new Error();
    return selected;
  } catch (e) {
    if (e.code === "ENOENT" && !fs.existsSync(filename)) return root;
    throw new Error(
      "The selected archive is unavailable. Reconnect its drive, or use Open archive to choose another archive. Existing files have been preserved.",
    );
  }
}
function validateProfile(directory) {
  directory = fs.realpathSync(directory);
  if (fs.existsSync(path.join(directory, "ukinfo-backup.json")))
    throw new Error(
      "Restore this backup before opening it. The backup itself must remain unchanged.",
    );
  if (
    fs
      .readFileSync(path.join(directory, "catalogue/pg18/PG_VERSION"), "utf8")
      .trim() !== "18"
  )
    throw new Error("Choose a PostgreSQL 18 UK-Info archive.");
  if (fs.existsSync(path.join(directory, "ukinfo-profile.json"))) {
    const info = JSON.parse(
      fs.readFileSync(path.join(directory, "ukinfo-profile.json"), "utf8"),
    );
    if (
      info.platform !== process.platform ||
      info.architecture !== process.arch
    )
      throw new Error(
        "This archive was restored for another operating system or CPU architecture.",
      );
  }
  return directory;
}
function selectProfile(root, directory) {
  directory = validateProfile(directory);
  fs.mkdirSync(root, { recursive: true, mode: 0o700 });
  const file = path.join(root, "archive-location.json"),
    temporary = file + "." + randomUUID() + ".tmp";
  try {
    fs.writeFileSync(temporary, JSON.stringify({ version: 1, directory }), {
      mode: 0o600,
      flag: "wx",
    });
    fs.renameSync(temporary, file);
  } finally {
    fs.rmSync(temporary, { force: true });
  }
}
module.exports = { readProfile, selectProfile, validateProfile };
