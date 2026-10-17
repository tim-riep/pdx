import { expect, test } from "@playwright/test";
import { dragData, muster } from "./helpers";

test("XSD laden, Felder und Tabelle ziehen, Vorschau wechseln", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("button", { name: "Neu", exact: true }).first().click();
  await expect(page.getByTestId("doc-title")).toHaveText("Neue Vorlage");

  await page.getByRole("tab", { name: "Daten" }).click();
  await page.getByRole("button", { name: "XSD laden" }).click();
  const box = page.getByRole("dialog").getByRole("textbox");
  await box.fill(`${muster}/schema/invoice.xsd`);
  await box.press("Enter");
  const data = page.getByTestId("data");
  await expect(data.locator('[data-path="d:invoice"]')).toBeVisible();

  // Ein einfaches Feld auf die leere Seite ziehen.
  const pageEl = page.locator('[data-page="1"]');
  await dragData(page, data.locator('[data-path="d:invoice/d:number"]'), pageEl, { x: 100, y: 100 });
  await expect(page.locator("[data-sel]")).toHaveAttribute("data-sel", "number");

  // Ein wiederholter Knoten wird zur Tabelle.
  await data.locator('[data-path="d:invoice/d:items"] .twisty').click();
  await dragData(page, data.locator('[data-path="d:invoice/d:items/d:item"]'), pageEl, { x: 100, y: 200 });
  await expect(page.getByTestId("inspector").locator("header strong")).toHaveText("Tabelle");
  await expect(page.getByTestId("inspector").locator('[data-prop="bind"] input')).toHaveValue("d:items/d:item");

  // Entwurf zeigt Platzhalter, die Tabelle hat drei Zeilen.
  await expect(page.getByTestId("render-info")).toContainText("1 Seite");
  await page.getByRole("tab", { name: "Hierarchie" }).click();
  await expect(page.getByTestId("hierarchy").getByText("item", { exact: true })).toBeVisible();

  await page.getByRole("tab", { name: "Vorschau" }).click();
  await expect(page.locator("[data-page]")).toHaveCount(1);
  await page.getByRole("tab", { name: "Entwurf" }).click();
});
