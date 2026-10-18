"use strict";
const { test, expect } = require("@playwright/test");
test("empty desktop catalogue opens the guided importer and finishes with a searchable demo", async ({
  page,
}) => {
  await page.addInitScript(() => {
    let progress;
    window.imported = false;
    window.ukinfoDesktop = {
      capabilities: async () => ({ canImport: true }),
      onProgress: (fn) => {
        progress = fn;
      },
      chooseSource: async () => {
        throw new Error("That folder does not match a supported edition.");
      },
      demoSource: async () => ({
        selection: "demo",
        title: "Synthetic demo",
        edition: "synthetic-demo",
        entries: 126,
        label: "Invented demo",
      }),
      importSource: async () => {
        progress({ progress: { phase: "loading", done: 50, total: 126 } });
        await new Promise((resolve) => setTimeout(resolve, 100));
        window.imported = true;
      },
      cancelImport: async () => {},
    };
  });
  await page.route("**/api/datasets", async (route) =>
    route.fulfill({
      json: (await page.evaluate(() => window.imported))
        ? [
            {
              key: "synthetic-demo",
              manifest: { title: "Synthetic demo", edition: "synthetic-demo" },
              entries: 126,
            },
          ]
        : [],
    }),
  );
  await page.goto("/");
  await expect(page.locator("#import-dialog")).toBeVisible();
  await expect(page.locator("#import-heading")).toHaveText(
    "Set up your archive",
  );
  await expect(page.locator("#import-welcome")).toContainText(
    "No directory data is bundled",
  );
  await page
    .getByRole("button", { name: "Choose source folder", exact: false })
    .click();
  await expect(page.locator("#import-message")).toContainText(
    "does not match a supported edition",
  );
  await page
    .getByRole("button", { name: "Try invented demo", exact: true })
    .click();
  await expect(page.locator("#import-steps [aria-current=step]")).toContainText(
    "Review edition",
  );
  await page
    .getByRole("button", { name: "Import edition", exact: true })
    .click();
  await expect(page.locator("#import-message")).toHaveText(
    "Edition imported successfully.",
  );
  await expect(page.locator("#import-steps [aria-current=step]")).toContainText(
    "Search",
  );
  await page
    .getByRole("button", { name: "Start searching", exact: true })
    .click();
  await expect(page.locator("#import-dialog")).not.toBeVisible();
  await expect(page.locator("[name=surname]")).toHaveValue("Example");
  await page.getByRole("button", { name: "Search archive" }).click();
  await expect(page.locator("#items article")).toHaveCount(50);
});
