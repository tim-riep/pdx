import { expect, test, type Page } from "@playwright/test";

// The update check against the stand-in for GitHub (fixtures/release-server.mjs).
const stub = process.env.PDX_E2E_RELEASE!;

async function setRelease(page: Page, release: object | null) {
  const r = await page.request.post(`${stub}/latest`, { data: release ? JSON.stringify(release) : "" });
  expect(r.ok()).toBe(true);
}

const checks = async (page: Page) => Number(await (await page.request.get(`${stub}/count`)).text());

test.afterEach(async ({ page }) => {
  await setRelease(page, null);
});

test("Neue Version: Dialog mit bereinigten Hinweisen, Herunterladen, Überspringen, Ausschalten", async ({ page, context }) => {
  // A version of its own per run, the skipped version stays in the data directory.
  const version = `9.${Date.now() % 100000}.0`;
  const url = `https://github.com/tim-riep/pdx/releases/tag/v${version}`;
  await setRelease(page, {
    tag_name: `v${version}`,
    html_url: url,
    body_html: `<h2>Neu</h2><p>Signierte Programme<img src="https://tracker.example/p.gif"><script>window.boese = 1</script></p>`,
    published_at: "2026-10-24T10:30:00Z",
  });
  // The release page does not load from the internet in the test.
  await context.route("https://github.com/**", (r) => r.fulfill({ body: "Release" }));

  await page.goto("/");
  const dlg = page.getByTestId("update-dialog");
  await expect(dlg).toBeVisible();
  await expect(page.getByRole("dialog")).toHaveAccessibleName(`pdx Designer ${version} ist verfügbar`);
  await expect(dlg).toContainText("Signierte Programme");
  await expect(dlg).toContainText("24.10.2026");
  await expect(dlg.locator("img, script")).toHaveCount(0);
  expect(await page.evaluate(() => (window as { boese?: number }).boese)).toBeUndefined();

  // The server build has no browser of its own, the page opens in a new tab.
  const popup = page.waitForEvent("popup");
  await page.getByTestId("update-download").click();
  expect((await popup).url()).toBe(url);

  // Later closes, the button in the toolbar opens the dialog again.
  await page.getByTestId("update-later").click();
  await expect(dlg).toHaveCount(0);
  await page.getByTestId("update-open").click();
  await expect(dlg).toBeVisible();

  // Skipped, the dialog no longer opens by itself, the button stays.
  await page.getByTestId("update-skip").click();
  await expect(dlg).toHaveCount(0);
  await page.reload();
  await expect(page.getByTestId("update-open")).toHaveText(`Version ${version} verfügbar`);
  await expect(dlg).toHaveCount(0);

  // Switched off in About, the button disappears and no request goes out.
  await page.getByRole("button", { name: /Designer .*Engine/ }).click();
  const check = page.getByTestId("update-check");
  await expect(check).toBeChecked();
  await check.uncheck();
  await expect(page.getByTestId("update-open")).toHaveCount(0);
  await page.getByRole("button", { name: "Schließen" }).click();
  const before = await checks(page);
  await page.reload();
  await expect(page.getByTestId("status")).toBeVisible();
  await page.waitForTimeout(500);
  expect(await checks(page)).toBe(before);
  await expect(page.getByTestId("update-open")).toHaveCount(0);

  // Switched on again for the other tests.
  await page.getByRole("button", { name: /Designer .*Engine/ }).click();
  await page.getByTestId("update-check").check();
  await expect(page.getByTestId("update-open")).toBeVisible();
  await page.getByRole("button", { name: "Schließen" }).click();
});

test("Gleiche oder ältere Version: kein Dialog, kein Knopf", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("status")).toBeVisible();
  await expect.poll(() => checks(page)).toBeGreaterThan(0);
  await expect(page.getByTestId("update-open")).toHaveCount(0);
  await expect(page.getByTestId("update-dialog")).toHaveCount(0);
});
