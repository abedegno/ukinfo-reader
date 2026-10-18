"use strict";
const { test, expect } = require("@playwright/test");
test("saved criteria rerun without a page cursor; opt-in history records empty searches and coverage", async ({
  page,
}) => {
  await page.goto(
    "/?dataset=synthetic-demo&kind=person&surname=Example&match=prefix&limit=25",
  );
  await expect(page.locator("#items article")).toHaveCount(25);
  await page.locator("#next").click();
  await expect(page.locator("#status")).toContainText("Page 2");
  await page.locator("#workbench-open").click();
  await page.locator("#workbench-name").fill("Invented people");
  await page.locator("#workbench-save").click();
  await expect(page.locator("#workbench-searches article")).toHaveCount(1);
  const stored = await page.evaluate(() =>
    JSON.parse(localStorage.getItem("ukinfo.workbench.v1")),
  );
  expect(stored.searches[0].query.cursor).toBeUndefined();
  expect(stored.log).toHaveLength(0);
  await page.locator("#workbench-logging").check();
  await expect(page.locator("#workbench-status")).toContainText("enabled");
  await page
    .locator("#workbench-searches")
    .getByRole("button", { name: "Run search" })
    .click();
  await expect(page.locator("#status")).toContainText("Page 1");
  await page.locator("#surname").fill("Inventedmissingname");
  await page.locator("#submit").click();
  await expect(page.locator("#status")).toContainText("No matching");
  await page.locator("#about-search summary").click();
  await expect(page.locator("#search-explanation")).toContainText(
    "does not establish",
  );
  await page.locator("#workbench-open").click();
  await expect(page.locator("#workbench-log article")).toHaveCount(2);
  await expect(page.locator("#workbench-log article").first()).toContainText(
    "0 source entries",
  );
  await expect(page.locator("#workbench-log article").first()).toContainText(
    "Synthetic",
  );
  await page.locator("#workbench-clear-log").click();
  await expect(page.locator("#workbench-log article")).toHaveCount(0);
});
test("note drafts survive reload, save deliberately, and cannot overwrite a concurrent draft", async ({
  page,
  context,
}) => {
  await page.goto(
    "/?view=record&record_dataset=synthetic-demo&record_id=1&record_source=invented%3A0",
  );
  await expect(page.locator("#record-title")).toContainText("Person 000");
  await page.locator("#save-record").click();
  await page.locator("#research-new-name").fill("Invented draft test");
  await page.locator("#research-save-note").fill("Committed original");
  await page.locator("#research-save-form button[type=submit]").click();
  await page.locator("#close-details").click();
  await page.locator("#research-open").click();
  await page.locator("#research-entries textarea").fill("Recoverable draft");
  await expect(page.locator("#research-draft-status")).toContainText(
    "Drafts saved for recovery",
  );
  // Reload without committing the note; accept the browser's unsaved-note prompt.
  page.on("dialog", (dialog) => dialog.accept());
  await page.reload();
  await page.locator("#research-open").click();
  await expect(page.locator("#research-entries textarea")).toHaveValue(
    "Recoverable draft",
  );
  expect(
    await page.evaluate(
      () =>
        JSON.parse(localStorage.getItem("ukinfo.research.v1")).lists[0]
          .entries[0].note,
    ),
  ).toBe("Committed original");
  const second = await context.newPage();
  await second.goto("/");
  await second.locator("#research-open").click();
  await second.locator("#research-entries textarea").fill("Other tab draft");
  await expect(second.locator("#research-draft-status")).toContainText(
    "Drafts saved for recovery",
  );
  await page
    .locator("#research-entries textarea")
    .fill("Local competing draft");
  await expect(page.locator("#research-draft-status")).toContainText(
    "changed in another window",
  );
  expect(
    await page.evaluate(
      () =>
        JSON.parse(localStorage.getItem("ukinfo.workbench.v1")).drafts[0].note,
    ),
  ).toBe("Other tab draft");
  await second.getByRole("button", { name: "Save note", exact: true }).click();
  await expect(second.locator("#research-status")).toHaveText(
    "Note saved locally.",
  );
  await expect
    .poll(() =>
      second.evaluate(
        () =>
          JSON.parse(localStorage.getItem("ukinfo.workbench.v1")).drafts.length,
      ),
    )
    .toBe(0);
});
