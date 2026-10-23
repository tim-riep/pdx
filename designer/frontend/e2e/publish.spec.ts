import { expect, test } from "@playwright/test";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { openPath, selectInTree } from "./helpers";
import { e2eServerKey } from "../playwright.config";

const muster = resolve(dirname(fileURLToPath(import.meta.url)), "../../../testdata/templates/musterrechnung");

// Publishing: connect, the comparison shows a new template, upload
// and activate, then no changes. A change in the designer
// appears as a line diff, becomes version 2 and can be activated.
test("Veröffentlichen mit Versionsvergleich", async ({ page }, info) => {
  const id = `e2e-${info.project.name}-${info.repeatEachIndex}-${info.retry}`;
  await page.goto("/");
  await openPath(page, muster);
  await expect(page.getByTestId("doc-title")).toHaveText("musterrechnung");
  await page.getByTestId("publish-open").click();
  const dlg = page.getByTestId("publish");
  await expect(dlg).toHaveAttribute("data-ready", "true");
  // One backend for all tests: disconnect a connection from an earlier
  // run.
  const disconnect = page.getByRole("button", { name: "Trennen" });
  await expect(dlg.getByTestId("publish-url").or(disconnect)).toBeVisible();
  if (await disconnect.isVisible()) {
    await disconnect.click();
    await expect(dlg.getByTestId("publish-url")).toBeVisible();
  }
  await dlg.getByTestId("publish-url").fill(process.env.PDX_E2E_SERVER!);
  await dlg.getByTestId("publish-id").fill(id);

  await dlg.getByTestId("publish-key").fill("falsch");
  await page.getByTestId("publish-connect").click();
  await expect(dlg.getByTestId("publish-error")).toContainText("API-Schlüssel");

  await dlg.getByTestId("publish-key").fill(e2eServerKey);
  await page.getByTestId("publish-connect").click();
  await expect(dlg.getByTestId("publish-connected")).toBeVisible();
  await expect(dlg.getByTestId("publish-diff")).toContainText("noch nicht");
  await expect(dlg.locator('[data-file="template.xml"]')).toContainText("neu");

  await page.getByTestId("publish-upload-activate").click();
  await expect(dlg.getByTestId("publish-result")).toContainText("Version 1 angelegt");
  await expect(dlg.getByTestId("publish-result")).toContainText("Version 1 ist aktiv");
  await expect(dlg.getByTestId("publish-same")).toBeVisible();
  await page.getByRole("button", { name: "Schließen" }).click();

  // Change: margin of the first page master.
  await selectInTree(page, ["seiten", "erste"]);
  const margin = page.locator('[data-prop="margin"] input');
  await margin.fill("20mm");
  await margin.press("Enter");
  await page.getByTestId("publish-open").click();
  await expect(dlg).toHaveAttribute("data-ready", "true");
  await expect(dlg.getByTestId("publish-connected")).toBeVisible();
  await dlg.getByTestId("publish-id").fill(id);
  await dlg.getByTestId("publish-id").press("Enter");
  await expect(dlg.getByTestId("publish-diff")).toContainText("Aktive Version auf dem Server: 1");
  await expect(dlg.getByTestId("publish-lines")).toContainText('margin="20mm"');

  await page.getByTestId("publish-upload").click();
  await expect(dlg.getByTestId("publish-result")).toContainText("Version 2 angelegt");
  await page.getByTestId("publish-activate").click();
  await expect(dlg.getByTestId("publish-result")).toContainText("Version 2 ist aktiv");
  await expect(dlg.getByTestId("publish-diff")).toContainText("Aktive Version auf dem Server: 2 (von 2)");

  // The key appears nowhere in the document of the page, disconnecting forgets it.
  expect(await page.content()).not.toContain(e2eServerKey);
  await disconnect.click();
  await expect(dlg.getByTestId("publish-key")).toHaveValue("");
  await expect(dlg.getByTestId("publish-url")).toHaveValue(process.env.PDX_E2E_SERVER!);
});
