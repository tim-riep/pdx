import { expect, test } from "@playwright/test";
import { dragData, muster } from "./helpers";

test("Tabelle mit Spalte, Kopf, Fuß, verbundener Zelle und Summe", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("button", { name: "Neu", exact: true }).first().click();
  await expect(page.getByTestId("doc-title")).toHaveText("Neue Vorlage");
  await expect(page.locator('[data-page="1"] canvas')).toBeVisible();
  await page.getByRole("tab", { name: "Daten" }).click();
  await page.getByRole("button", { name: "XSD laden" }).click();
  const box = page.getByRole("dialog").getByRole("textbox");
  await box.fill(`${muster}/schema/invoice.xsd`);
  await box.press("Enter");
  const data = page.getByTestId("data");
  await data.locator('[data-path="d:invoice/d:items"] .twisty').click();
  await dragData(page, data.locator('[data-path="d:invoice/d:items/d:item"]'), page.locator('[data-page="1"]'), { x: 100, y: 200 });

  const inspector = page.getByTestId("inspector");
  const tools = page.getByTestId("table-tools");
  await expect(inspector.locator("header strong")).toHaveText("Tabelle");

  // Der Kopf entsteht schon beim Ziehen. Spalte und Fuß ergänzen.
  await expect(tools.getByRole("button", { name: "Kopf hinzufügen" })).toHaveCount(0);
  await tools.getByRole("button", { name: "Spalte rechts" }).click();
  await tools.getByRole("button", { name: "Fuß hinzufügen" }).click();
  await expect(tools.getByRole("button", { name: "Fuß hinzufügen" })).toHaveCount(0);

  await page.getByRole("tab", { name: "Hierarchie" }).click();
  const tree = page.getByTestId("hierarchy");
  for (const id of ["item-fuss", "item-fuss-zeile"]) {
    await tree.getByRole("treeitem").filter({ has: page.locator(".id", { hasText: new RegExp(`^${id}$`) }) }).getByRole("button", { name: "Aufklappen" }).click();
  }
  const cells = page.getByTestId("hierarchy").locator("button.name", { hasText: "Zelle" }).filter({ hasText: /fuss/ });
  await expect(cells).toHaveCount(6);

  // Über der Beschreibung lässt sich nichts summieren.
  await cells.nth(1).click();
  await expect(inspector.locator("header strong")).toHaveText("Zelle");
  await tools.getByRole("button", { name: "Summe", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("summieren");

  // Über dem Betrag schon.
  await cells.nth(4).click();
  await tools.getByRole("button", { name: "Summe", exact: true }).click();
  await expect(inspector.locator("header strong")).toHaveText("Summe");
  await expect(inspector.locator('[data-prop="of"] input')).toHaveValue(/amount/);

  // Die erste Fußzelle über zwei Spalten, Rückgängig stellt es wieder her.
  await cells.nth(0).click();
  const span = tools.getByRole("spinbutton");
  await span.fill("2");
  await span.press("Tab");
  await expect(inspector.locator('[data-prop="colspan"] input')).toHaveValue("2");
  await expect(cells).toHaveCount(5);
  await page.getByRole("button", { name: "Rückgängig", exact: true }).click();
  await expect(cells).toHaveCount(6);
});
