import { expect, test } from "@playwright/test";
import { openMuster } from "./helpers";

test("Canvas zeigt die Seiten aus der Engine", async ({ page }) => {
  await openMuster(page, true);
  await expect(page.getByTestId("render-info")).toContainText("3 Seiten");
  // Die erste Seite ist gezeichnet: das Canvas hat Pixel ungleich Weiß.
  const inked = await page.locator('[data-page="1"] canvas').evaluate((c: HTMLCanvasElement) => {
    const d = c.getContext("2d")!.getImageData(0, 0, c.width, c.height).data;
    let n = 0;
    for (let i = 0; i < d.length; i += 4) if (d[i] < 128) n++;
    return n;
  });
  expect(inked).toBeGreaterThan(1000);

  // Der Entwurf zeigt Platzhalter mit drei Positionen, also eine Seite.
  await page.getByRole("tab", { name: "Entwurf" }).click();
  await expect(page.locator("[data-page]")).toHaveCount(1);
  const before = await page.getByTestId("zoom").textContent();
  await page.getByRole("button", { name: "Vergrößern" }).click();
  await expect(page.getByTestId("zoom")).not.toHaveText(before!);
});
