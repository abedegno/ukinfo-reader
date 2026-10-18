"use strict";
const { test, expect } = require("@playwright/test");
test("page maps group approximate postcode positions and load tiles only on request", async ({
  page,
}) => {
  let tiles = 0;
  const referrers = [];
  await page.route("https://tile.openstreetmap.org/**", async (route) => {
    tiles++;
    referrers.push((await route.request().allHeaders()).referer);
    await route.fulfill({
      contentType: "image/png",
      body: Buffer.from(
        "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aR1sAAAAASUVORK5CYII=",
        "base64",
      ),
    });
  });
  await page.route("**/api/map?*", (route) =>
    route.fulfill({
      json: {
        reference: { release: "2026-08" },
        points: [
          {
            postcode: "ZZ11ZZ",
            latitude: 51,
            longitude: -1,
            quality: 8,
            terminated: "200102",
          },
        ],
        unlocated: 0,
      },
    }),
  );
  await page.goto("/?dataset=synthetic-demo&surname=Example&limit=25");
  await expect(page.locator("#items article")).toHaveCount(25);
  expect(tiles).toBe(0);
  await page
    .getByRole("button", { name: "Map this page", exact: true })
    .click();
  await expect(page.locator("#map-status")).toContainText("25 of 25");
  await expect(page.locator("#map-locations .map-location")).toHaveCount(1);
  await expect(page.locator("#map-locations")).toContainText("retired 200102");
  await expect(page.locator("#map-attribution")).toContainText("ONSPD 2026-08");
  expect(tiles).toBe(0);
  await page.locator("#map-online").check();
  await expect.poll(() => tiles).toBeGreaterThan(0);
  expect(
    referrers.every((value) => value === new URL(page.url()).origin + "/"),
  ).toBe(true);
  await expect(page.locator(".leaflet-control-attribution")).toContainText(
    "OpenStreetMap",
  );
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: "test-results/map-mobile.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "Close map", exact: true }).click();
  await expect(page.locator("#map-canvas")).toBeEmpty();
});
test("nearby maps label district centres and radius, and skip partial displayed postcodes", async ({
  page,
}) => {
  await page.route("**/api/search?*", (route) =>
    route.fulfill({
      json: {
        items: [
          {
            dataset: "synthetic-demo",
            source_id: "invented",
            record: {
              surname: "Invented",
              postcode: "WS13",
              postcode_raw: "WS137SW",
            },
          },
        ],
        next_cursor: "",
        geography: {
          reference: { release: "2026-08" },
          near: "WS13",
          latitude: 52.68,
          longitude: -1.82,
          radius_m: 8047,
          centre_type: "district",
          centre_postcodes: 120,
          centre_retired_only: false,
        },
      },
    }),
  );
  let lookups = 0;
  await page.route("**/api/map?*", (route) => {
    lookups++;
    return route.abort();
  });
  await page.goto("/?dataset=synthetic-demo&near=WS13");
  await expect(page.locator("#items article")).toHaveCount(1);
  await page
    .getByRole("button", { name: "Map this page", exact: true })
    .click();
  await expect(page.locator("#map-locations")).toContainText(
    "Approximate district centre",
  );
  await expect(page.locator("#map-locations")).toContainText(
    "not a district/sector boundary",
  );
  await expect(page.locator("#map-status")).toContainText("0 of 1");
  expect(lookups).toBe(0);
});
test("unavailable map reference fails clearly without loading remote tiles", async ({
  page,
}) => {
  await page.route("**/api/map?*", (route) =>
    route.fulfill({
      status: 503,
      json: { error: "Maps need an imported ONSPD reference" },
    }),
  );
  await page.goto(
    "/?view=record&record_dataset=synthetic-demo&record_id=1&record_source=invented%3A0",
  );
  await expect(page.locator("#record-title")).toContainText("Person 000");
  await page
    .getByRole("button", { name: "Show postcode map", exact: true })
    .click();
  await expect(page.locator("#map-status")).toContainText(
    "Maps need an imported ONSPD reference",
  );
  await expect(page.locator("#map-online")).toBeDisabled();
});
