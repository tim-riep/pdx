import { expect, test } from "@playwright/test";

test("Über zeigt die Version und führt zu den Lizenzen", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("button", { name: /Designer .*Engine/ }).click();
  const about = page.getByTestId("about");
  await expect(about).toBeVisible();
  await page.getByTestId("about-licenses").click();
  await expect(about).toHaveCount(0);
  await expect(page.getByTestId("licenses")).toBeVisible();
  await page.getByRole("button", { name: "Schließen" }).click();
  await expect(page.getByTestId("licenses")).toHaveCount(0);
});

// Der Knopf Lizenzen zeigt die eigene Lizenz und jeden Fremdbestandteil mit
// Lizenz und vollem Text, die Suche filtert die Liste.
test("Lizenzen: eigene Lizenz und alle Fremdbestandteile", async ({ page }) => {
  await page.goto("/");
  await page.getByTestId("licenses-open").click();
  const dlg = page.getByTestId("licenses");
  const text = dlg.getByTestId("licenses-text");
  await expect(text).toContainText("Apache License");
  await expect(text).toContainText("Copyright 2026 Tim Riep");
  const list = dlg.getByTestId("licenses-list");
  for (const [name, lic] of [
    ["github.com/wailsapp/wails/v3", "MIT"],
    ["github.com/tim-riep/go-xslt", "Apache-2.0"],
    ["pdfjs-dist", "Apache-2.0"],
    ["svelte", "MIT"],
    ["Go (Standardbibliothek und Laufzeit)", "BSD-3-Clause"],
    ["Liberation", "OFL-1.1"],
  ]) {
    const item = list.locator(`[data-component^="${name}"]`);
    await expect(item).toContainText(lic);
  }
  await list.locator('[data-component^="Liberation"]').click();
  await expect(text).toContainText("SIL OPEN FONT LICENSE");
  await dlg.getByTestId("licenses-filter").fill("BSD");
  await expect(list.locator('[data-component^="svelte"]')).toHaveCount(0);
  await expect(list.locator('[data-component^="golang.org/x/text"]')).toBeVisible();
  await page.getByRole("button", { name: "Schließen" }).click();
});
