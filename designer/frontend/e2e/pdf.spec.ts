import { expect, test } from "@playwright/test";
import { openMuster } from "./helpers";

test("Ansicht PDF zeigt das erzeugte PDF mit pdf.js", async ({ page }) => {
  const external: string[] = [];
  page.on("request", (r) => {
    const u = new URL(r.url());
    if (u.hostname !== "localhost" && u.protocol.startsWith("http")) external.push(r.url());
  });
  await openMuster(page);
  await page.getByRole("tab", { name: "PDF" }).click();
  const view = page.getByTestId("pdf-view");
  await expect(view.getByTestId("pdf-info")).toHaveText("3 Seiten, PDF/A-3b", { timeout: 15000 });
  await expect(view.locator("canvas[data-pdf-page]")).toHaveCount(3);
  // Die Seite ist gezeichnet: nicht nur weiß.
  const inked = await view.locator('canvas[data-pdf-page="1"]').evaluate((c) => {
    const ctx = (c as HTMLCanvasElement).getContext("2d")!;
    const d = ctx.getImageData(0, 0, (c as HTMLCanvasElement).width, (c as HTMLCanvasElement).height).data;
    let dark = 0;
    for (let i = 0; i < d.length; i += 16) if (d[i] < 128) dark++;
    return dark;
  });
  expect(inked).toBeGreaterThan(500);
  expect(external).toEqual([]);
  await page.getByRole("tab", { name: "Entwurf" }).click();
  await expect(page.locator('[data-page="1"] canvas')).toBeVisible();
});
