import { expect, test } from "@playwright/test";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { openPath, selectInTree } from "./helpers";

const cii = resolve(dirname(fileURLToPath(import.meta.url)), "../../../testdata/templates/cii-rechnung");

// E-Rechnung: der Inspektor zeigt die aus den Daten erkannte Stufe und
// warnt, wenn das Ausgabeprofil nicht PDF/A-3 ist.
test("E-Rechnung: Stufe aus den Daten und Profilwarnung", async ({ page }) => {
  await page.goto("/");
  await openPath(page, cii);
  await expect(page.getByTestId("doc-title")).toHaveText("cii-rechnung");
  await page.getByRole("tab", { name: "Vorschau" }).click();
  await selectInTree(page, ["erechnung"]);
  const info = page.getByTestId("einvoice-info");
  await expect(info.getByTestId("einvoice-level")).toHaveText("EN 16931");
  await expect(info).toContainText("factur-x.xml");
  await expect(info).toContainText("Alternative (nach Stufe)");
  await expect(info.getByTestId("einvoice-profile")).toHaveCount(0);

  // Profil auf PDF/A-4f stellen: Warnung.
  await selectInTree(page, ["cii-rechnung"]);
  await page.getByTestId("profile-select").selectOption("pdfa-4f,pdfua-2");
  await selectInTree(page, ["erechnung"]);
  await expect(info.getByTestId("einvoice-profile")).toContainText("verlangen PDF/A-3");
  await page.getByRole("button", { name: "Rückgängig", exact: true }).click();
  await expect(info.getByTestId("einvoice-profile")).toHaveCount(0);
});
