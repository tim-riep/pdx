import { expect, type Locator, type Page } from "@playwright/test";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
export const muster = resolve(here, "../../../testdata/templates/musterrechnung");

// openPath öffnet ein Paket über die Pfadabfrage des Browsers.
export async function openPath(page: Page, path: string) {
  await page.getByRole("button", { name: "Öffnen", exact: true }).first().click();
  const box = page.getByRole("dialog").getByRole("textbox");
  await box.fill(path);
  await box.press("Enter");
}

// openMuster öffnet die Musterrechnung. preview wählt die Vorschau mit den
// Beispieldaten (drei Seiten), sonst gilt der Entwurf mit Platzhaltern.
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

// dragData zieht ein Element mit HTML5-Drag-and-drop auf einen Punkt eines
// Ziels. Die Ereignisse werden mit einem gemeinsamen DataTransfer selbst
// geschickt, weil nicht jeder Browser von Playwright sie auslöst.
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
