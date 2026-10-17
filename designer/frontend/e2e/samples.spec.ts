import { expect, test } from "@playwright/test";
import { openMuster } from "./helpers";

test("Beispieldaten wählen und Randfälle erzeugen", async ({ page }) => {
  await openMuster(page, true);
  await page.getByRole("tab", { name: "Beispiele" }).click();
  const panel = page.getByTestId("samples");
  const rechnung = panel.locator('[data-sample="samples/rechnung.xml"]');
  await expect(rechnung.getByRole("radio")).toBeChecked();

  // Minimaler Randfall: ohne optionale IBAN scheitert der GiroCode.
  await panel.getByRole("button", { name: "Minimal" }).click();
  const minimal = panel.locator('[data-sample="samples/rand-minimal.xml"]');
  await expect(minimal.getByRole("radio")).toBeChecked();
  const problems = page.getByTestId("problems");
  await expect(problems).toContainText("GiroCode");
  await expect(problems.locator('button[data-ref="girocode"]')).toBeVisible();

  // Zurück zur Rechnung, die Auswahl übersteht einen Wechsel der Ansicht.
  await rechnung.getByRole("radio").check();
  await expect(problems).not.toContainText("GiroCode");
  await page.getByRole("tab", { name: "Entwurf" }).click();
  await page.getByRole("tab", { name: "Vorschau" }).click();
  await expect(page.locator("[data-page]")).toHaveCount(3);
  await expect(rechnung.getByRole("radio")).toBeChecked();

  // Entfernen und Rückgängig.
  await minimal.getByRole("button", { name: "Beispieldaten entfernen" }).click();
  await expect(minimal).toHaveCount(0);
  await page.getByRole("button", { name: "Rückgängig", exact: true }).click();
  await expect(minimal).toBeVisible();
});
