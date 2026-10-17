import { expect, test, type Page } from "@playwright/test";
import { dragData, openMuster } from "./helpers";

// Punkt auf Seite 1 in Bildschirmkoordinaten (Angaben in Punkt).
async function at(page: Page, x: number, y: number): Promise<[number, number]> {
  const box = (await page.locator('[data-page="1"]').boundingBox())!;
  const zoom = parseFloat((await page.getByTestId("zoom").textContent())!) / 100;
  return [box.x + x * zoom, box.y + y * zoom];
}

const inspector = (page: Page, prop: string) => page.getByTestId("inspector").locator(`[data-prop="${prop}"] input`);

test("Positioniertes Element verschieben und Größe ändern", async ({ page }) => {
  await openMuster(page);
  const [x, y] = await at(page, 62, 248);
  await page.mouse.click(x, y);
  await expect(page.locator("[data-sel]")).toHaveAttribute("data-sel", "adressfenster");
  const zoom = parseFloat((await page.getByTestId("zoom").textContent())!) / 100;
  const mm = 72 / 25.4;

  // 12,3 mm nach rechts, 7,6 mm nach unten, mit Raster auf ganze mm.
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.mouse.move(x + 5, y + 5);
  await page.mouse.move(x + 12.3 * mm * zoom, y + 7.6 * mm * zoom);
  await expect(page.locator("[data-ghost]")).toBeVisible();
  await page.mouse.up();
  await expect(inspector(page, "x")).toHaveValue("32mm");
  await expect(inspector(page, "y")).toHaveValue("53mm");
  // Der Auswahlrahmen folgt dem neu gesetzten Element, 12 mm weiter rechts.
  await expect
    .poll(async () => (await page.locator(".box.sel").boundingBox())!.x - x)
    .toBeGreaterThan(12 * mm * zoom - 3 - (62 - 20 * mm) * zoom);

  // Größe über den rechten unteren Griff.
  const handle = page.locator('[data-handle="1,1"]');
  const hb = (await handle.boundingBox())!;
  await page.mouse.move(hb.x + 4, hb.y + 4);
  await page.mouse.down();
  await page.mouse.move(hb.x + 10, hb.y + 10);
  await page.mouse.move(hb.x + 4 + 10 * mm * zoom, hb.y + 4 + 5 * mm * zoom);
  await page.mouse.up();
  await expect(inspector(page, "w")).toHaveValue(/^9[45](\.[0-9])?mm$/);

  await page.keyboard.press("ControlOrMeta+z");
  await page.keyboard.press("ControlOrMeta+z");
  await expect(inspector(page, "x")).toHaveValue("20mm");
});

test("Bausteine einfügen und fließend umsortieren", async ({ page }) => {
  await openMuster(page);
  // Klick auf einen Baustein fügt hinter der Auswahl ein, hier ans Ende des Inhalts.
  await page.getByTestId("palette").locator('[data-kind="text"]').click();
  await expect(page.getByTestId("inspector").locator("header strong")).toHaveText("Text");
  const id = await page.getByTestId("inspector").locator("header code").textContent();

  // Ziehen auf die Seite fügt an der Stelle ein.
  await dragData(page, page.getByTestId("palette").locator('[data-kind="rect"]'), page.locator('[data-page="1"]'), { x: 260, y: 330 });
  await expect(page.getByTestId("inspector").locator("header strong")).toHaveText("Rechteck");

  // Der Text wird per Ziehen vor die Tabelle sortiert: Ziel ist der linke
  // Rand knapp über der Tabelle, dort liegt nur der Inhalt selbst.
  const tree = page.getByTestId("hierarchy");
  await tree.getByRole("button", { name: /Tabelle\s*positionen/ }).click();
  // Lage relativ zur Seite, weil die Auswahl den Canvas scrollt.
  const tableTop = (await page.locator(".box.sel").boundingBox())!.y - (await page.locator('[data-page="1"]').boundingBox())!.y;
  await tree.getByRole("button", { name: new RegExp(`Text\\s*${id}`) }).click();
  const sel = (await page.locator(".box.sel").boundingBox())!;
  await page.mouse.move(sel.x + 5, sel.y + sel.height / 2);
  await page.mouse.down();
  await page.mouse.move(sel.x + 20, sel.y + 20);
  const pageBox = (await page.locator('[data-page="1"]').boundingBox())!;
  await page.mouse.move(pageBox.x + 6, pageBox.y + tableTop + 2);
  await expect(page.locator("[data-reorder]")).toBeVisible();
  await page.mouse.up();
  await expect(page.getByTestId("doc-title")).toContainText("geändert");
  await expect(page.getByRole("alert")).toHaveCount(0);
  // Der Text steht jetzt vor der Tabelle.
  await expect
    .poll(async () => {
      const order = await page.getByTestId("hierarchy").locator(".id").allTextContents();
      return order.indexOf(id!) < order.indexOf("positionen");
    })
    .toBe(true);
});
