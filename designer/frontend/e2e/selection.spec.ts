import { expect, test } from "@playwright/test";
import { openMuster } from "./helpers";

test("Klick im Canvas wählt das Vorlagenelement", async ({ page }) => {
  await openMuster(page);
  await expect(page.locator('[data-page="1"] canvas')).toBeVisible();
  // Das Adressfenster liegt bei 20 mm bis 105 mm waagerecht und 45 mm bis
  // 90 mm senkrecht. Ein Punkt darin ist 60 mm, 75 mm (170 pt, 213 pt).
  const zoom = parseFloat((await page.getByTestId("zoom").textContent())!) / 100;
  await page.locator('[data-page="1"]').click({ position: { x: 170 * zoom, y: 213 * zoom } });
  await expect(page.locator("[data-sel]")).toHaveAttribute("data-sel", /adressfenster|empfaenger/);
  const inspector = page.getByTestId("inspector");
  await expect(inspector.locator("code")).toHaveText(/adressfenster|empfaenger/);
  // Die Hierarchie zeigt die Auswahl.
  await expect(page.getByTestId("hierarchy").locator('[aria-selected="true"]')).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.locator("[data-sel]")).toHaveCount(0);
});

test("Auswahl in der Hierarchie zeigt Eigenschaften und alle Vorkommen", async ({ page }) => {
  await openMuster(page);
  const tree = page.getByTestId("hierarchy");
  await tree.getByRole("treeitem").filter({ hasText: "Inhalt" }).getByRole("button", { name: "Aufklappen" }).click();
  await tree.getByRole("button", { name: /Tabelle\s*positionen/ }).click();
  const inspector = page.getByTestId("inspector");
  await expect(inspector.locator("header strong")).toHaveText("Tabelle");
  await expect(inspector.locator('[data-prop="bind"] input')).toHaveValue(/inv:/);
  // Suche filtert die Eigenschaften.
  await inspector.getByRole("searchbox").fill("bindung");
  await expect(inspector.locator("[data-prop]")).toHaveCount(1);
  // Die Tabelle läuft über drei Seiten, jede Seite zeigt ein Vorkommen.
  await expect(page.locator(".box.related")).toHaveCount(2);
});
