import { expect, test } from "@playwright/test";
import { openMuster } from "./helpers";

test("Klick im Canvas wählt das Vorlagenelement", async ({ page }) => {
  await openMuster(page);
  await expect(page.locator('[data-page="1"] canvas')).toBeVisible();
  // The address window lies at 20 mm to 105 mm horizontally and 45 mm to
  // 90 mm vertically. A point inside is 60 mm, 75 mm (170 pt, 213 pt).
  // The zoom can still change after opening, so read and
  // click until the selection is set.
  await expect(async () => {
    const zoom = parseFloat((await page.getByTestId("zoom").textContent())!) / 100;
    await page.locator('[data-page="1"]').click({ position: { x: 170 * zoom, y: 213 * zoom } });
    await expect(page.locator("[data-sel]")).toHaveAttribute("data-sel", /adressfenster|empfaenger/, { timeout: 1000 });
  }).toPass({ timeout: 15_000 });
  const inspector = page.getByTestId("inspector");
  await expect(inspector.locator("code")).toHaveText(/adressfenster|empfaenger/);
  // The hierarchy shows the selection.
  await expect(page.getByTestId("hierarchy").locator('[aria-selected="true"]')).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.locator("[data-sel]")).toHaveCount(0);
});

test("Auswahl in der Hierarchie zeigt Eigenschaften und alle Vorkommen", async ({ page }) => {
  await openMuster(page, true);
  const tree = page.getByTestId("hierarchy");
  await tree.getByRole("treeitem").filter({ hasText: "Inhalt" }).getByRole("button", { name: "Aufklappen" }).click();
  await tree.getByRole("button", { name: /Tabelle\s*positionen/ }).click();
  const inspector = page.getByTestId("inspector");
  await expect(inspector.locator("header strong")).toHaveText("Tabelle");
  await expect(inspector.locator('[data-prop="bind"] input')).toHaveValue(/inv:/);
  // Search filters the properties.
  await inspector.getByRole("searchbox").fill("bindung");
  await expect(inspector.locator("[data-prop]")).toHaveCount(1);
  // The table runs over three pages, each page shows one occurrence.
  await expect(page.locator(".box.related")).toHaveCount(2);
});
