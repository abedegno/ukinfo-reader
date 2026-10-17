"use strict";
const { packager } = require("@electron/packager");
const fs = require("node:fs");
const path = require("node:path");

(async () => {
  if (!["darwin", "linux"].includes(process.platform))
    throw new Error(
      "Only native macOS and Linux development bundles are supported.",
    );
  const runtime = path.resolve(__dirname, "../build/desktop");
  for (const name of ["ukinfo-server", "pg/bin/postgres", "pg/bin/initdb"]) {
    if (!fs.existsSync(path.join(runtime, name)))
      throw new Error(
        `Missing staged runtime: ${name}. See docs/search-app.md.`,
      );
  }
  const destinations = await packager({
    dir: __dirname,
    out: path.resolve(__dirname, "../dist/desktop"),
    name: "UK-Info Archive",
    appBundleId: "org.ukinfo.archive",
    platform: process.platform,
    arch: process.arch,
    overwrite: true,
    electronVersion: require("./package.json").devDependencies.electron,
    asar: true,
    prune: true,
    ignore: [
      /^\/(?:tests|test-results|playwright-report)(?:\/|$)/,
      /\.(?:test|spec)\.cjs$/,
      /^\/playwright\.config\.cjs$/,
      /^\/package\.cjs$/,
    ],
    afterCopy: [
      async ({ buildPath }) => {
        await fs.promises.cp(
          runtime,
          path.join(path.dirname(buildPath), "runtime"),
          { recursive: true },
        );
        await fs.promises.copyFile(
          path.resolve(__dirname, "../LICENSE"),
          path.join(buildPath, "LICENSE"),
        );
      },
    ],
  });
  for (const destination of destinations) console.log(destination);
})().catch((error) => {
  console.error(error.message);
  process.exitCode = 1;
});
