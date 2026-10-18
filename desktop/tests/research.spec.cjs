"use strict";
const { test, expect } = require("@playwright/test");
const openRecord = async (page) => {
  await page.goto(
    "/?view=record&record_dataset=synthetic-demo&record_id=1&record_source=invented%3A0",
  );
  await expect(page.locator("#record-title")).toContainText("Person 000");
};
async function saveRecord(page, name, note) {
  await page.getByRole("button", { name: "Save to list", exact: true }).click();
  await page.locator("#research-new-name").fill(name);
  await page.locator("#research-save-note").fill(note);
  await page.getByRole("button", { name: "Save entry", exact: true }).click();
  await expect(page.locator("#research-save")).not.toBeVisible();
}
async function downloadText(page, button) {
  const pending = page.waitForEvent("download");
  await button.click();
  const stream = await (await pending).createReadStream();
  let raw = "";
  for await (const chunk of stream) raw += chunk;
  return raw;
}
test("research lists save notes and citations, compare, export/import and reopen guarded sources", async ({
  page,
}) => {
  await openRecord(page);
  await saveRecord(
    page,
    "Invented family research",
    "Check this source against another edition.",
  );
  await page
    .getByRole("button", { name: "Close details", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Research lists", exact: true })
    .click();
  await expect(page.locator("#research-entries article")).toHaveCount(1);
  await expect(page.locator("#research-entries")).toContainText("invented:0");
  const note = page.locator("#research-entries textarea");
  await note.fill("=Keep this literal research note");
  await page.getByRole("button", { name: "Save note", exact: true }).click();
  await expect(page.locator("#research-status")).toHaveText(
    "Note saved locally.",
  );
  await page.locator("#research-entries input[type=checkbox]").check();
  await page
    .getByRole("button", { name: "Compare selected", exact: false })
    .click();
  await expect(page.locator("#research-comparison")).toContainText(
    "=Keep this literal research note",
  );
  await expect(page.locator("#research-comparison")).toContainText("Invented");
  await page
    .getByRole("button", { name: "Close comparison", exact: true })
    .click();
  const csv = await downloadText(
    page,
    page.getByRole("button", { name: "Export list (CSV)", exact: true }),
  );
  expect(csv).toContain('"\'=Keep this literal research note"');
  const raw = await downloadText(
    page,
    page.getByRole("button", { name: "Export all lists (JSON)", exact: true }),
  );
  const backup = JSON.parse(raw);
  expect(backup.lists[0].entries[0].ref.source_id).toBe("invented:0");
  expect(backup.lists[0].entries[0].note).toBe(
    "=Keep this literal research note",
  );
  await page.locator("#research-dialog details summary").click();
  await page.locator("#research-file").setInputFiles({
    name: "invented-research.json",
    mimeType: "application/json",
    buffer: Buffer.from(raw),
  });
  await expect(page.locator("#research-import-preview")).toContainText(
    "1 lists",
  );
  await page
    .getByRole("button", { name: "Add imported lists", exact: true })
    .click();
  await expect(page.locator("#research-list option")).toHaveCount(2);
  await page.locator("#research-file").setInputFiles({
    name: "bad.json",
    mimeType: "application/json",
    buffer: Buffer.from('{"version":999}'),
  });
  await expect(page.locator("#research-import-preview")).toContainText(
    "Invalid research file",
  );
  await expect(page.locator("#research-import")).toBeDisabled();
  await expect(page.locator("#research-list option")).toHaveCount(2);
  await page.reload();
  await page
    .getByRole("button", { name: "Research lists", exact: true })
    .click();
  await expect(page.locator("#research-entries textarea")).toHaveValue(
    "=Keep this literal research note",
  );
  await page.getByRole("button", { name: "Open source", exact: true }).click();
  await expect(page.locator("#record-title")).toContainText("Person 000");
  await page.getByRole("button", { name: "Save to list", exact: true }).click();
  await page.getByRole("button", { name: "Save entry", exact: true }).click();
  await expect(page.locator("#research-save-status")).toContainText(
    "already in that list",
  );
});
test("research addresses retain scope, safe text and source identity without overwriting notes", async ({
  page,
}) => {
  await openRecord(page);
  await page
    .getByRole("button", { name: "Explore this address", exact: true })
    .click();
  await expect(page.locator("#address-status")).toContainText("source listing");
  await page.locator("#address-scope").selectOption("all");
  await expect(page.locator("#address-status")).toContainText(
    "End of listings",
  );
  await page.getByRole("button", { name: "Save address", exact: true }).click();
  await page.locator("#research-new-name").fill("<img src=x onerror=alert(1)>");
  await page.getByRole("button", { name: "Save entry", exact: true }).click();
  await page
    .getByRole("button", { name: "Close address", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Research lists", exact: true })
    .click();
  await expect(page.locator("#research-entries")).toContainText(
    "Address: Person 000",
  );
  await expect(page.locator("#research-dialog img")).toHaveCount(0);
  await page.getByRole("button", { name: "Open source", exact: true }).click();
  await expect(page.locator("#address-scope")).toHaveValue("all");
  await page
    .getByRole("button", { name: "Close address", exact: true })
    .click();
  await page.evaluate(() => {
    const value = JSON.parse(localStorage.getItem("ukinfo.research.v1"));
    value.lists[0].entries[0].ref.source_id = "changed";
    localStorage.setItem("ukinfo.research.v1", JSON.stringify(value));
  });
  await page
    .getByRole("button", { name: "Research lists", exact: true })
    .click();
  await page.getByRole("button", { name: "Open source", exact: true }).click();
  await expect(page.locator("#research-status")).toContainText(
    "no longer identifies",
  );
  await expect(page.locator("#research-dialog")).toBeVisible();
});
test("another tab cannot silently overwrite a note draft", async ({
  page,
  context,
}) => {
  await openRecord(page);
  await saveRecord(page, "Shared local list", "Original note");
  await page
    .getByRole("button", { name: "Close details", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Research lists", exact: true })
    .click();
  await page
    .locator("#research-entries textarea")
    .fill("Unsaved first-tab draft");
  const other = await context.newPage();
  await other.goto("/");
  await other
    .getByRole("button", { name: "Research lists", exact: true })
    .click();
  await other
    .locator("#research-entries textarea")
    .fill("Saved second-tab note");
  await other.getByRole("button", { name: "Save note", exact: true }).click();
  await expect(other.locator("#research-status")).toHaveText(
    "Note saved locally.",
  );
  await page.getByRole("button", { name: "Save note", exact: true }).click();
  await expect(page.locator("#research-status")).toContainText(
    "changed in another window",
  );
  await expect(page.locator("#research-entries textarea")).toHaveValue(
    "Unsaved first-tab draft",
  );
  await page.getByRole("button", { name: "Reload lists", exact: true }).click();
  await expect(page.locator("#research-entries textarea")).toHaveValue(
    "Unsaved first-tab draft",
  );
  // Discard the test's unsaved draft explicitly before closing its context.
  await page
    .locator("#research-entries textarea")
    .fill("Saved second-tab note");
});

test("edits made during a pending note save remain unsaved drafts", async ({
  page,
}) => {
  await openRecord(page);
  await saveRecord(page, "Pending note test", "Original");
  await page
    .getByRole("button", { name: "Close details", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Research lists", exact: true })
    .click();
  await page.evaluate(() => {
    const original = navigator.locks.request.bind(navigator.locks);
    navigator.locks.request = (name, callback) =>
      original(name, async () => {
        await new Promise((resolve) => (window.releaseResearchWrite = resolve));
        return callback();
      });
  });
  await page.locator("#research-entries textarea").fill("First save");
  await page.getByRole("button", { name: "Save note", exact: true }).click();
  await expect
    .poll(() => page.evaluate(() => typeof window.releaseResearchWrite))
    .toBe("function");
  await page.locator("#research-entries textarea").fill("Later draft");
  await page.evaluate(() => window.releaseResearchWrite());
  await expect(page.locator("#research-status")).toHaveText(
    "Note saved locally.",
  );
  await expect(page.locator("#research-entries textarea")).toHaveValue(
    "Later draft",
  );
  await expect(
    page.getByRole("button", { name: "Save note", exact: true }),
  ).toBeEnabled();
  await page.locator("#research-entries textarea").fill("First save");
});

test("unavailable browser locking reports a save error without writing lists", async ({
  page,
}) => {
  await openRecord(page);
  await page.evaluate(() =>
    Object.defineProperty(navigator, "locks", { value: undefined }),
  );
  await page.getByRole("button", { name: "Save to list", exact: true }).click();
  await page.locator("#research-new-name").fill("Unsupported browser test");
  await page.getByRole("button", { name: "Save entry", exact: true }).click();
  await expect(page.locator("#research-save-status")).toContainText(
    "Web Locks support",
  );
  expect(
    await page.evaluate(() => localStorage.getItem("ukinfo.research.v1")),
  ).toBeNull();
});
