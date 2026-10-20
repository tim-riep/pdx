import { expect, test } from "@playwright/test";
import { openMuster, selectInTree } from "./helpers";

async function selectAddress(page: import("@playwright/test").Page) {
  // Rand des Adressfensters, außerhalb des inneren Teilformulars. Der Zoom
  // kann sich nach dem Öffnen noch ändern (Breite einpassen), daher Zoom
  // lesen und klicken, bis die Auswahl steht.
  let zoom = 1;
  await expect(async () => {
    zoom = parseFloat((await page.getByTestId("zoom").textContent())!) / 100;
    await page.locator('[data-page="1"]').click({ position: { x: 62 * zoom, y: 248 * zoom } });
    await expect(page.locator("[data-sel]")).toHaveAttribute("data-sel", "adressfenster", { timeout: 1000 });
  }).toPass({ timeout: 15_000 });
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

test("Baustein Text bei gewähltem Text kommt dahinter, nicht hinein", async ({ page }) => {
  await openMuster(page);
  await selectInTree(page, ["inhalt", "titel"]);
  await page.getByTestId("palette").locator('[data-kind="text"]').click();
  await expect(page.getByRole("alert")).toHaveCount(0);
  await expect(page.getByTestId("inspector").locator("header strong")).toHaveText("Text");
  await expect(page.getByTestId("inspector").locator("header code")).not.toHaveText("titel");
});
