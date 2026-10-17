const { defineConfig } = require("@playwright/test");
module.exports = defineConfig({
  testDir: "./tests",
  timeout: 30000,
  workers: 1,
  use: {
    baseURL: process.env.UKINFO_TEST_URL || "http://127.0.0.1:8080",
    headless: true,
    screenshot: "only-on-failure",
  },
});
