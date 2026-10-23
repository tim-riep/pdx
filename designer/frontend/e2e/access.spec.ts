import { expect, test } from "@playwright/test";
import { openMuster } from "./helpers";

// Accessibility: analysis without veraPDF, tree in reading order, a click
// leads to the element, a missing alternative text appears as a finding.
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

  // Remove the alternative text of the GiroCode: finding with element.
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

  // Reading order on the page: letterhead first, heading after it.
  await panel.getByTestId("access-order").check();
  const first = page.locator('.order[data-order="1"]');
  await expect(first).toHaveAttribute("data-ref", "briefkopf-firma");
  const titel = page.locator('.order[data-ref="titel"]');
  await expect(titel).toBeVisible();
  expect(Number(await titel.getAttribute("data-order"))).toBeGreaterThan(3);
  await page.screenshot({ path: "test-results/lesereihenfolge.png" });

  // Change the reading order: invoice data before the address window.
  const num = async (ref: string) => Number(await page.locator(`.order[data-ref="${ref}"]`).getAttribute("data-order"));
  expect(await num("absenderzeile")).toBeLessThan(await num("rechnungsnummer"));
  await tree.locator('[data-up="rechnungsdaten"]').click();
  await expect.poll(async () => (await num("rechnungsnummer")) < (await num("absenderzeile")), { timeout: 10_000 }).toBe(true);
  await page.getByRole("button", { name: "Rückgängig", exact: true }).click();
  await expect.poll(async () => (await num("absenderzeile")) < (await num("rechnungsnummer")), { timeout: 10_000 }).toBe(true);

  // Manual check: progress counts only applicable items.
  const mh = panel.getByTestId("matterhorn");
  await mh.getByRole("button", { name: /Manuelle Prüfung/ }).click();
  await expect(mh.getByTestId("matterhorn-progress")).toHaveText("0 von 34 geprüft");
  await expect(mh.locator('[data-check="03-003"]')).toBeDisabled();
  await mh.locator('[data-check="09-001"]').check();
  await expect(mh.getByTestId("matterhorn-progress")).toHaveText("1 von 34 geprüft");
  await mh.locator('[data-check="09-001"]').uncheck();
});
