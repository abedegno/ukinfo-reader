"use strict";
const fs = require("node:fs/promises");
const path = require("node:path");
const { execFileSync } = require("node:child_process");
const { chromium } = require("playwright");
(async () => {
  const output = path.resolve(__dirname, "../build/desktop");
  await fs.mkdir(output, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  try {
    const page = await browser.newPage({
      viewport: { width: 1024, height: 1024 },
      deviceScaleFactor: 1,
    });
    await page.setContent(
      "<style>html,body{margin:0;background:transparent}</style>" +
        (await fs.readFile(path.join(__dirname, "assets/icon.svg"), "utf8")),
    );
    await page.screenshot({
      path: path.join(output, "app-icon.png"),
      omitBackground: true,
    });
  } finally {
    await browser.close();
  }
  if (process.platform === "darwin") {
    const directory = path.join(output, "app-icon.iconset");
    await fs.mkdir(directory, { recursive: true });
    for (const size of [16, 32, 128, 256, 512])
      for (const scale of [1, 2])
        execFileSync(
          "sips",
          [
            "-z",
            String(size * scale),
            String(size * scale),
            path.join(output, "app-icon.png"),
            "--out",
            path.join(
              directory,
              `icon_${size}x${size}${scale === 2 ? "@2x" : ""}.png`,
            ),
          ],
          { stdio: "ignore" },
        );
    execFileSync("iconutil", [
      "-c",
      "icns",
      directory,
      "-o",
      path.join(output, "app-icon.icns"),
    ]);
    await fs.rm(directory, { recursive: true, force: true });
  }
})().catch((error) => {
  console.error(error.message);
  process.exitCode = 1;
});
