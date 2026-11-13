import { expect, test } from "@playwright/test";
import { selectWorkbenchNamespace } from "./helpers/kubernetes-workbench";
import { selectRowAction } from "./helpers/action-menu";
test("KWR Secret removal is explicit and keeps other protected keys", async ({
  page,
}) => {
  await page.goto("/login");
  await page.locator('input[autocomplete="username"]').fill("root");
  await page.locator('input[autocomplete="current-password"]').fill("123456");
  await page.locator('form button[type="submit"]').click();
  await expect(page).not.toHaveURL(/login/);
  await page.goto("/kubernetes/k8s-prod-east?tab=resources");
  await selectWorkbenchNamespace(page, "production");
  await page.getByRole("tab", { name: "配置", exact: true }).click();
  await page.getByRole("tab", { name: "Secret", exact: true }).click();
  const row = page
    .getByRole("row")
    .filter({
      has: page.getByRole("button", { name: "secret-example", exact: true }),
    });
  await selectRowAction(row, "编辑配置");
  const dialog = page.getByRole("dialog", {
    name: "编辑配置 · secret-example",
    exact: true,
  });
  await dialog.getByRole("checkbox", { name: "password", exact: true }).focus();
  await page.keyboard.press("Space");
  await expect(
    dialog.getByRole("checkbox", { name: "password", exact: true }),
  ).toBeChecked();
  await dialog
    .getByRole("button", { name: "验证并预览差异", exact: true })
    .click();
  await expect(dialog).toContainText("password");
  await dialog.getByRole("button", { name: "确认执行", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  await expect(page.locator(".argus-k8s-operation-feedback")).toContainText(
    "已生效",
  );
  await row
    .getByRole("button", { name: "secret-example", exact: true })
    .click();
  const detail = page.getByRole("dialog", {
    name: "Secret · secret-example",
    exact: true,
  });
  await detail.getByRole("tab", { name: "YAML / JSON", exact: true }).click();
  await expect(detail.locator("pre")).not.toContainText("password:");
});
