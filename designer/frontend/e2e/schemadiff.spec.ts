import { expect, test } from "@playwright/test";
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { muster, openMuster } from "./helpers";

// Neues XSD: seller heißt vendor, tax entfällt, orderRef ist neu und Pflicht.
function changedXSD(): string {
  let s = readFileSync(join(muster, "schema/invoice.xsd"), "utf8");
  s = s.replace(`<xs:element name="seller" type="Party"/>`, `<xs:element name="vendor" type="Party"/>`);
  s = s.replace(`<xs:element name="tax" type="Amount"/>`, ``);
  s = s.replace(`<xs:element name="number" type="xs:string"/>`, `<xs:element name="number" type="xs:string"/><xs:element name="orderRef" type="xs:string"/>`);
  const dir = mkdtempSync(join(tmpdir(), "pdx-xsd-"));
  const p = join(dir, "rechnung-v2.xsd");
  writeFileSync(p, s);
  return p;
}

test("Schemaänderung: Unterschiede, Umstellung und Aufgabenliste", async ({ page }) => {
  await openMuster(page, true);
  await page.getByRole("tab", { name: "Daten" }).click();
  await page.getByRole("button", { name: "XSD laden" }).click();
  const box = page.getByRole("dialog").getByRole("textbox");
  await box.fill(changedXSD());
  await box.press("Enter");

  const diff = page.getByTestId("schema-diff");
  await expect(diff).toBeVisible();
  const ren = diff.locator('[data-change="inv:invoice/inv:seller"]');
  await expect(ren).toContainText("inv:invoice/inv:vendor");
  await expect(ren.getByRole("checkbox")).toBeChecked();
  await expect(diff.locator('[data-change="inv:invoice/inv:totals/inv:tax"]')).toContainText("1 Element betroffen");
  await expect(diff.locator('[data-change="inv:invoice/inv:orderRef"]')).toContainText("Pflicht");
  await page.getByRole("button", { name: "Übernehmen" }).click();
  await expect(diff).toHaveCount(0);

  // Datenbaum mit vendor, Aufgaben für tax und die Beispieldaten.
  await expect(page.getByTestId("data").locator('[data-path="inv:invoice/inv:vendor"]')).toBeVisible();
  const tasks = page.getByTestId("schema-tasks");
  const tax = tasks.locator('[data-task="removed:inv:invoice/inv:totals/inv:tax"]');
  await expect(tax).toBeVisible();
  await expect(tax.getByRole("checkbox")).not.toBeChecked();
  await expect(tasks.locator('[data-task="samples"] input')).not.toBeChecked();
  await expect(tasks).not.toContainText("seller");

  // Zum Element springen und es entfernen: die Aufgabe ist erledigt.
  await tax.getByRole("button", { name: "steuer-wert" }).click();
  await expect(page.getByTestId("inspector").locator("header code")).toHaveText("steuer-wert");
  await page.locator("body").press("Delete");
  await expect(tax.getByRole("checkbox")).toBeChecked();

  // Zwei Schritte zurück: das alte Schema ist wieder da.
  await page.getByRole("button", { name: "Rückgängig", exact: true }).click();
  await page.getByRole("button", { name: "Rückgängig", exact: true }).click();
  await expect(page.getByTestId("data").locator('[data-path="inv:invoice/inv:seller"]')).toBeVisible();
});
