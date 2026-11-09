import { expect, type Page } from "@playwright/test";

export async function selectWorkbenchNamespace(page: Page, name: string) {
  const select = page.getByRole("combobox", {
    name: /^(命名空间范围|Namespace scope)$/,
  });
  await select.fill(name);
  await page.getByRole("option", { name, exact: true }).click();
  await expect(select).toHaveValue(name);
}
