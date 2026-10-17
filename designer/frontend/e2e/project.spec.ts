import { expect, test } from "@playwright/test";
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { muster } from "./helpers";

test("Vorlage anlegen, speichern und wieder öffnen", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("button", { name: "Neu", exact: true }).first().click();
  await expect(page.getByTestId("doc-title")).toHaveText("Neue Vorlage");

  // Im Browser gibt es keine nativen Dialoge, der Designer fragt nach dem Pfad.
  const target = join(mkdtempSync(join(tmpdir(), "pdx-save-")), "neu.zip");
  await page.keyboard.press("ControlOrMeta+s");
  await page.getByRole("dialog").getByRole("textbox").fill(target);
  await page.getByRole("dialog").getByRole("button", { name: "OK" }).click();
  await expect(page.getByTestId("doc-title")).toHaveText("neu");

  await page.getByRole("button", { name: "Öffnen", exact: true }).first().click();
  await page.getByRole("dialog").getByRole("textbox").fill(muster);
  await page.getByRole("dialog").getByRole("textbox").press("Enter");
  await expect(page.getByTestId("doc-title")).toHaveText("musterrechnung");
  await expect(page.getByRole("alert")).toHaveCount(0);
});

test("Fehler beim Öffnen erscheint in der Statusleiste", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("button", { name: "Öffnen", exact: true }).first().click();
  await page.getByRole("dialog").getByRole("textbox").fill("/gibt/es/nicht.zip");
  await page.getByRole("dialog").getByRole("button", { name: "OK" }).click();
  await expect(page.getByRole("alert")).toBeVisible();
});
