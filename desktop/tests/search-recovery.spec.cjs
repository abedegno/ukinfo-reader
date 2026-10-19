"use strict";
const { test, expect } = require("@playwright/test");

async function form(page) {
  await page.goto("/");
  await page.getByLabel("Edition", { exact: true }).selectOption("synthetic-demo");
  await page.getByLabel("Surname", { exact: true }).fill("Example");
}

test("cancel search and retain clearly identified previous results", async ({ page }) => {
  await form(page);
  await page.getByRole("button", { name: "Search archive" }).click();
  await expect(page.locator("article")).toHaveCount(50);
  await page.route("**/api/search?**", async route => {
    await new Promise(resolve => setTimeout(resolve, 1000));
    await route.fulfill({ json: { items: [], next_cursor: "" } }).catch(() => {});
  });
  await page.getByRole("button", { name: "Search archive" }).click();
  await expect(page.locator("#cancel-search")).toBeVisible();
  await page.locator("#cancel-search").click();
  await expect(page.locator("#status")).toContainText("Search cancelled");
  await expect(page.locator("#status")).toContainText("Previous results are shown");
  await expect(page.locator("article")).toHaveCount(50);
  await expect(page.locator("#search-recovery")).toBeHidden();
  await expect(page.locator("#submit")).toBeEnabled();
});

test("HTML gateway errors remain readable and retry is manual", async ({ page }) => {
  await form(page);
  let calls = 0;
  await page.route("**/api/search?**", route => {
    calls++;
    return route.fulfill({ status: 502, contentType: "text/html", body: "<html>PRIVATE_PROXY_BODY</html>" });
  });
  await page.getByRole("button", { name: "Search archive" }).click();
  await expect(page.locator("#retry-search")).toBeVisible();
  await expect(page.locator("#status")).not.toContainText("PRIVATE_PROXY_BODY");
  expect(calls).toBe(1);
  await page.locator("#retry-search").click();
  await expect.poll(() => calls).toBe(2);
});

test("rate limits enforce the server cooldown", async ({ page }) => {
  await form(page);
  let calls = 0;
  await page.route("**/api/search?**", route => {
    calls++;
    return route.fulfill({ status: 429, headers: { "Retry-After": "2" }, json: { error: "Please wait", code: "rate_limited", request_id: "invented-reference" } });
  });
  await page.getByRole("button", { name: "Search archive" }).click();
  await expect(page.locator("#retry-search")).toBeDisabled();
  await expect(page.locator("#status")).toContainText("invented-reference");
  await expect(page.locator("#retry-search")).toBeEnabled({ timeout: 5000 });
  expect(calls).toBe(1);
});

test("grouped timeout offers source entries without losing filters", async ({ page }) => {
  await form(page);
  await page.locator("#group-results").selectOption("listing");
  const queries = [];
  await page.route("**/api/search?**", route => {
    queries.push(new URL(route.request().url()).searchParams);
    if (queries.length === 1) return route.fulfill({ status: 503, json: { error: "Search timed out", code: "query_timeout" } });
    return route.fulfill({ json: { items: [], next_cursor: "" } });
  });
  await page.getByRole("button", { name: "Search archive" }).click();
  await expect(page.locator("#source-search")).toBeVisible();
  await page.locator("#source-search").click();
  await expect.poll(() => queries.length).toBe(2);
  expect(queries[1].has("group")).toBe(false);
  expect(queries[1].get("surname")).toBe("Example");
  expect(queries[1].get("dataset")).toBe("synthetic-demo");
});
