import { expect, test } from "@playwright/test";
import { openMuster, selectInTree } from "./helpers";

test("Ausrichten am Elternelement und an mehreren Elementen", async ({ page }) => {
  await openMuster(page);
  await selectInTree(page, ["seiten", "erste", "erste-kopf", "adressfenster"]);
  const inspector = page.getByTestId("inspector");
  const tools = inspector.getByTestId("align-tools");
  const y = inspector.locator('[data-prop="y"] input');
  await expect(y).toHaveValue("45mm");

  // Allein: am Kopf ausrichten.
  await tools.getByRole("button", { name: "Oben", exact: true }).click();
  await expect(y).toHaveValue("0mm");
  await page.getByRole("button", { name: "Rückgängig", exact: true }).click();
  await expect(y).toHaveValue("45mm");

  // Umschalt+Klick auf die Rechnungsdaten (Treffer im Text darin).
  const zoom = parseFloat((await page.getByTestId("zoom").textContent())!) / 100;
  const mm = (72 / 25.4) * zoom;
  await page.locator('[data-page="1"]').click({ position: { x: 130 * mm, y: 52 * mm }, modifiers: ["Shift"] });
  await expect(page.locator('[data-extra="rechnungsdaten"]')).toBeVisible();
  await expect(tools).toContainText("2 Elemente");
  await tools.getByRole("button", { name: "Oben", exact: true }).click();
  await expect(page.getByRole("button", { name: "Rückgängig", exact: true })).toBeEnabled();

  // Die Rechnungsdaten stehen jetzt auf der Höhe des Adressfensters.
  await selectInTree(page, ["seiten", "erste", "erste-kopf", "rechnungsdaten"]);
  await expect(y).toHaveValue("45mm");
  // Neue Auswahl leert die Mehrfachauswahl.
  await expect(page.locator("[data-extra]")).toHaveCount(0);
});
