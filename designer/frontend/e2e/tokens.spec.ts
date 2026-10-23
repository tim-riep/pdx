import { expect, test } from "@playwright/test";
import { openMuster } from "./helpers";

test("Tokens anlegen, umbenennen und in Stilen verwenden", async ({ page }) => {
  await openMuster(page);
  await page.getByRole("tab", { name: "Stile" }).click();
  const panel = page.getByTestId("styles");
  await expect(panel).toContainText("Noch keine Tokens");

  // Create a color token, change value and name.
  await panel.getByRole("button", { name: "+ Farbe" }).click();
  const row = panel.locator('[data-token="farbe"]');
  await expect(row).toBeVisible();
  const value = row.getByRole("textbox", { name: "Wert" });
  await value.fill("#aa0000");
  await value.press("Enter");
  await expect(value).toHaveValue("#aa0000");
  const name = row.getByRole("textbox", { name: "Name" });
  await name.fill("primaer");
  await name.press("Enter");
  const primaer = panel.locator('[data-token="primaer"]');
  await expect(primaer).toBeVisible();

  // Open a style and set the token as color.
  await panel.getByRole("button", { name: /st-titel/ }).click();
  const inspector = page.getByTestId("inspector");
  await expect(inspector.locator("header code")).toHaveText("st-titel");
  const color = inspector.locator('[data-prop="color"] input');
  await expect(color).toHaveAttribute("list", "tok-color");
  await expect(page.locator("#tok-color option")).toHaveAttribute("value", "$primaer");
  await color.fill("$primaer");
  await color.press("Enter");
  await expect(inspector.locator('[data-prop="color"] .swatch')).toHaveCSS("background-color", "rgb(170, 0, 0)");

  // In use: removing locked, layout without errors.
  await page.getByRole("tab", { name: "Stile" }).click();
  await expect(primaer.locator(".used")).toHaveText("1×");
  await expect(primaer.getByRole("button", { name: "Token entfernen" })).toBeDisabled();
  await expect(page.getByTestId("problems").locator("li.error")).toHaveCount(0);
  await expect(page.getByTestId("render-info")).toContainText("Seite");

  // Renaming updates the reference in the style.
  await primaer.getByRole("textbox", { name: "Name" }).fill("marke");
  await primaer.getByRole("textbox", { name: "Name" }).press("Enter");
  await panel.getByRole("button", { name: /st-titel/ }).click();
  await expect(color).toHaveValue("$marke");

  // Undo: name is primaer again.
  await page.getByRole("button", { name: "Rückgängig", exact: true }).click();
  await expect(color).toHaveValue("$primaer");
});
