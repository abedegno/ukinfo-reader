"use strict";
const { test, expect } = require("@playwright/test");

test("real Go/PG search, duplicate pagination, original fields and page export", async ({
  page,
}) => {
  await page.goto("/");
  await page
    .getByLabel("Edition", { exact: true })
    .selectOption("synthetic-demo");
  await expect(page.locator("#edition-note")).toContainText(
    "126 imported entries",
  );
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
  await page.getByRole("button", { name: "Previous page" }).click();
  await expect(page.locator("article").first()).toContainText(
    "Person 000 Example",
  );
  await expect(page.locator("#previous")).toBeHidden();
  await page.getByRole("button", { name: "Next page" }).click();
  await expect(page.locator("article").first()).toContainText(
    "Person 050 Example",
  );
  await page.getByRole("button", { name: "Next page" }).click();
  await expect(page.locator("article")).toHaveCount(25);
  await expect(page.locator("#status")).toContainText("Page 3");
  await expect(page.locator("#previous")).toBeVisible();
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

test("partial source imports are explicitly labelled", async ({ page }) => {
  await page.route("**/api/datasets", (route) =>
    route.fulfill({
      json: [
        {
          key: "invented:sample:2",
          manifest: {
            title: "Invented partial fixture",
            edition: "invented",
            sample: true,
          },
          entries: 2,
        },
      ],
    }),
  );
  await page.goto("/");
  await expect(page.locator("#edition-note")).toContainText("PARTIAL SAMPLE");
  await expect(page.locator("#dataset")).toContainText("[SAMPLE]");
  await expect(page.locator("#import-open")).toBeHidden();
});

test("prefix search, postcode district, page sizes and bookmark restoration", async ({
  page,
}) => {
  await page.goto("/");
  await expect(page.locator("#dataset")).toHaveValue("all");
  await page
    .getByLabel("Edition", { exact: true })
    .selectOption("synthetic-demo");
  await page.getByLabel("Surname", { exact: true }).fill("Exam*");
  await page.locator('[name="postcode"]').fill("ZZ1");
  await page.locator("#page-size").selectOption("25");
  await page.getByRole("button", { name: "Search archive" }).click();
  await expect(page.locator("article")).toHaveCount(25);
  await expect(page.locator("article").first()).toContainText("Synthetic demo");
  await expect(page.locator("#status")).toContainText("Results 1–25");
  await page.getByRole("button", { name: "Next page" }).click();
  await expect(page.locator("#status")).toContainText("Results 26–50");
  await page.reload();
  await expect(page.locator("#status")).toContainText("Page 2");
  await expect(page.locator("article").first()).toContainText("Person 025");
  await page.goBack();
  await expect(page.locator("article").first()).toContainText("Person 000");
  await page
    .locator("article")
    .first()
    .getByRole("button", { name: "View record" })
    .click();
  await expect(page.locator("#record-fields")).toContainText(
    "Recorded address",
  );
  await expect(page.locator("#record-fields")).toContainText(
    "Invented demonstration record",
  );
  await expect(page.locator("#record-fields details")).not.toHaveAttribute(
    "open",
  );
  const recordDownload = page.waitForEvent("download");
  await page.getByRole("button", { name: "Export original record" }).click();
  const recordFile = await recordDownload;
  const recordStream = await recordFile.createReadStream();
  let original = "";
  for await (const chunk of recordStream) original += chunk;
  expect(JSON.parse(original).record.building).toBeNull();
  await page.getByRole("button", { name: "Close details" }).click();
  await page.locator("#export-format").selectOption("csv");
  const download = page.waitForEvent("download");
  await page.getByRole("button", { name: "Export this page" }).click();
  const file = await download;
  const stream = await file.createReadStream();
  let content = "";
  for await (const chunk of stream) content += chunk;
  expect(file.suggestedFilename()).toBe("ukinfo-page.csv");
  expect(content).toContain('"Source entry","Citation"');
  expect(content).toContain("invented:0");
});

test("conservative page grouping retains editions and excludes incomplete addresses", async ({
  page,
}) => {
  const record = {
    entry_type: "electoral",
    surname: "Example",
    firstName: "June",
    houseno: "12",
    street: "Invented Street",
    postcode: "ZZ1 1ZZ",
    place_type: "T",
    town: "Fictional Town",
  };
  await page.route("**/api/datasets", (route) =>
    route.fulfill({
      json: [
        {
          key: "a",
          manifest: { title: "Invented 1999", edition: "a" },
          entries: 2,
        },
        {
          key: "b",
          manifest: { title: "Invented 2005", edition: "b" },
          entries: 1,
        },
      ],
    }),
  );
  await page.route("**/api/search?*", (route) =>
    route.fulfill({
      json: {
        items: [
          { dataset: "a", source_id: "one", record },
          { dataset: "b", source_id: "two", record },
          {
            dataset: "a",
            source_id: "three",
            record: { ...record, houseno: "", street: "" },
          },
        ],
        next_cursor: "",
      },
    }),
  );
  await page.goto("/");
  await page.getByLabel("Surname", { exact: true }).fill("Example");
  await page.getByRole("button", { name: "Search archive" }).click();
  await expect(page.locator("article")).toHaveCount(3);
  await page.locator("#group-results").check();
  await expect(page.locator("article")).toHaveCount(2);
  await expect(page.locator("article").first()).toContainText(
    "Invented 1999 · Invented 2005",
  );
  await expect(page.locator("#group-note")).toBeVisible();
  await page
    .locator("article")
    .first()
    .getByRole("button", { name: "View record" })
    .click();
  await expect(page.locator("#record-fields")).toContainText("Town");
  await expect(page.locator("#record-fields")).toContainText(
    "Source entry one",
  );
  await expect(page.locator("#record-fields")).toContainText(
    "Source entry two",
  );
});

test("nearby area searches preserve radius, distances and coordinate provenance", async ({
  page,
}) => {
  const reference = { release: "2026-08", scope: "GB", entries: 1, located: 1 };
  await page.route("**/api/geography", (route) =>
    route.fulfill({ json: { available: true, reference } }),
  );
  await page.route("**/api/search?*", (route) => {
    const params = new URL(route.request().url()).searchParams;
    expect(params.get("near")).toBe("ZZ1");
    expect(params.get("radius_m")).toBe("3219");
    expect(params.has("surname")).toBe(false);
    return route.fulfill({
      json: {
        items: [
          {
            dataset: "synthetic-demo",
            source_id: "nearby-fixture",
            record: {
              entry_type: "electoral",
              surname: "Example",
              firstName: "June",
              houseno: "12",
              street: "Invented Street",
              postcode: "ZZ1 1ZZ",
            },
            distance_m: 321.9,
            location_quality: 8,
            location_terminated: "200102",
          },
        ],
        next_cursor: "",
        geography: {
          reference,
          near: "ZZ1",
          centre_type: "district",
          centre_postcodes: 42,
          radius_m: 3219,
          terminated: "",
        },
      },
    });
  });
  await page.goto("/");
  await page.getByText("Search near a postcode", { exact: true }).click();
  await expect(page.locator("#near")).toBeEnabled();
  await page.getByLabel("Centre postcode", { exact: true }).fill("ZZ1");
  const centreBox = await page.locator("#near").boundingBox();
  const submitBox = await page.locator("#submit").boundingBox();
  expect(submitBox.y).toBeGreaterThan(centreBox.y + centreBox.height);
  await page.locator("#radius").selectOption("3219");
  await page.getByRole("button", { name: "Search archive" }).click();
  await expect(page.locator("article")).toHaveCount(1);
  await expect(page.locator("article")).toContainText("0.20 miles");
  await expect(page.locator("#geography-note")).toContainText("ONSPD 2026-08");
  await expect(page.locator("#geography-note")).toContainText(
    "not distance order",
  );
  await expect(page.locator("#geography-note")).toContainText(
    "approximate district centre from 42 postcode points",
  );
  await page
    .locator("article")
    .getByRole("button", { name: "View record" })
    .click();
  await expect(page.locator("#record-fields")).toContainText(
    "Location estimate",
  );
  await expect(page.locator("#record-fields")).toContainText(
    "Last known position of a retired postcode",
  );
  await page.getByRole("button", { name: "Close details" }).click();
  await page.reload();
  await expect(page.locator("#nearby-controls")).toHaveAttribute("open", "");
  await expect(page.locator("#near")).toHaveValue("ZZ1");
  await expect(page.locator("#radius")).toHaveValue("3219");
  await expect(page.locator("article")).toHaveCount(1);
  await expect(page.locator("#geography-attribution")).toContainText(
    "Royal Mail",
  );
});

test("address-only searches preserve fields, paging, bookmarks and edition coverage", async ({
  page,
}) => {
  await page.route("**/api/datasets", (route) =>
    route.fulfill({
      json: [
        {
          key: "synthetic-demo",
          manifest: { edition: "invented", title: "Invented demo" },
          entries: 126,
        },
        {
          key: "unindexed-demo",
          manifest: { edition: "unindexed", title: "Unindexed demo" },
          entries: 10,
        },
      ],
    }),
  );
  await page.route("**/api/addresses", (route) =>
    route.fulfill({
      json: {
        available: true,
        datasets: [{ dataset: "synthetic-demo", entries: 126, locations: 1 }],
      },
    }),
  );
  await page.route("**/api/search?*", (route) => {
    const params = new URL(route.request().url()).searchParams;
    expect(params.get("street")).toBe("Invented*");
    expect(params.get("town")).toBe("Example Town");
    expect(params.get("county")).toBe("Old County");
    expect(params.has("surname")).toBe(false);
    return route.fulfill({
      json: {
        items: [
          {
            dataset: "synthetic-demo",
            source_id: params.has("cursor") ? "address-2" : "address-1",
            record: {
              surname: "Example",
              firstName: params.has("cursor") ? "Second" : "First",
              street: "Invented Street",
              town: "Example Town",
              county: "Old County",
              postcode: "ZZ1 1ZZ",
            },
          },
        ],
        next_cursor: params.has("cursor") ? "" : "address-next",
      },
    });
  });
  await page.goto("/");
  await page
    .getByText("Search by street, town or county", { exact: true })
    .click();
  await expect(page.locator('[name="street"]')).toBeDisabled();
  await page
    .getByLabel("Edition", { exact: true })
    .selectOption("synthetic-demo");
  await expect(page.locator('[name="street"]')).toBeEnabled();
  await page.getByLabel("Street", { exact: true }).fill("Invented*");
  await page.getByLabel("Town / city", { exact: true }).fill("Example Town");
  await page.getByLabel("County", { exact: true }).fill("Old County");
  await page.getByRole("button", { name: "Search archive" }).click();
  await expect(page.locator("article")).toContainText("First Example");
  await page.getByRole("button", { name: "Next page" }).click();
  await expect(page.locator("article")).toContainText("Second Example");
  await page.reload();
  await expect(page.locator("#address-controls")).toHaveAttribute("open", "");
  await expect(page.locator('[name="street"]')).toHaveValue("Invented*");
  await expect(page.locator('[name="town"]')).toHaveValue("Example Town");
  await expect(page.locator("article")).toContainText("Second Example");
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
});
