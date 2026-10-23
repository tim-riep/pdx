import { expect, type Locator, type Page } from "@playwright/test";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
export const muster = resolve(here, "../../../testdata/templates/musterrechnung");

// openPath opens a package via the path prompt of the browser.
export async function openPath(page: Page, path: string) {
  await page.getByRole("button", { name: "Öffnen", exact: true }).first().click();
  const box = page.getByRole("dialog").getByRole("textbox");
  await box.fill(path);
  await box.press("Enter");
}

// openMuster opens the sample invoice. preview selects the preview with the
// sample data (three pages), otherwise the draft with placeholders applies.
export async function openMuster(page: Page, preview = false) {
  await page.goto("/");
  await openPath(page, muster);
  await expect(page.getByTestId("doc-title")).toHaveText("musterrechnung");
  if (preview) {
    await page.getByRole("tab", { name: "Vorschau" }).click();
    await expect(page.locator("[data-page]")).toHaveCount(3);
  }
  await expect(page.getByTestId("render-info")).toBeVisible();
  await expect(page.locator('[data-page="1"] canvas')).toBeVisible();
}

// dragData drags an element with HTML5 drag and drop onto a point of a
// target. The events are sent manually with a shared DataTransfer,
// because not every Playwright browser fires them.
export async function dragData(page: Page, source: Locator, target: Locator, pos: { x: number; y: number }) {
  const s = await source.elementHandle();
  const t = await target.elementHandle();
  await page.evaluate(
    ([src, dst, p]) => {
      const dt = new DataTransfer();
      const r = (dst as Element).getBoundingClientRect();
      const at = { clientX: r.left + p.x, clientY: r.top + p.y, bubbles: true, cancelable: true, dataTransfer: dt };
      (src as Element).dispatchEvent(new DragEvent("dragstart", { bubbles: true, cancelable: true, dataTransfer: dt }));
      (dst as Element).dispatchEvent(new DragEvent("dragenter", at));
      (dst as Element).dispatchEvent(new DragEvent("dragover", at));
      (dst as Element).dispatchEvent(new DragEvent("drop", at));
      (src as Element).dispatchEvent(new DragEvent("dragend", { bubbles: true, dataTransfer: dt }));
    },
    [s, t, pos] as const,
  );
}

// selectInTree expands the hierarchy down to an element and selects it.
// ids are the ancestors from top to bottom, the last one is selected.
export async function selectInTree(page: Page, ids: string[]) {
  await page.getByRole("tab", { name: "Hierarchie" }).click();
  const tree = page.getByTestId("hierarchy");
  const row = (id: string) => tree.getByRole("treeitem").filter({ has: page.locator(".id", { hasText: new RegExp(`^${id}$`) }) });
  // Wait for the row first, then expand by aria-expanded. A
  // check for the button before the tree was drawn otherwise skipped the
  // expansion.
  for (const id of ids.slice(0, -1)) {
    const r = row(id);
    await expect(r).toBeVisible();
    if ((await r.getAttribute("aria-expanded")) === "false") await r.getByRole("button", { name: "Aufklappen" }).click();
    await expect(r).toHaveAttribute("aria-expanded", "true");
  }
  await row(ids[ids.length - 1]).locator("button.name").click();
}
