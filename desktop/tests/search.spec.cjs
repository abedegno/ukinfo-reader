"use strict";
const { test, expect } = require("@playwright/test");

test("real Go/PG search, duplicate pagination, original fields and page export", async ({
  page,
}) => {
  await page.goto("/");
  await page
    .getByLabel("Edition", { exact: true })
    .selectOption("synthetic-demo");
  await expect(page.locator("#edition-note")).toContainText("PARTIAL SAMPLE");
  await page.getByLabel("Surname", { exact: true }).fill("Example");
  await page.getByRole("button", { name: "Search archive" }).click();
  await expect(page.locator("article")).toHaveCount(50);
  await expect(page.locator("article").first()).toContainText(
    "Person 000 Example",
  );
  await page
    .locator("article")
    .first()
    .getByRole("button", { name: "View record" })
    .click();
  await expect(page.getByRole("dialog")).toContainText("invented:0");
  await expect(page.locator("#record-fields")).toContainText(
    "synthetic_demo_not_a_real_person",
  );
  await page.getByRole("button", { name: "Close details" }).click();
  const download = page.waitForEvent("download");
  await page.getByRole("button", { name: "Export this page" }).click();
  const file = await download;
  const stream = await file.createReadStream();
  let content = "";
  for await (const chunk of stream) content += chunk;
  const exported = content.trim().split("\n").map(JSON.parse);
  expect(exported).toHaveLength(50);
  expect(exported[0].dataset).toBe("synthetic-demo");
  expect(exported[0].record.building).toBeNull();
  await page.getByRole("button", { name: "Next page" }).click();
  await expect(page.locator("article").first()).toContainText(
    "Person 050 Example",
  );
  await page.getByRole("button", { name: "Next page" }).click();
  await expect(page.locator("article")).toHaveCount(25);
  await expect(page.locator("#next")).toBeHidden();
  await page.screenshot({
    path: "test-results/search-desktop.png",
    fullPage: true,
  });
});
test("business search and narrow mobile layout", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/");
  await page
    .getByLabel("Edition", { exact: true })
    .selectOption("synthetic-demo");
  await page.getByRole("button", { name: "Businesses", exact: true }).click();
  await page
    .getByLabel("Business name", { exact: true })
    .fill("Aster Archive Supplies");
  await page.getByRole("button", { name: "Search archive" }).click();
  await expect(page.locator("article")).toHaveCount(1);
  await expect(page.locator("article")).toContainText("Aster Archive Supplies");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: "test-results/search-mobile.png",
    fullPage: true,
  });
});
