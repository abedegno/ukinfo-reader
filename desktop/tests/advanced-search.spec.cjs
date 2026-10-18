"use strict";
const { test, expect } = require("@playwright/test");
test("whole-search grouping, source dialog, export and order survive paging and reload", async ({
  page,
}) => {
  await page.goto(
    "/?dataset=synthetic-demo&surname=Example&group=listing&sort=name&limit=25",
  );
  await expect(page.locator("#items article")).toHaveCount(25);
  await expect(page.locator("#group-results")).toHaveValue("listing");
  await expect(page.locator("#status")).toContainText("listing groups");
  await expect(page.locator("#items article").first()).toContainText(
    "source occurrence",
  );
  await page
    .locator("#items article")
    .first()
    .getByRole("button", { name: "View record" })
    .click();
  await expect(page.locator("#group-dialog")).toBeVisible();
  await expect(page.locator("#group-sources article")).toHaveCount(1);
  const download = page.waitForEvent("download");
  await page.locator("#group-export").click();
  const stream = await (await download).createReadStream();
  let text = "";
  for await (const chunk of stream) text += chunk;
  expect(JSON.parse(text).source_id).toBeTruthy();
  await page.locator("#group-close").click();
  await page.locator("#next").click();
  await expect(page.locator("#status")).toContainText("Page 2");
  const first = await page.locator("#items article").first().textContent();
  await page.reload();
  await expect(page.locator("#items article").first()).toHaveText(first);
  await page.locator("#result-sort").selectOption("edition");
  await expect(page.locator("#status")).toContainText("Page 1");
  await expect(page.locator("#order-note")).toContainText("publication first");
  await page.locator("#group-results").selectOption("source");
  await expect(page.locator("#status")).toContainText("source entries");
});
test("classification-only search uses business mode and preserves explicit criteria", async ({
  page,
}) => {
  // Backend classification matching is covered by real PostgreSQL integration;
  // this checks the UI against an invented matching business page.
  await page.route("**/api/research", (route) =>
    route.fulfill({
      json: {
        datasets: ["synthetic-demo"],
        business_datasets: ["synthetic-demo"],
      },
    }),
  );
  await page.route("**/api/search?*", async (route) => {
    const params = new URL(route.request().url()).searchParams;
    expect(params.get("industry")).toBe("invented*");
    expect(params.get("sic")).toBe("1234");
    expect(params.get("kind")).toBe("business");
    await route.fulfill({ json: { items: [], next_cursor: "" } });
  });
  await page.goto(
    "/?dataset=synthetic-demo&kind=business&industry=invented*&sic=1234",
  );
  await expect(page.locator("#industry")).toHaveValue("invented*");
  await expect(page.locator("#business-classification")).toBeVisible();
  await expect(page.locator("#status")).toContainText("No matching");
  await expect(page.locator("#active-filters")).toContainText("Industry");
});
test("invalid fields have associated errors and keyboard focus; clear resets them", async ({
  page,
}) => {
  await page.goto("/");
  await page.locator("#dataset").selectOption("synthetic-demo");
  await page.locator("#postcode").fill("invalid");
  await page.locator("#submit").click();
  await expect(page.locator("#postcode")).toHaveAttribute(
    "aria-invalid",
    "true",
  );
  await expect(page.locator("#postcode")).toBeFocused();
  await expect(page.locator("#postcode")).toHaveAttribute(
    "aria-describedby",
    /search-error-postcode/,
  );
  await expect(page.locator("#search-error-postcode")).toContainText(
    "valid postcode",
  );
  await page.locator("#clear-search").click();
  await expect(page.locator("#postcode")).not.toHaveAttribute("aria-invalid");
  await page.locator("#surname").fill("Example");
  await page.locator("#submit").click();
  await expect(page.locator("#results-title")).toBeFocused();
  await page
    .locator("#items article")
    .first()
    .getByRole("button", { name: "View record" })
    .click();
  await expect(page.locator("#record-title")).toBeFocused();
});
test("grouping and order labels fit narrow screens and magnified layouts", async ({
  page,
}) => {
  for (const width of [1240, 768, 390, 320]) {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      "/?dataset=synthetic-demo&surname=Example&group=listing&sort=edition&limit=25",
    );
    await expect(page.locator("#items article")).toHaveCount(25);
    const sizes = await page
      .locator("#group-results, #result-sort")
      .evaluateAll((nodes) =>
        nodes.map((node) => {
          const style = getComputedStyle(node),
            canvas = document.createElement("canvas"),
            ctx = canvas.getContext("2d");
          ctx.font = style.font;
          return {
            available:
              node.clientWidth -
              parseFloat(style.paddingLeft) -
              parseFloat(style.paddingRight) -
              24,
            text: ctx.measureText(node.selectedOptions[0].textContent.trim())
              .width,
          };
        }),
      );
    for (const size of sizes)
      expect(size.available).toBeGreaterThanOrEqual(size.text);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
  }
});
