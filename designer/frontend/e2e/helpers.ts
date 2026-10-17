import { expect, type Page } from "@playwright/test";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
export const muster = resolve(here, "../../../testdata/templates/musterrechnung");

// openPath öffnet ein Paket über die Pfadabfrage des Browsers.
export async function openPath(page: Page, path: string) {
  await page.getByRole("button", { name: "Öffnen", exact: true }).first().click();
  const box = page.getByRole("dialog").getByRole("textbox");
  await box.fill(path);
  await box.press("Enter");
}

export async function openMuster(page: Page) {
  await page.goto("/");
  await openPath(page, muster);
  await expect(page.getByTestId("doc-title")).toHaveText("musterrechnung");
}
