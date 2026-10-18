"use strict";
const { test, expect } = require("@playwright/test");
test("archive management shows sample completeness, native removal results and guided restart", async ({
  page,
}) => {
  await page.addInitScript(() => {
    let removed = false,
      attempts = 0;
    const manifest = {
      title: "Invented historical edition <safe>",
      edition: "invented",
      sample: true,
    };
    window.ukinfoDesktop = {
      capabilities: async () => ({
        canImport: false,
        canManageCatalogue: true,
      }),
      onProgress: () => {},
      catalogue: async () => ({
        datasets: removed
          ? []
          : [
              {
                selection: "opaque",
                key: "invented:sample:10000",
                manifest,
                entries: 10000,
                bytes: 1024 * 1024,
                imported_at: "2026-10-01T12:00:00Z",
                address_ready: false,
              },
            ],
        database_bytes: 8 * 1024 ** 3,
        shared_bytes: 1024 ** 3,
        geography_bytes: 0,
        free_bytes: 20 * 1024 ** 3,
        state: { running: false },
        setup: {
          status: "running",
          kind: "folder",
          label: "source",
          edition: "invented",
          title: "Invented historical edition",
          limit: 10000,
        },
      }),
      manageDataset: async (selection, action) => {
        if (selection !== "opaque") throw Error("unsafe ref");
        if (action === "remove") {
          if (++attempts === 1) return { canceled: true };
          removed = true;
        }
        return { complete: true };
      },
      resumeSetup: async () => ({
        selection: "restored",
        edition: "invented",
        title: "Invented historical edition",
        entries: 100000,
        label: "source",
        limit: 10000,
      }),
    };
  });
  await page.goto("/");
  await page.locator("#catalogue-open").click();
  await expect(page.locator("#catalogue-editions")).toContainText(
    "Sample · 10,000 entries",
  );
  await expect(page.locator("#catalogue-storage")).toContainText("8 GiB");
  await expect(page.locator("#catalogue-editions script")).toHaveCount(0);
  await page
    .getByRole("button", { name: "Remove edition…", exact: true })
    .click();
  await expect(page.locator("#catalogue-message")).toHaveText("Edition kept.");
  await expect(page.locator(".catalogue-edition")).toHaveCount(1);
  await page
    .getByRole("button", { name: "Add complete edition", exact: true })
    .click();
  await expect(page.locator("#import-dialog")).toBeVisible();
  await expect(page.locator("#import-scope")).toHaveValue("full");
  await expect(page.locator("#import-title")).toHaveText(
    "Invented historical edition",
  );
  await page.locator("#import-close").click();
  await page.locator("#catalogue-open").click();
  await page
    .getByRole("button", { name: "Remove edition…", exact: true })
    .click();
  await expect(page.locator("#catalogue-message")).toContainText(
    "notes are preserved",
  );
  await expect(page.locator("#catalogue-empty")).toBeVisible();
  await expect(page.locator("#catalogue-continue-label")).toContainText(
    "interrupted",
  );
  await page.locator("#catalogue-resume").click();
  await expect(page.locator("#import-scope")).toHaveValue("10000");
});
