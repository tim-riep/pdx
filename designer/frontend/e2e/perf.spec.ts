import { expect, test } from "@playwright/test";
import { muster, openPath, selectInTree } from "./helpers";

// Performance measurement against the goals from the plan. The values are in the
// test report (attachment perf) and in the README. Without PDX_PERF=1 the test
// checks only generous limits, so slow CI machines do not fail.
const strict = !!process.env.PDX_PERF;

const p95 = (xs: number[]) => [...xs].sort((a, b) => a - b)[Math.floor(xs.length * 0.95)];

test("Leistung: Öffnen, Ziehen, Bearbeiten bis zur neuen Seite", async ({ page, browserName }, info) => {
  test.skip(browserName !== "chromium", "Messung nur in Chromium");
  test.setTimeout(120_000);
  const results: Record<string, number> = {};

  // Opening until the first drawn page.
  await page.goto("/");
  const t0 = Date.now();
  await openPath(page, muster);
  await expect(page.locator('[data-page="1"] canvas')).toBeVisible();
  results.openToFirstPageMs = Date.now() - t0;

  // Dragging: frame intervals during 120 mouse moves.
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
  // Go back in case the dragging applied a change.
  if (await page.getByRole("button", { name: "Rückgängig", exact: true }).isEnabled()) {
    await page.getByRole("button", { name: "Rückgängig", exact: true }).click();
  }

  // Editing until the new page: change the position until the new page is loaded.
  await selectInTree(page, ["seiten", "erste", "erste-kopf", "adressfenster"]);
  const x = page.getByTestId("inspector").locator('[data-prop="x"] input');
  const edits: number[] = [];
  // Measured until the selection is at the new position, i.e. until laid out again,
  // transferred and drawn. Each step has a new value.
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
  // Headless browsers on CI machines often throttle requestAnimationFrame to
  // 100 ms, so the frame intervals there measure the clock of the browser. They are
  // checked only when measuring with PDX_PERF=1, otherwise only recorded.
  if (strict) expect(results.dragFrameP95Ms).toBeLessThan(33.4);
  expect(results.editToPageP95Ms).toBeLessThan(strict ? 300 : 1500);
});
