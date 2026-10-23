import { expect, test } from "@playwright/test";
import { openMuster, selectInTree } from "./helpers";

// Läuft mit der veraPDF-Attrappe aus e2e/fixtures (siehe playwright.config.ts).
test("Konformität: Befunde nach Regeln, Klick führt zum Element", async ({ page }) => {
  await openMuster(page, true);
  await page.getByRole("tab", { name: "Konformität" }).click();
  const panel = page.getByTestId("conformance");
  await panel.getByRole("button", { name: "Konformität prüfen" }).click();
  const verdict = panel.getByTestId("conformance-verdict");
  await expect(verdict).toContainText("Nicht konform, 2 Regeln verletzt");
  await expect(verdict).toContainText("veraPDF lokal");
  await expect(verdict).toContainText("rechnung.xml");

  // Regel aufklappen, Befunde mit Element und ohne Seite.
  await panel.getByRole("button", { name: /Attrappe: Farbraum/ }).click();
  const withRef = panel.locator("button.item:not([disabled])");
  await expect(withRef.first()).toBeVisible();
  const ref = await withRef.first().getAttribute("data-ref");
  expect(ref).toBeTruthy();
  await withRef.first().click();
  await expect(page.getByTestId("inspector").locator("header code")).toHaveText(ref!);

  await panel.getByRole("button", { name: /Attrappe: Metadaten/ }).click();
  await expect(panel.locator("button.item", { hasText: "Attrappe ohne Seitenbezug" })).toContainText("Dokument");

  // Nach einer Änderung ist das Ergebnis veraltet.
  await selectInTree(page, ["inhalt", "titel"]);
  const space = page.getByTestId("inspector").locator('[data-prop="spaceAfter"] input');
  await space.fill("6mm");
  await space.press("Enter");
  await expect(panel).toContainText("Veraltet, die Vorlage hat sich seitdem geändert.");
  await page.getByRole("button", { name: "Rückgängig", exact: true }).click();
});

// Die Einstellung veraPDF zeigt das gefundene Programm, übernimmt einen
// gültigen Pfad, lehnt einen ungültigen mit Meldung ab und schaltet zurück.
test("veraPDF-Pfad einstellen", async ({ page }) => {
  test.skip(!process.env.PDX_E2E_VERAPDF, "nur mit Attrappe oder Pfad");
  await openMuster(page);
  await page.getByRole("tab", { name: "Konformität" }).click();
  const box = page.getByTestId("verapdf-settings");
  await box.locator("summary").click();
  await expect(box.getByTestId("verapdf-status")).toContainText("veraPDF lokal");
  const stub = process.env.PDX_E2E_VERAPDF!;
  await box.getByTestId("verapdf-path").fill(stub);
  await box.getByTestId("verapdf-apply").click();
  await expect(box.getByTestId("verapdf-status")).toContainText(stub);
  await box.getByTestId("verapdf-path").fill(stub + "-fehlt");
  await box.getByTestId("verapdf-apply").click();
  await expect(box.getByRole("alert")).toBeVisible();
  await box.getByTestId("verapdf-auto").click();
  await expect(box.getByTestId("verapdf-path")).toHaveValue("");
  await expect(box.getByTestId("verapdf-status")).toContainText("veraPDF lokal");
});
