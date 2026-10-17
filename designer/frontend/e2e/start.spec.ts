import { expect, test } from "@playwright/test";

test("Designer startet und spricht mit dem Backend", async ({ page }) => {
  await page.goto("/");
  await expect(page).toHaveTitle(/pdx Designer$/);
  await expect(page.getByTestId("status")).toContainText("Engine");
  await expect(page.getByRole("tab", { name: "Entwurf" })).toHaveAttribute("aria-selected", "true");
  await page.getByRole("tab", { name: "Vorschau" }).click();
  await expect(page.getByRole("tab", { name: "Vorschau" })).toHaveAttribute("aria-selected", "true");
});
