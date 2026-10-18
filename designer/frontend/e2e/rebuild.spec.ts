import { expect, test, type Page } from "@playwright/test";
import { existsSync, mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { dragData, muster, selectInTree } from "./helpers";

// Abnahme: die Musterrechnung im Designer von Grund auf nachbauen, ohne XML
// zu schreiben. Mit PDX_E2E_REAL_VERAPDF=1 prüft das echte veraPDF (lokal
// oder Docker) das Ergebnis gegen PDF/A-3b, sonst die Attrappe.
const realVeraPDF = !!process.env.PDX_E2E_REAL_VERAPDF;

class Designer {
  constructor(readonly page: Page) {}
  get inspector() {
    return this.page.getByTestId("inspector");
  }
  get tree() {
    return this.page.getByTestId("hierarchy");
  }

  async answer(path: string) {
    const box = this.page.getByRole("dialog").getByRole("textbox");
    await box.fill(path);
    await box.press("Enter");
  }

  // selected wartet, bis das Element mit der ID im Inspektor steht.
  async selected(id: string) {
    await expect(this.inspector.locator("header code")).toHaveText(id);
  }

  // prop setzt eine Eigenschaft im Inspektor: Auswahl, Feld oder XPath.
  async prop(name: string, value: string) {
    const box = this.inspector.locator(`[data-prop="${name}"]`);
    const select = box.locator("select");
    if (await select.count()) {
      await select.first().selectOption(value);
    } else {
      const input = box.locator("input").first();
      await input.fill(value);
      if ((await input.getAttribute("role")) === "combobox") await input.evaluate((e) => (e as HTMLInputElement).blur());
      else await input.press("Enter");
    }
    const field = box.locator("input, select").first();
    await expect(field).toHaveValue(value);
  }

  // rename gibt dem gewählten Element eine sprechende ID.
  async rename(id: string) {
    await this.prop("id", id);
    await this.selected(id);
  }

  async props(values: Record<string, string>) {
    for (const [k, v] of Object.entries(values)) await this.prop(k, v);
  }

  // insert fügt einen Baustein in das Element ein, das gerade gewählt ist.
  async insert(kind: string, id: string, values: Record<string, string> = {}) {
    const before = await this.inspector.locator("header code").textContent();
    await this.page.getByTestId("palette").locator(`[data-kind="${kind}"]`).click();
    await expect(this.inspector.locator("header code")).not.toHaveText(before ?? "");
    await this.rename(id);
    await this.props(values);
  }

  // child fügt über das Kontextmenü der Hierarchie ein Kind ein.
  async child(key: string, id?: string, values: Record<string, string> = {}) {
    const header = this.inspector.locator("header");
    const before = (await header.textContent()) ?? "";
    await this.tree.getByRole("button", { name: "Aktionen" }).click();
    await this.page.getByTestId("context-menu").getByTestId(`insert-${key}`).click();
    await expect(this.page.getByTestId("context-menu")).toHaveCount(0);
    await expect(header).not.toHaveText(before); // das neue Element ist gewählt
    if (id) await this.rename(id);
    await this.props(values);
  }

  async go(path: string[]) {
    await selectInTree(this.page, path);
    await this.selected(path[path.length - 1]);
  }

  // content schreibt den Text im Inspektor. @pfad fügt ein Feld ein.
  async content(parts: (string | { field: string })[]) {
    const editor = this.inspector.getByTestId("inspector-content").getByTestId("content-editor");
    await editor.click({ position: { x: 4, y: 4 } });
    await this.page.keyboard.press("ControlOrMeta+a");
    await this.page.keyboard.press("Delete");
    for (const p of parts) {
      if (typeof p === "string") {
        await this.page.keyboard.type(p);
      } else {
        await this.page.keyboard.type("@" + p.field);
        await expect(this.page.getByTestId("content-menu").getByRole("option").first()).toHaveText(p.field);
        await this.page.keyboard.press("Enter");
      }
    }
    await this.page.keyboard.press("Enter");
    await expect(this.page.getByRole("alert")).toHaveCount(0);
  }

  // keys bedient die Hierarchie mit der Tastatur ab der Auswahl.
  async keys(...keys: string[]) {
    await this.tree.locator('[aria-selected="true"] button.name').focus();
    for (const k of keys) await this.page.keyboard.press(k);
  }
}

test("Musterrechnung von Grund auf nachbauen", async ({ page }) => {
  test.setTimeout(realVeraPDF ? 300_000 : 180_000);
  const d = new Designer(page);
  await page.goto("/");
  await page.getByRole("button", { name: "Neu", exact: true }).first().click();
  await expect(page.getByTestId("doc-title")).toHaveText("Neue Vorlage");

  // Daten: XSD und Beispieldaten.
  await page.getByRole("tab", { name: "Daten" }).click();
  await page.getByRole("button", { name: "XSD laden" }).click();
  await d.answer(`${muster}/schema/invoice.xsd`);
  await expect(page.getByTestId("data").locator('[data-path$=":invoice"]')).toBeVisible();
  // Präfix des Daten-Namespace, den der Designer vergeben hat.
  const p = (await page.getByTestId("data").locator("[data-path]").first().getAttribute("data-path"))!.split(":")[0];
  await page.getByRole("tab", { name: "Beispiele" }).click();
  await page.getByRole("button", { name: "Importieren" }).click();
  await d.answer(`${muster}/samples/rechnung.xml`);
  await expect(page.getByTestId("samples").locator('[data-sample="samples/rechnung.xml"]')).toBeVisible();
  const f = (path: string) => ({ field: path.replace(/(^|\/)/g, `$1${p}:`) });

  // Formate und Textressourcen über das Kontextmenü der Wurzel.
  await d.go(["vorlage"]);
  await d.child("formats");
  await d.keys("ArrowRight"); // Elemente ohne ID per Tastatur aufklappen
  await d.go(["vorlage", "locale"]);
  await d.props({ decimalSeparator: ",", groupingSeparator: "." });
  await d.go(["vorlage", "locale", "locale-pattern"]);
  await d.props({ name: "betrag", picture: "#.##0,00" });
  await d.go(["vorlage", "locale"]);
  await d.child("locale/pattern", "fmt-datum", { name: "datum", kind: "date", picture: "[D01].[M01].[Y0001]" });
  await d.go(["vorlage"]);
  await d.child("texts");
  await d.keys("ArrowRight");
  await d.go(["vorlage", "lang", "lang-string"]);
  await d.prop("key", "rechnung");
  await d.content(["Rechnung"]);

  // Stil und Farb-Token im Panel Stile.
  await page.getByRole("tab", { name: "Stile" }).click();
  await page.getByTestId("styles").getByRole("button", { name: "+ Farbe" }).click();
  const token = page.getByTestId("styles").locator('[data-token="farbe"]');
  await token.getByRole("textbox", { name: "Name" }).fill("marke");
  await token.getByRole("textbox", { name: "Name" }).press("Enter");
  const marke = page.getByTestId("styles").locator('[data-token="marke"]');
  await marke.getByRole("textbox", { name: "Wert" }).fill("#1f4e79");
  await marke.getByRole("textbox", { name: "Wert" }).press("Enter");
  await page.getByTestId("styles").getByRole("button", { name: "+ Stil" }).click();
  await d.rename("st-klein");
  await d.props({ fontSize: "7.5pt", color: "#555555" });

  // Erste Seite: Kopf mit Briefkopf, Adressfenster und Rechnungsdaten, Fuß
  // mit Seitenzahl.
  await d.go(["seiten", "seite"]);
  await d.rename("erste");
  await d.props({ use: "first", margin: "15mm 20mm 15mm 25mm" });
  await d.child("pageMaster/header", "erste-kopf", { height: "85mm" });
  await d.go(["seiten", "erste"]);
  await d.child("pageMaster/footer", "erste-fuss", { height: "12mm" });

  await d.go(["seiten", "erste", "erste-kopf"]);
  await d.insert("text", "briefkopf", { x: "0mm", y: "0mm", w: "165mm", textAlign: "end", color: "$marke", fontSize: "16pt", fontWeight: "bold" });
  await d.content([f("seller/name")]);
  await d.go(["seiten", "erste", "erste-kopf"]);
  await d.insert("positioned", "adressfenster", { x: "0mm", y: "40mm", w: "85mm", h: "45mm" });
  await d.insert("text", "absender", { x: "0mm", y: "5mm", w: "85mm", style: "st-klein" });
  await d.content([f("seller/name"), " · ", f("seller/street")]);
  await d.go(["seiten", "erste", "erste-kopf", "adressfenster"]);
  await d.insert("flowed", "empfaenger", { x: "0mm", y: "12mm", w: "85mm", bind: `${p}:buyer` });
  for (const [id, parts] of [
    ["empf-name", [f("name")]],
    ["empf-strasse", [f("street")]],
    ["empf-ort", [f("postcode"), " ", f("city")]],
  ] as const) {
    await d.go(["seiten", "erste", "erste-kopf", "adressfenster", "empfaenger"]);
    await d.insert("text", id);
    await d.content([...parts]);
  }
  await d.go(["seiten", "erste", "erste-kopf"]);
  await d.insert("flowed", "rechnungsdaten", { x: "100mm", y: "45mm", w: "65mm" });
  await d.insert("text", "rechnungsnummer");
  await d.content(["Rechnungsnummer: ", f("number")]);
  await d.go(["seiten", "erste", "erste-kopf", "rechnungsdaten"]);
  await d.insert("text", "rechnungsdatum");
  await d.content(["Rechnungsdatum: ", f("issueDate")]);
  await d.go(["seiten", "erste", "erste-kopf", "rechnungsdaten", "rechnungsdatum", "rechnungsdatum-issueDate"]);
  await d.prop("format", "datum");
  await d.go(["seiten", "erste", "erste-fuss"]);
  await d.insert("pageNumber", "erste-seitenzahl", { x: "0mm", y: "4mm", w: "165mm", textAlign: "center", style: "st-klein" });

  // Folgeseiten: schmaler Kopf und Seitenzahl.
  await d.go(["seiten"]);
  await d.child("pageMaster", "folge", { use: "rest", margin: "15mm 20mm 15mm 25mm" });
  await d.child("pageMaster/header", "folge-kopf", { height: "10mm" });
  await d.insert("text", "folge-titel", { x: "0mm", y: "0mm", w: "165mm", style: "st-klein" });
  await d.content([f("seller/name"), " · Rechnung ", f("number")]);
  await d.go(["seiten", "folge"]);
  await d.child("pageMaster/footer", "folge-fuss", { height: "12mm" });
  await d.insert("pageNumber", "folge-seitenzahl", { x: "0mm", y: "4mm", w: "165mm", textAlign: "center", style: "st-klein" });

  // Inhalt: Titel aus der Textressource, Tabelle per Drag-and-drop.
  await d.go(["inhalt"]);
  await d.insert("text", "titel");
  await d.content([]); // ein Text mit Textressource hat keinen eigenen Inhalt
  await d.props({ ref: "rechnung", role: "H1", fontSize: "16pt", fontWeight: "bold", spaceAfter: "4mm" });
  await page.getByRole("tab", { name: "Daten" }).click();
  const data = page.getByTestId("data");
  await data.locator(`[data-path="${p}:invoice/${p}:items"] .twisty`).click();
  const zoom = parseFloat((await page.getByTestId("zoom").textContent())!) / 100;
  const mm = (72 / 25.4) * zoom;
  // Unter den gesetzten Titel ablegen: erst warten, bis der Canvas ihn zeigt.
  const title = page.locator('[data-page="1"] [data-sel="titel"]');
  await expect(title).toBeVisible();
  const pageBox = (await page.locator('[data-page="1"]').boundingBox())!;
  const titleBox = (await title.boundingBox())!;
  await dragData(page, data.locator(`[data-path="${p}:invoice/${p}:items/${p}:item"]`), page.locator('[data-page="1"]'),
    { x: titleBox.x - pageBox.x + 20 * mm, y: titleBox.y - pageBox.y + titleBox.height + 25 * mm });
  await expect(d.inspector.locator("header strong")).toHaveText("Tabelle");
  await d.rename("positionen");

  // Kopf: zweite Zeile mit Übertrag, Fuß mit Zwischensumme.
  await page.getByRole("tab", { name: "Hierarchie" }).click();
  const tools = page.getByTestId("table-tools");
  await d.keys("ArrowRight"); // Tabelle aufklappen
  await expect(d.tree.locator('[data-row="positionen"]')).toHaveAttribute("aria-expanded", "true");
  const headerRow = d.tree.locator("[data-row]", { hasText: /^.*Tabellenkopf/ }).first();
  await headerRow.locator("button.name").click();
  await d.keys("ArrowRight", "ArrowRight", "ArrowRight", "ArrowRight"); // Kopf, Zeile, erste Zelle
  await expect(d.inspector.locator("header strong")).toHaveText("Zelle");
  await tools.getByRole("button", { name: "Zeile einfügen" }).click();
  await expect(d.inspector.locator("header strong")).toHaveText("Zeile");
  await d.keys("ArrowRight", "ArrowRight");
  await expect(d.inspector.locator("header strong")).toHaveText("Zelle");
  await tools.getByRole("spinbutton").fill("4");
  await tools.getByRole("spinbutton").press("Tab");
  await d.keys("ArrowDown");
  await tools.getByRole("button", { name: "Übertrag" }).click();
  await expect(d.inspector.locator("header strong")).toHaveText("Übertrag");
  await d.go(["inhalt", "positionen"]);
  await tools.getByRole("button", { name: "Fuß hinzufügen" }).click();
  await expect(d.inspector.locator("header strong")).toHaveText("Tabellenfuß");
  await d.keys("ArrowRight", "ArrowRight", "ArrowRight", "ArrowRight"); // Fuß, Zeile, erste Zelle
  await expect(d.inspector.locator("header strong")).toHaveText("Zelle");
  await tools.getByRole("spinbutton").fill("4");
  await tools.getByRole("spinbutton").press("Tab");
  await d.keys("ArrowDown");
  await tools.getByRole("button", { name: "Zwischensumme" }).click();
  await expect(d.inspector.locator("header strong")).toHaveText("Zwischensumme");

  // Summen mit GiroCode.
  await d.go(["inhalt", "positionen"]);
  await d.insert("flowed", "summen", { spaceBefore: "4mm" });
  for (const [id, label, path] of [
    ["netto", "Nettobetrag: ", "totals/net"],
    ["steuer", "Umsatzsteuer: ", "totals/tax"],
    ["brutto", "Gesamtbetrag: ", "totals/gross"],
  ] as const) {
    await d.go(["inhalt", "summen"]);
    await d.insert("text", id);
    await d.content([label, f(path), " €"]);
  }
  await d.go(["inhalt", "summen"]);
  await d.insert("barcode", "girocode", { type: "girocode", value: "", w: "30mm" });
  for (const [i, name, path] of [
    [1, "name", "seller/name"],
    [2, "iban", "seller/iban"],
    [3, "amount", "totals/gross"],
    [4, "text", "number"],
  ] as const) {
    await d.go(["inhalt", "summen", "girocode"]);
    await d.child("barcode/data", `gc-${i}`, { name, bind: f(path).field });
  }

  // Anhang mit den Rechnungsdaten.
  await d.go(["vorlage"]);
  await d.child("attachments");
  await d.keys("ArrowRight");
  await d.go(["vorlage", "attachment"]);
  await d.props({ name: "rechnung.xml", relationship: "Alternative", description: "Rechnungsdaten" });

  // Ergebnis: ohne Fehler, Vorschau über mehrere Seiten, PDF.
  const problems = page.getByTestId("problems");
  await expect(problems.locator("li.error")).toHaveCount(0);
  await page.getByRole("tab", { name: "Vorschau" }).click();
  await expect.poll(() => page.locator("[data-page]").count()).toBeGreaterThanOrEqual(2);
  await expect(problems.locator("li.error")).toHaveCount(0);
  await page.getByRole("tab", { name: "PDF" }).click();
  await expect(page.getByTestId("pdf-info")).toContainText("PDF/A-3b", { timeout: 20_000 });

  // Konformität.
  await page.getByRole("tab", { name: "Konformität" }).click();
  await page.getByTestId("conformance").getByRole("button", { name: "Konformität prüfen" }).click();
  const verdict = page.getByTestId("conformance-verdict");
  if (realVeraPDF) {
    await expect(verdict).toContainText("Konform mit PDF/A-3b", { timeout: 240_000 });
  } else {
    await expect(verdict).toBeVisible({ timeout: 30_000 });
  }

  // Speichern als Paket.
  const out = join(mkdtempSync(join(tmpdir(), "pdx-nachbau-")), "nachbau.zip");
  await page.getByRole("button", { name: "Speichern", exact: true }).click();
  await d.answer(out);
  await expect(page.getByTestId("doc-title")).toHaveText("nachbau");
  expect(existsSync(out)).toBe(true);
});
