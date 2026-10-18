"use strict";
const { test, expect } = require("@playwright/test");
test("desktop postcode setup verifies a chosen ZIP, reports errors and enables nearby without reloading", async ({
  page,
}) => {
  await page.addInitScript(() => {
    let listener,
      attempts = 0;
    window.referenceReady = false;
    window.ukinfoDesktop = {
      capabilities: async () => ({
        canImport: false,
        canImportGeography: true,
      }),
      onProgress: () => {},
      geographySource: async () => ({
        release: "2026-08",
        url: "https://www.arcgis.com/sharing/rest/content/items/9e5a92a3cfb14dc7ad43d6ea7a7b8c7f/data",
      }),
      chooseGeography: async () => ({
        selection: "opaque-selection",
        label: "ONSPD_AUG_2026.zip",
      }),
      onGeographyProgress: (fn) => {
        listener = fn;
      },
      importGeography: async (selection) => {
        if (selection !== "opaque-selection") throw new Error("Bad selection");
        if (++attempts === 1) throw new Error("ONSPD ZIP SHA-256 mismatch");
        listener({ progress: { phase: "loading postcodes", done: 20000 } });
        await new Promise((resolve) => setTimeout(resolve, 100));
        window.referenceReady = true;
      },
      cancelImport: async () => {},
    };
  });
  await page.route("**/api/geography", async (route) =>
    route.fulfill({
      json: (await page.evaluate(() => window.referenceReady))
        ? {
            available: true,
            reference: { release: "2026-08", entries: 1234567 },
          }
        : { available: false },
    }),
  );
  await page.goto("/");
  await expect(page.locator("#postcode-near")).toBeDisabled();
  await page
    .getByRole("button", { name: "Postcode setup", exact: true })
    .click();
  await expect(page.locator("#postcode-reference-status")).toContainText(
    "No postcode reference",
  );
  await expect(page.locator("#postcode-download")).toHaveAttribute(
    "href",
    /arcgis.com/,
  );
  await expect(page.locator("#postcode-start")).toBeDisabled();
  await page.locator("#postcode-choose").click();
  await expect(page.locator("#postcode-selected")).toHaveText(
    "ONSPD_AUG_2026.zip",
  );
  await page.locator("#postcode-start").click();
  await expect(page.locator("#postcode-message")).toContainText(
    "SHA-256 mismatch",
  );
  await expect(page.locator("#postcode-near")).toBeDisabled();
  await page.locator("#postcode-start").click();
  await expect(page.locator("#postcode-message")).toContainText("are ready");
  await expect(page.locator("#postcode-reference-status")).toContainText(
    "1,234,567",
  );
  await page.locator("#postcode-setup-close").click();
  await expect(page.locator("#postcode-near")).toBeEnabled();
});
