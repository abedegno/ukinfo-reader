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

test("record links and exact-address exploration work with the real invented catalogue", async ({
  page,
}) => {
  await page.addInitScript(() =>
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: {
        writeText: async (text) => {
          window.copiedRecordLink = text;
        },
      },
    }),
  );
  await page.goto(
    "/?view=record&record_dataset=synthetic-demo&record_id=1&record_source=invented%3A0",
  );
  await expect(page.locator("#details")).toBeVisible();
  await expect(page.locator("#record-title")).toContainText(
    "Person 000 Example",
  );
  await expect(page.locator("#explore-address")).toBeEnabled();
  await page
    .getByRole("button", { name: "Copy record link", exact: true })
    .click();
  const copied = new URL(await page.evaluate(() => window.copiedRecordLink));
  expect(copied.searchParams.get("record_source")).toBe("invented:0");
  expect(copied.searchParams.has("surname")).toBe(false);

  await page
    .getByRole("button", { name: "Explore this address", exact: true })
    .click();
  await expect(page.locator("#address-details")).toBeVisible();
  await expect(page.locator("#address-items article")).toHaveCount(1);
  await expect(page.locator("#address-heading")).toContainText(
    "1 Invented Street",
  );
  await expect(page.locator("#address-items")).toContainText(
    "Selected source record",
  );
  await page.locator("#address-scope").selectOption("all");
  await expect(page.locator("#address-items article")).toHaveCount(1);
  await expect(page).toHaveURL(/address_scope=all/);
  await page.reload();
  await expect(page.locator("#address-details")).toBeVisible();
  await expect(page.locator("#address-scope")).toHaveValue("all");
  await expect(page.locator("#address-items article")).toHaveCount(1);
  const downloadPromise = page.waitForEvent("download");
  await page.getByRole("button", { name: "Export address page" }).click();
  const download = await downloadPromise;
  const stream = await download.createReadStream();
  let body = "";
  for await (const chunk of stream) body += chunk;
  const items = body.trim().split("\n").map(JSON.parse);
  expect(items).toHaveLength(1);
  expect(items[0].source_id).toBe("invented:0");
  await page
    .getByRole("button", { name: "View source record", exact: true })
    .click();
  await expect(page.locator("#details")).toBeVisible();
  await expect(page.locator("#record-title")).toContainText(
    "Person 000 Example",
  );
  await expect(page.locator("#back-address")).toBeVisible();
  await page.getByRole("button", { name: "Back to address listings" }).click();
  await expect(page.locator("#address-details")).toBeVisible();
  await page.goBack();
  await expect(page.locator("#details")).toBeVisible();
  await page.goBack();
  await expect(page.locator("#address-details")).toBeVisible();
  await page
    .getByRole("button", { name: "Close address", exact: true })
    .click();
  await expect(page.locator("#address-details")).not.toBeVisible();
  expect(new URL(page.url()).searchParams.has("view")).toBe(false);
});

test("address pages preserve scope, pagination, reload and raw record exports", async ({
  page,
}) => {
  const anchor = {
    dataset: "synthetic-demo",
    id: 1,
    source_id: "anchor",
    record: {
      surname: "Example",
      firstName: "June",
      houseno: "12",
      subbuilding: "Flat 1",
      street: "Invented Street",
      postcode: "ZZ1 1ZZ",
    },
  };
  await page.route("**/api/record?*", (route) => {
    const params = new URL(route.request().url()).searchParams;
    if (params.get("format") === "jsonl")
      return route.fulfill({
        contentType: "application/x-ndjson",
        body: '{"dataset":"synthetic-demo","id":1,"source_id":"anchor","record":{"raw_integer":9007199254740993}}\n',
      });
    return route.fulfill({ json: { item: anchor, address_available: true } });
  });
  await page.route("**/api/address?*", (route) => {
    const params = new URL(route.request().url()).searchParams;
    const second = params.has("cursor");
    expect(params.get("source_id")).toBe("anchor");
    return route.fulfill({
      json: {
        anchor,
        scope: params.get("scope"),
        items: [
          {
            ...anchor,
            source_id: second ? "second" : "anchor",
            record: { ...anchor.record, firstName: second ? "Second" : "June" },
          },
        ],
        next_cursor: second ? "" : "page-two",
        notice:
          "Same recorded premises; relationships and continuous residence are not inferred.",
      },
    });
  });
  await page.goto(
    "/?view=record&record_dataset=synthetic-demo&record_id=1&record_source=anchor",
  );
  await expect(page.locator("#details")).toBeVisible();
  const downloadPromise = page.waitForEvent("download");
  await page.getByRole("button", { name: "Export original record" }).click();
  const download = await downloadPromise;
  let body = "";
  for await (const chunk of await download.createReadStream()) body += chunk;
  expect(body).toContain("9007199254740993");
  await page.getByRole("button", { name: "Explore this address" }).click();
  await expect(page.locator("#address-items")).toContainText("June");
  await page.getByRole("button", { name: "Next address page" }).click();
  await expect(page.locator("#address-items")).toContainText("Second");
  await expect(page).toHaveURL(/address_cursor=page-two/);
  await page.getByRole("button", { name: "Previous address page" }).click();
  await expect(page.locator("#address-items")).toContainText("June");
  await page.locator("#address-scope").selectOption("all");
  await expect(page).toHaveURL(/address_scope=all/);
  await page.getByRole("button", { name: "Next address page" }).click();
  await page.reload();
  await expect(page.locator("#address-items")).toContainText("Second");
  await expect(page.locator("#address-scope")).toHaveValue("all");
  await page.getByRole("button", { name: "First page", exact: true }).click();
  await expect(page.locator("#address-items")).toContainText("June");
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
});

test("saved links from another installation open locally without external navigation", async ({
  page,
}) => {
  await page.goto("/");
  await page
    .getByRole("button", { name: "Open saved link", exact: true })
    .click();
  await page
    .getByLabel("Saved link", { exact: true })
    .fill(
      "https://other-installation.invalid/archive?view=record&record_dataset=synthetic-demo&record_id=1&record_source=invented%3A0",
    );
  await page.getByRole("button", { name: "Open link", exact: true }).click();
  await expect(page.locator("#details")).toBeVisible();
  await expect(page.locator("#record-title")).toContainText(
    "Person 000 Example",
  );
  expect(new URL(page.url()).hostname).toBe("127.0.0.1");
  await page
    .getByRole("button", { name: "Close details", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Open saved link", exact: true })
    .click();
  await page
    .getByLabel("Saved link", { exact: true })
    .fill("https://other-installation.invalid/ordinary-page");
  await page.getByRole("button", { name: "Open link", exact: true }).click();
  await expect(page.locator("#saved-link-status")).toContainText(
    "Paste a record or address link",
  );
});

test("record links reject changed identities and explain unavailable premises", async ({
  page,
}) => {
  await page.goto(
    "/?view=record&record_dataset=synthetic-demo&record_id=1&record_source=changed",
  );
  await expect(page.locator("#status")).toContainText("same source entry");
  await expect(page.locator("#details")).not.toBeVisible();
  await page.goto(
    "/?view=record&record_dataset=synthetic-demo&record_id=126&record_source=invented%3Abusiness",
  );
  await expect(page.locator("#details")).toBeVisible();
  await expect(page.locator("#explore-address")).toBeDisabled();
  await expect(page.locator("#record-address-note")).toContainText(
    "house number or building name",
  );
});
