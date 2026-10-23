import { expect, test } from "@playwright/test";
import { openMuster, selectInTree } from "./helpers";

test("Befehlspalette und Verschieben mit Pfeiltasten", async ({ page }) => {
  await openMuster(page);
  const palette = page.getByTestId("command-palette");

  // Go to an element by its ID.
  await page.keyboard.press("ControlOrMeta+k");
  await expect(palette).toBeVisible();
  await page.keyboard.type("adressfenster");
  await expect(palette.getByRole("option").first()).toContainText("adressfenster");
  await page.keyboard.press("Enter");
  await expect(palette).toHaveCount(0);
  const inspector = page.getByTestId("inspector");
  await expect(inspector.locator("header code")).toHaveText("adressfenster");

  // Arrows move by 1 mm, with Shift by 10 mm.
  const x = inspector.locator('[data-prop="x"] input');
  const y = inspector.locator('[data-prop="y"] input');
  await expect(x).toHaveValue("20mm");
  await page.locator("body").press("ArrowRight");
  await expect(x).toHaveValue("21mm");
  await page.locator("body").press("Shift+ArrowDown");
  await expect(y).toHaveValue("55mm");
  await page.getByRole("button", { name: "Rückgängig", exact: true }).click();
  await page.getByRole("button", { name: "Rückgängig", exact: true }).click();
  await expect(x).toHaveValue("20mm");
  await expect(y).toHaveValue("45mm");

  // Run a command: switch the area.
  await page.keyboard.press("ControlOrMeta+k");
  await page.keyboard.type("xpath zeigen");
  await page.keyboard.press("Enter");
  await expect(page.getByTestId("xpath-panel")).toBeVisible();

  // Escape closes without an action.
  await page.keyboard.press("ControlOrMeta+k");
  await page.keyboard.press("Escape");
  await expect(palette).toHaveCount(0);

  // In flowing subforms the arrows move nothing.
  await selectInTree(page, ["inhalt", "titel"]);
  await page.locator("body").press("ArrowRight");
  await expect(page.getByRole("button", { name: "Rückgängig", exact: true })).toBeDisabled();
});
