import { expect, test } from "@playwright/test";

test("Struktur: Kinder einfügen, duplizieren, verschieben, löschen", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("button", { name: "Neu", exact: true }).first().click();
  await expect(page.getByTestId("doc-title")).toHaveText("Neue Vorlage");
  await page.getByRole("tab", { name: "Hierarchie" }).click();
  const tree = page.getByTestId("hierarchy");
  const menu = page.getByTestId("context-menu");
  // Only once the tree shows the new template, not still the previous one.
  await expect(tree.locator(".id", { hasText: /^vorlage$/ })).toBeVisible();

  // Select the root, insert formats via the button.
  await tree.getByRole("button", { name: /^Vorlage/ }).click();
  await tree.getByRole("button", { name: "Aktionen" }).click();
  await expect(menu).toBeVisible();
  await expect(menu.getByRole("menuitem", { name: "Löschen" })).toBeDisabled();
  await menu.getByTestId("insert-formats").click();
  await expect(menu).toHaveCount(0);
  await expect(tree.getByRole("button", { name: /^Formate/ })).toBeVisible();
  await expect(page.getByTestId("inspector").locator("header strong")).toHaveText("Formate");

  // Right click on the root: insert styles.
  await tree.getByRole("button", { name: /^Vorlage/ }).click({ button: "right" });
  await menu.getByTestId("insert-styles").click();
  await expect(tree.getByRole("button", { name: /^Stile/ })).toBeVisible();
  // Formats exist only once.
  await tree.getByRole("button", { name: /^Vorlage/ }).click({ button: "right" });
  await expect(menu).toBeVisible();
  await expect(menu.getByTestId("insert-formats")).toHaveCount(0);
  await page.keyboard.press("Escape");
  await expect(menu).toHaveCount(0);

  // Duplicate, move and delete a style by keyboard.
  const stylesRow = tree.getByRole("treeitem").filter({ has: page.getByRole("button", { name: /^Stile$/ }) });
  await expect(stylesRow).toBeVisible();
  await expect(stylesRow.getByRole("button", { name: /^(Auf|Zu)klappen$/ })).toBeVisible();
  const expand = stylesRow.getByRole("button", { name: "Aufklappen" });
  if (await expand.count()) await expand.click();
  const style = tree.getByRole("button", { name: /^Stil\s/ });
  await expect(style).toHaveCount(1);
  await style.first().click();
  const first = (await style.first().locator(".id").textContent())!;
  await page.keyboard.press("ControlOrMeta+d");
  await expect(style).toHaveCount(2);
  const copy = (await page.getByTestId("inspector").locator("header code").textContent())!;
  expect(copy).not.toBe(first);
  await page.keyboard.press("Alt+ArrowUp");
  await expect(style.first().locator(".id")).toHaveText(copy);
  await page.keyboard.press("Delete");
  await expect(style).toHaveCount(1);
  await expect(style.first().locator(".id")).toHaveText(first);

  // Undo brings the copy back.
  await page.getByRole("button", { name: "Rückgängig", exact: true }).click();
  await expect(style).toHaveCount(2);
});
