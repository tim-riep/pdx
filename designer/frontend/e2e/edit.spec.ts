import { expect, test } from "@playwright/test";
import { openMuster } from "./helpers";

async function selectAddress(page: import("@playwright/test").Page) {
  const zoom = parseFloat((await page.getByTestId("zoom").textContent())!) / 100;
  // Rand des Adressfensters, außerhalb des inneren Teilformulars.
  await page.locator('[data-page="1"]').click({ position: { x: 62 * zoom, y: 248 * zoom } });
  await expect(page.locator("[data-sel]")).toHaveAttribute("data-sel", "adressfenster");
  return zoom;
}

test("Eigenschaft ändern, rückgängig machen und wiederholen", async ({ page }) => {
  await openMuster(page);
  const zoom = await selectAddress(page);
  const sel = page.locator(".box.sel");
  const left = async () => (await sel.boundingBox())!.x;
  const before = await left();

  const x = page.getByTestId("inspector").locator('[data-prop="x"] input');
  await expect(x).toHaveValue("20mm");
  await x.fill("40mm");
  await x.press("Enter");
  await expect(page.getByTestId("doc-title")).toContainText("geändert");
  // Der Canvas zeigt das Ergebnis der Engine: 20 mm weiter rechts.
  await expect.poll(left).toBeGreaterThan(before + 50 * zoom);

  await page.keyboard.press("ControlOrMeta+z");
  await expect(x).toHaveValue("20mm");
  await expect.poll(left).toBeLessThan(before + 2);
  await expect(page.getByTestId("doc-title")).not.toContainText("geändert");

  await page.keyboard.press("ControlOrMeta+Shift+z");
  await expect(x).toHaveValue("40mm");
  await page.keyboard.press("ControlOrMeta+z");
});

test("Ungültiger Wert ändert nichts und meldet den Fehler", async ({ page }) => {
  await openMuster(page);
  await selectAddress(page);
  const x = page.getByTestId("inspector").locator('[data-prop="x"] input');
  await x.fill("zwanzig");
  await x.press("Enter");
  await expect(page.getByRole("alert")).toBeVisible();
  await expect(x).toHaveValue("20mm");
});

test("Entf entfernt das ausgewählte Element", async ({ page }) => {
  await openMuster(page);
  await selectAddress(page);
  await page.keyboard.press("Delete");
  await expect(page.locator("[data-sel]")).toHaveCount(0);
  await expect(page.getByTestId("hierarchy").getByText("adressfenster")).toHaveCount(0);
  await page.keyboard.press("ControlOrMeta+z");
  await expect(page.getByTestId("hierarchy").getByText("adressfenster", { exact: true })).toBeVisible();
});
