import { expect, test } from "@playwright/test";
import { openMuster, selectInTree } from "./helpers";

test("Problemliste meldet Fehler und führt zum Element", async ({ page }) => {
  await openMuster(page, true);
  const panel = page.getByTestId("problems");
  await expect(panel).toBeVisible();
  const entry = panel.locator('button[data-ref="z-betrag-f"]');
  await expect(entry).toHaveCount(0);

  // Enter a wrong binding.
  await selectInTree(page, ["inhalt", "positionen", "positionen-zeilen", "position", "z-betrag", "z-betrag-f"]);
  const bind = page.getByTestId("inspector").locator('[data-prop="bind"] input');
  await bind.fill("inv:betrag ");
  await bind.evaluate((e) => (e as HTMLInputElement).blur());
  await expect(entry.first()).toBeVisible();
  await expect(entry.first()).toContainText("Feld");
  await expect(page.getByRole("tab", { name: /^Probleme \(\d+\)$/ })).toBeVisible();

  // Select another element, a click on the finding leads back.
  await selectInTree(page, ["inhalt", "titel"]);
  await expect(page.getByTestId("inspector").locator("header code")).toHaveText("titel");
  await entry.first().click();
  await expect(page.getByTestId("inspector").locator("header code")).toHaveText("z-betrag-f");
  // Without matching data the field sets nothing, the hierarchy still shows it.
  await expect(page.getByTestId("hierarchy").locator('[aria-selected="true"] .id')).toHaveText("z-betrag-f");

  // Undo: the finding disappears.
  await page.getByRole("button", { name: "Rückgängig", exact: true }).click();
  await expect(entry).toHaveCount(0);
});
