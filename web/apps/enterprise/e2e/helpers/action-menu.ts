import { expect, type Locator, type Page } from "@playwright/test";

export async function openActionMenu(row: Locator) {
  await row.getByRole("button", { name: /更多操作|More actions/ }).click();
  const menu = row
    .page()
    .locator(".argus-dropdown:not([data-exiting])")
    .getByRole("menu");
  await expect(menu).toBeVisible();
  const id = await menu.getAttribute("id");
  if (!id)
    throw new Error("The action menu is missing its accessible element ID");
  return row.page().locator(`[id=${JSON.stringify(id)}]`);
}

export async function selectRowAction(row: Locator, name: string | RegExp) {
  const menu = await openActionMenu(row);
  await menu
    .getByRole("menuitem", { name, exact: typeof name === "string" })
    .click();
  await expect(menu).toHaveCount(0);
}

export async function selectFirstRowAction(page: Page, name: string | RegExp) {
  const row = page
    .locator("tbody > tr")
    .filter({
      has: page.getByRole("button", { name: /更多操作|More actions/ }),
    })
    .first();
  await selectRowAction(row, name);
}
