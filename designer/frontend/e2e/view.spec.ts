import { expect, test } from "@playwright/test";
import { openMuster, selectInTree } from "./helpers";

test("Lineale, Einheit pt und Seitenübersicht", async ({ page }) => {
  await openMuster(page, true);
  const first = page.locator('[data-page="1"]');
  await expect(first.locator('[data-ruler="x"]')).toBeVisible();
  await expect(first.locator('[data-ruler="y"]')).toBeVisible();
  await expect(first.locator('[data-ruler="x"]')).toContainText("100");

  // Rulers off and on again.
  await page.getByLabel("Lineale", { exact: true }).uncheck();
  await expect(first.locator("[data-ruler]")).toHaveCount(0);
  await page.getByLabel("Lineale", { exact: true }).check();

  // Page overview: three pages, a click jumps to the third.
  await page.getByLabel("Seiten", { exact: true }).check();
  const thumbs = page.getByTestId("page-thumbs");
  await expect(thumbs.getByRole("button")).toHaveCount(3);
  await thumbs.getByRole("button", { name: "Seite 3" }).click();
  await expect(thumbs.getByRole("button", { name: "Seite 3" })).toHaveAttribute("aria-current", "page");
  await expect.poll(() => page.locator("[data-viewport]").evaluate((e) => e.scrollTop)).toBeGreaterThan(500);

  // Unit pt: ruler in pt, arrow key moves by 1 pt.
  await page.getByLabel("Einheit").selectOption("pt");
  await page.getByRole("tab", { name: "Entwurf" }).click();
  await selectInTree(page, ["seiten", "erste", "erste-kopf", "adressfenster"]);
  const x = page.getByTestId("inspector").locator('[data-prop="x"] input');
  await expect(x).toHaveValue("20mm");
  // Arrows navigate in the hierarchy, so release the focus there.
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
  await page.keyboard.press("ArrowRight");
  await expect(x).toHaveValue("57.5pt");
  await page.getByRole("button", { name: "Rückgängig", exact: true }).click();
  await expect(x).toHaveValue("20mm");

  // The choice is kept after reloading.
  await page.reload();
  await expect(page.getByLabel("Einheit")).toHaveValue("pt");
  await expect(page.getByLabel("Seiten", { exact: true })).toBeChecked();
});
