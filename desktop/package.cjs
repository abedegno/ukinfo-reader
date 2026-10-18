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
  for (const name of [
    "ukinfo-server",
    "ukinfo-import",
    "ukinfo-cabinet",
    "cabinet-notices/BUILD.json",
    "pg/bin/postgres",
    "pg/bin/initdb",
  ]) {
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
    icon: path.join(
      runtime,
      process.platform === "darwin" ? "app-icon.icns" : "app-icon.png",
    ),
    appCategoryType: "public.app-category.reference",
    electronVersion: require("./package.json").devDependencies.electron,
    asar: true,
    prune: true,
    ignore: [
      /^\/(?:tests|test-results|playwright-report)(?:\/|$)/,
      /\.(?:test|spec)\.cjs$/,
      /^\/playwright\.config\.cjs$/,
      /^\/package\.cjs$/,
      /^\/build-icon\.cjs$/,
      /^\/signing\.cjs$/,
      /^\/entitlements(?:\/|$)/,
    ],
    afterCopy: [
      async ({ buildPath }) => {
        const commit = require("node:child_process")
          .execFileSync("git", ["rev-parse", "HEAD"], {
            cwd: __dirname,
            encoding: "utf8",
          })
          .trim();
        if (!/^[a-f0-9]{40}$/.test(commit))
          throw new Error("Invalid build commit");
        await fs.promises.writeFile(
          path.join(buildPath, "build-info.json"),
          JSON.stringify({
            commit,
            source_modified: Boolean(
              require("node:child_process")
                .execFileSync(
                  "git",
                  ["status", "--porcelain", "--untracked-files=no"],
                  { cwd: __dirname, encoding: "utf8" },
                )
                .trim(),
            ),
          }),
        );
        await fs.promises.copyFile(
          path.resolve(
            __dirname,
            "../server/cmd/ukinfo-server/web/research-model.js",
          ),
          path.join(buildPath, "research-model.js"),
        );
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
  for (const destination of destinations) {
    if (process.platform === "darwin" && process.env.UKINFO_CODESIGN_IDENTITY) {
      await require("./signing.cjs").signForDistribution(
        path.join(destination, "UK-Info Archive.app"),
        process.env.UKINFO_CODESIGN_IDENTITY,
        process.env.UKINFO_CODESIGN_KEYCHAIN,
      );
    } else if (process.platform === "darwin")
      require("node:child_process").execFileSync(
        "codesign",
        [
          "--force",
          "--deep",
          "--sign",
          "-",
          path.join(destination, "UK-Info Archive.app"),
        ],
        { stdio: "inherit" },
      );
    console.log(destination);
  }
})().catch((error) => {
  console.error(error.message);
  process.exitCode = 1;
});
