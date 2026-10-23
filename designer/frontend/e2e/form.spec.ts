import { expect, test } from "@playwright/test";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { openPath, selectInTree } from "./helpers";

const formular = resolve(dirname(fileURLToPath(import.meta.url)), "../../../testdata/templates/formular");

// Forms in the designer: fillable preview, tab order, inserting
// from the palette with properties in the inspector.
test("Formular: Vorschau, Tab-Reihenfolge, Palette", async ({ page }) => {
  await page.goto("/");
  await openPath(page, formular);
  await expect(page.getByTestId("doc-title")).toHaveText("formular");
  await page.getByRole("tab", { name: "Vorschau" }).click();

  // Fillable fields above the widgets, without the signature field.
  const fill = page.getByTestId("fill");
  await expect(fill.locator("[data-field]")).toHaveCount(11);
  const name = fill.locator('input[data-field="name"]');
  await expect(name).toHaveValue("Erika Musterfrau");
  await name.fill("Alex Beispiel");
  await expect(name).toHaveValue("Alex Beispiel");
  await fill.getByRole("radio", { name: "Anrede Herr" }).check();
  await expect(fill.getByRole("radio", { name: "Anrede Frau" })).not.toBeChecked();
  await expect(fill.locator('select[data-field="land"]')).toHaveValue("DE");

  // Tab order by structure tree: salutation first, signature last.
  await page.getByRole("tab", { name: "Barrierefreiheit" }).click();
  await page.getByTestId("access-tabs").check();
  await expect(page.locator('.order.tab[data-ref="anrede"]').first()).toHaveAttribute("data-tab", "1");
  await expect(page.locator('.order.tab[data-ref="unterschrift"]')).toHaveAttribute("data-tab", "9");

  // Insert a text field from the palette: inspector with form field and
  // label under accessibility.
  await page.getByRole("tab", { name: "Entwurf" }).click();
  await selectInTree(page, ["inhalt", "unterschrift"]);
  await page.getByTestId("palette").locator('[data-kind="textField"]').click();
  const inspector = page.getByTestId("inspector");
  await expect(inspector.locator("header strong")).toHaveText("Textfeld");
  await expect(inspector.getByText("Formularfeld", { exact: true })).toBeVisible();
  await expect(inspector.locator('[data-prop="label"] input')).toHaveValue("Textfeld");
  await expect(inspector.locator('[data-prop="multiline"]')).toBeVisible();
  await page.getByRole("button", { name: "Rückgängig", exact: true }).click();
});
