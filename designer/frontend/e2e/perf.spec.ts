import { expect, test } from "@playwright/test";
import { muster, openPath, selectInTree } from "./helpers";

// Leistungsmessung gegen die Ziele aus dem Plan. Die Werte stehen im
// Testbericht (Anhang perf) und im README. Ohne PDX_PERF=1 prüft der Test
// nur großzügige Grenzen, damit langsame CI-Rechner nicht scheitern.
const strict = !!process.env.PDX_PERF;

const p95 = (xs: number[]) => [...xs].sort((a, b) => a - b)[Math.floor(xs.length * 0.95)];

test("Leistung: Öffnen, Ziehen, Bearbeiten bis zur neuen Seite", async ({ page, browserName }, info) => {
  test.skip(browserName !== "chromium", "Messung nur in Chromium");
  test.setTimeout(120_000);
  const results: Record<string, number> = {};

  // Öffnen bis zur ersten gezeichneten Seite.
  await page.goto("/");
  const t0 = Date.now();
  await openPath(page, muster);
  await expect(page.locator('[data-page="1"] canvas')).toBeVisible();
  results.openToFirstPageMs = Date.now() - t0;

  // Ziehen: Frame-Abstände während 120 Mausbewegungen.
  await selectInTree(page, ["seiten", "erste", "erste-kopf", "adressfenster"]);
  const sel = page.locator('[data-page="1"] [data-sel="adressfenster"]');
  await expect(sel).toBeVisible();
  const box = (await sel.boundingBox())!;
  await page.evaluate(() => {
    const w = window as unknown as { __frames: number[]; __run: boolean };
    w.__frames = [];
    w.__run = true;
    let last = performance.now();
    const tick = (now: number) => {
      w.__frames.push(now - last);
      last = now;
      if (w.__run) requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
  });
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  for (let i = 0; i < 120; i++) {
    await page.mouse.move(box.x + box.width / 2 + Math.sin(i / 10) * 60, box.y + box.height / 2 + (i % 40), { steps: 1 });
  }
  const frames = await page.evaluate(() => {
    const w = window as unknown as { __frames: number[]; __run: boolean };
    w.__run = false;
    return w.__frames.slice(2);
  });
  await page.keyboard.press("Escape");
  await page.mouse.up();
  results.dragFrameP95Ms = Math.round(p95(frames) * 10) / 10;
  results.dragFrameMaxMs = Math.round(Math.max(...frames) * 10) / 10;
  // Zurück, falls das Ziehen eine Änderung übernommen hat.
  if (await page.getByRole("button", { name: "Rückgängig", exact: true }).isEnabled()) {
    await page.getByRole("button", { name: "Rückgängig", exact: true }).click();
  }

  // Bearbeiten bis zur neuen Seite: Lage ändern, bis die neue Seite geladen ist.
  await selectInTree(page, ["seiten", "erste", "erste-kopf", "adressfenster"]);
  const x = page.getByTestId("inspector").locator('[data-prop="x"] input');
  const edits: number[] = [];
  // Gemessen bis die Auswahl an der neuen Stelle steht, also bis neu gesetzt,
  // übertragen und gezeichnet ist. Jeder Schritt hat einen neuen Wert.
  for (let i = 0; i < 12; i++) {
    const before = await sel.getAttribute("style");
    const start = Date.now();
    await x.fill(`${21 + i}mm`);
    await x.press("Enter");
    await expect(sel).not.toHaveAttribute("style", before ?? "", { timeout: 5000 });
    edits.push(Date.now() - start);
  }
  results.editToPageP95Ms = p95(edits);

  console.log("perf", JSON.stringify(results));
  await info.attach("perf", { body: JSON.stringify(results, null, 2), contentType: "application/json" });

  expect(results.openToFirstPageMs).toBeLessThan(strict ? 1500 : 5000);
  expect(results.dragFrameP95Ms).toBeLessThan(strict ? 33.4 : 100);
  expect(results.editToPageP95Ms).toBeLessThan(strict ? 300 : 1500);
});
