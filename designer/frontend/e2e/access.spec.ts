import { expect, test } from "@playwright/test";
import { openMuster } from "./helpers";

// Barrierefreiheit: Analyse ohne veraPDF, Baum in Lesereihenfolge, Klick
// führt zum Element, ein fehlender Alternativtext erscheint als Befund.
test("Barrierefreiheit: Strukturbaum und Vorprüfung", async ({ page }) => {
  await openMuster(page, true);
  await page.getByRole("tab", { name: "Barrierefreiheit" }).click();
  const panel = page.getByTestId("access");
  await expect(panel.getByTestId("access-verdict")).toContainText("Keine Befunde der Vorprüfung");
  await expect(panel).toContainText("Geprüft wird zur Vorschau nach PDF/A-3a + PDF/UA-1");

  const tree = panel.getByTestId("access-tree");
  const h1 = tree.locator('[data-struct="H1"]');
  await expect(h1).toContainText("Überschrift 1");
  await expect(h1).toContainText("Rechnung");
  await expect(tree.locator('[data-struct="Figure"]')).toContainText("GiroCode zur Überweisung");
  await h1.click();
  await expect(page.getByTestId("inspector").locator("header code")).toHaveText("titel");

  // Alternativtext des GiroCodes entfernen: Befund mit Element.
  await tree.locator('[data-struct="Figure"]').click();
  const inspector = page.getByTestId("inspector");
  await expect(inspector.locator("header code")).toHaveText("girocode");
  const alt = inspector.locator('[data-prop="altBind"] [contenteditable], [data-prop="altBind"] input, [data-prop="altBind"] textarea').first();
  await alt.click();
  await page.keyboard.press(process.platform === "darwin" ? "Meta+A" : "Control+A");
  await page.keyboard.press("Backspace");
  await page.keyboard.press("Enter");
  await expect(panel.getByTestId("access-verdict")).toContainText("1 Befunde der Vorprüfung", { timeout: 10_000 });
  await expect(panel.locator('button.item[data-ref="girocode"]')).toContainText("Abbildung ohne Alternativtext");
  await page.getByRole("button", { name: "Rückgängig", exact: true }).click();
  await expect(panel.getByTestId("access-verdict")).toContainText("Keine Befunde der Vorprüfung", { timeout: 10_000 });

  // Lesereihenfolge auf der Seite: Briefkopf zuerst, Überschrift danach.
  await panel.getByTestId("access-order").check();
  const first = page.locator('.order[data-order="1"]');
  await expect(first).toHaveAttribute("data-ref", "briefkopf-firma");
  const titel = page.locator('.order[data-ref="titel"]');
  await expect(titel).toBeVisible();
  expect(Number(await titel.getAttribute("data-order"))).toBeGreaterThan(3);
  await page.screenshot({ path: "test-results/lesereihenfolge.png" });
});
