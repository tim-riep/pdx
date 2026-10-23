import { expect, test } from "@playwright/test";
import { openMuster, selectInTree } from "./helpers";

test("Hierarchie mit der Tastatur bedienen", async ({ page }) => {
  await openMuster(page);
  await selectInTree(page, ["inhalt", "titel"]);
  const code = page.getByTestId("inspector").locator("header code");
  await expect(code).toHaveText("titel");

  await page.keyboard.press("ArrowDown");
  await expect(code).toHaveText("positionen");
  await page.keyboard.press("ArrowRight"); // expand
  await expect(page.getByTestId("hierarchy").locator('[data-row="positionen"]')).toHaveAttribute("aria-expanded", "true");
  await page.keyboard.press("ArrowRight"); // first child
  await expect(code).toHaveText("sp-pos");
  await page.keyboard.press("ArrowLeft"); // to the parent element
  await expect(code).toHaveText("positionen");
  await page.keyboard.press("ArrowLeft"); // collapse
  await expect(page.getByTestId("hierarchy").locator('[data-row="positionen"]')).toHaveAttribute("aria-expanded", "false");
  await page.keyboard.press("ArrowUp");
  await expect(code).toHaveText("titel");

  // Context menu by keyboard.
  await page.keyboard.press("Shift+F10");
  await expect(page.getByTestId("context-menu")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByTestId("context-menu")).toHaveCount(0);
  // Nothing was changed.
  await expect(page.getByRole("button", { name: "Rückgängig", exact: true })).toBeDisabled();
});
