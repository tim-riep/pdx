import { expect, test } from "@playwright/test";
import { openMuster, selectInTree } from "./helpers";

test("Text mit Feld-Chips bearbeiten: Inspektor, @ und Doppelklick", async ({ page }) => {
  await openMuster(page);
  await selectInTree(page, ["seiten", "erste", "erste-kopf", "rechnungsdaten", "rechnungsnummer"]);
  const editor = page.getByTestId("inspector-content").getByTestId("content-editor");
  await expect(editor).toContainText("Rechnungsnummer:");
  await expect(editor.locator(".chip")).toHaveText("inv:number");

  // Add text and apply.
  // Click at the start (a click on a chip selects all of it), then go to the end.
  await editor.click({ position: { x: 4, y: 4 } });
  await page.keyboard.press("End");
  await page.keyboard.type(" vom ");
  // Insert a field with @.
  await page.keyboard.type("@issue");
  const menu = page.getByTestId("content-menu");
  await expect(menu).toContainText("inv:issueDate");
  await page.keyboard.press("Enter");
  await expect(editor.locator(".chip")).toHaveCount(2);
  await page.keyboard.press("Enter"); // applies
  await expect(page.getByRole("button", { name: "Rückgängig", exact: true })).toBeEnabled();
  // After applying, the editor rebuilds from the model: the new
  // field has an ID.
  await expect(editor.locator('.chip[data-ref="rechnungsnummer-issueDate"]')).toHaveText("inv:issueDate");
  await expect(editor).toContainText("vom");

  // Undo reverts both.
  await page.getByRole("button", { name: "Rückgängig", exact: true }).click();
  await expect(editor.locator(".chip")).toHaveCount(1);
  await expect(editor).not.toContainText("vom");

  // A double click in the canvas opens the editor at the text, Escape discards. The target
  // is the center of the selection frame, not a point computed from the zoom.
  // The zoom can still change on slow machines.
  const frame = page.locator('[data-page="1"] [data-sel="rechnungsnummer"]');
  await expect(frame).toBeVisible();
  const box = (await frame.boundingBox())!;
  await page.mouse.dblclick(box.x + box.width / 2, box.y + box.height / 2);
  const inline = page.getByTestId("inline-edit").getByTestId("content-editor");
  await expect(inline).toBeVisible();
  await expect(inline).toBeFocused();
  await expect(inline.locator(".chip")).toHaveText("inv:number");
  await page.keyboard.type("XYZ");
  await page.keyboard.press("Escape");
  await expect(page.getByTestId("inline-edit")).toHaveCount(0);
  await expect(editor).not.toContainText("XYZ");
});

// Slow backend (as on CI machines): typing continues while the fields
// load after the @. The menu shows the state of the last
// input, not that of an earlier query.
test("@-Menü bei langsamem Backend", async ({ page }) => {
  await openMuster(page);
  await selectInTree(page, ["seiten", "erste", "erste-kopf", "rechnungsdaten", "rechnungsnummer"]);
  const editor = page.getByTestId("inspector-content").getByTestId("content-editor");
  await expect(editor.locator(".chip")).toHaveText("inv:number");
  await page.route("**/wails/runtime**", async (route) => {
    await new Promise((r) => setTimeout(r, 400));
    await route.continue();
  });
  await editor.click({ position: { x: 4, y: 4 } });
  await page.keyboard.press("End");
  await page.keyboard.type(" @inv:issueDa", { delay: 30 });
  await expect(page.getByTestId("content-menu").getByRole("option").first()).toHaveText("inv:issueDate");
  await page.unroute("**/wails/runtime**");
  await page.keyboard.press("Escape");
});

// Chromium on Linux and Windows puts the cursor into the field with End
// at the end of the line. The editor moves it out, the input arrives.
test("Cursor im Feld landet dahinter", async ({ page }) => {
  await openMuster(page);
  await selectInTree(page, ["seiten", "erste", "erste-kopf", "rechnungsdaten", "rechnungsnummer"]);
  const editor = page.getByTestId("inspector-content").getByTestId("content-editor");
  await expect(editor.locator(".chip")).toHaveText("inv:number");
  await editor.click({ position: { x: 4, y: 4 } });
  await editor.evaluate((el) => {
    const t = el.querySelector(".chip")!.firstChild!;
    const r = document.createRange();
    r.setStart(t, t.textContent!.length);
    r.collapse(true);
    getSelection()!.removeAllRanges();
    getSelection()!.addRange(r);
  });
  await page.keyboard.type(" vom @issue");
  await expect(page.getByTestId("content-menu")).toContainText("inv:issueDate");
  await page.keyboard.press("Escape");
  await expect(editor).toContainText("vom");
});
