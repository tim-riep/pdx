import { expect, test } from "@playwright/test";
import { openMuster, selectInTree } from "./helpers";

test("XPath im Inspektor: Vorschläge, Prüfung und Wert", async ({ page }) => {
  await openMuster(page, true);
  await selectInTree(page, ["inhalt", "positionen", "positionen-zeilen", "position", "z-betrag", "z-betrag-f"]);
  const inspector = page.getByTestId("inspector");
  const bind = inspector.locator('[data-prop="bind"] input');
  const status = inspector.getByTestId("xpath-status");
  await expect(bind).toHaveValue("inv:amount");

  // Vorschläge aus dem Kontext der Position.
  await bind.fill("inv:am");
  await expect(page.getByTestId("xpath-menu")).toContainText("inv:amount");
  await bind.press("Enter");
  await expect(bind).toHaveValue("inv:amount");
  await expect(status).toContainText("1 Treffer");

  // Unbekannter Knoten und Syntaxfehler, Escape verwirft.
  await bind.fill("inv:betrag ");
  await expect(status).toContainText("inv:betrag gibt es unter inv:invoice/inv:items/inv:item nicht");
  await bind.fill("sum( ");
  await expect(status).toContainText("unvollständig");
  await bind.press("Escape");
  await expect(bind).toHaveValue("inv:amount");

  // Übernehmen beim Verlassen, Rückgängig stellt den alten Wert her.
  await bind.fill("inv:unitPrice ");
  await bind.evaluate((e) => (e as HTMLInputElement).blur());
  await expect(page.getByRole("button", { name: "Rückgängig", exact: true })).toBeEnabled();
  await expect(bind).toHaveValue("inv:unitPrice");
  await page.getByRole("button", { name: "Rückgängig", exact: true }).click();
  await expect(bind).toHaveValue("inv:amount");

  // Der Tab XPath zeigt denselben Ausdruck mit Kontext, Daten und Funktionen.
  await page.getByRole("tab", { name: "XPath" }).click();
  const panel = page.getByTestId("xpath-panel");
  const area = panel.locator("textarea");
  await expect(area).toHaveValue("inv:amount");
  await expect(panel.getByTestId("xpath-context")).toHaveText("inv:invoice/inv:items/inv:item");
  // Cursor an den Anfang, die Funktion kommt davor.
  await area.focus();
  await area.evaluate((e) => (e as HTMLTextAreaElement).setSelectionRange(0, 0));
  await area.press("ArrowLeft");
  await panel.getByRole("button", { name: "sum(folge)" }).dispatchEvent("mousedown");
  await expect(area).toHaveValue("sum(inv:amount");
  await area.press("End");
  await area.pressSequentially(")");
  await expect(panel.getByTestId("xpath-status")).toBeVisible();
  await expect(panel.getByTestId("xpath-status")).not.toHaveClass(/bad/);
  await panel.getByRole("button", { name: "Verwerfen" }).click();
  await expect(area).toHaveValue("inv:amount");
});

test("Präsenz über die Auswahl", async ({ page }) => {
  await openMuster(page, true);
  await selectInTree(page, ["inhalt", "titel"]);
  const presence = page.getByTestId("inspector").getByTestId("presence");
  await expect(presence.getByRole("radio", { name: "Immer" })).toBeChecked();
  await expect(presence.getByRole("combobox")).toHaveCount(0);

  await presence.getByRole("radio", { name: "Wenn vorhanden" }).check();
  const expr = presence.locator("input[role=combobox]");
  await expect(expr).toHaveValue(/^exists\(inv:.+\)$/);
  const field = (await expr.inputValue()).slice("exists(".length, -1);
  await expect(presence.getByRole("combobox", { name: "Datenknoten" })).toHaveValue(field);

  await presence.getByRole("radio", { name: "Wenn nicht leer" }).check();
  await expect(expr).toHaveValue(`normalize-space(${field})`);

  await presence.getByRole("radio", { name: "Immer" }).check();
  await expect(expr).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Rückgängig", exact: true })).toBeEnabled();
});
