import { expect, test } from "@playwright/test";

test("Über zeigt Version und Drittanbieterhinweise", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("button", { name: /Designer .*Engine/ }).click();
  const about = page.getByTestId("about");
  await expect(about).toBeVisible();
  const notices = about.getByTestId("notices");
  for (const name of ["pdx (Engine)", "github.com/wailsapp/wails/v3", "github.com/tim-riep/go-xslt", "pdfjs-dist", "svelte", "Go (Standardbibliothek und Laufzeit)", "Liberation"]) {
    await expect(notices).toContainText(name);
  }
  await page.getByRole("button", { name: "Schließen" }).click();
  await expect(about).toHaveCount(0);
});
