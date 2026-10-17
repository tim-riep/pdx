import { expect, test } from "@playwright/test";
import { openMuster, selectInTree } from "./helpers";

test("Text mit Feld-Chips bearbeiten: Inspektor, @ und Doppelklick", async ({ page }) => {
  await openMuster(page);
  await selectInTree(page, ["seiten", "erste", "erste-kopf", "rechnungsdaten", "rechnungsnummer"]);
  const editor = page.getByTestId("inspector-content").getByTestId("content-editor");
  await expect(editor).toContainText("Rechnungsnummer:");
  await expect(editor.locator(".chip")).toHaveText("inv:number");

  // Text ergänzen und übernehmen.
  // Am Anfang klicken (ein Klick auf einen Chip wählt ihn ganz), dann ans Ende.
  await editor.click({ position: { x: 4, y: 4 } });
  await page.keyboard.press("End");
  await page.keyboard.type(" vom ");
  // Feld mit @ einfügen.
  await page.keyboard.type("@issue");
  const menu = page.getByTestId("content-menu");
  await expect(menu).toContainText("inv:issueDate");
  await page.keyboard.press("Enter");
  await expect(editor.locator(".chip")).toHaveCount(2);
  await page.keyboard.press("Enter"); // übernimmt
  await expect(page.getByRole("button", { name: "Rückgängig", exact: true })).toBeEnabled();
  // Nach dem Übernehmen baut der Editor aus dem Modell neu auf: das neue
  // Feld hat eine ID.
  await expect(editor.locator('.chip[data-ref="rechnungsnummer-issueDate"]')).toHaveText("inv:issueDate");
  await expect(editor).toContainText("vom");

  // Rückgängig nimmt beides zurück.
  await page.getByRole("button", { name: "Rückgängig", exact: true }).click();
  await expect(editor.locator(".chip")).toHaveCount(1);
  await expect(editor).not.toContainText("vom");

  // Doppelklick im Canvas öffnet den Editor am Text, Escape verwirft.
  const zoom = parseFloat((await page.getByTestId("zoom").textContent())!) / 100;
  const mm = 72 / 25.4;
  await page.locator('[data-page="1"]').dblclick({ position: { x: 130 * mm * zoom, y: 52 * mm * zoom } });
  const inline = page.getByTestId("inline-edit").getByTestId("content-editor");
  await expect(inline).toBeVisible();
  await expect(inline).toBeFocused();
  await expect(inline.locator(".chip")).toHaveText("inv:number");
  await page.keyboard.type("XYZ");
  await page.keyboard.press("Escape");
  await expect(page.getByTestId("inline-edit")).toHaveCount(0);
  await expect(editor).not.toContainText("XYZ");
});
