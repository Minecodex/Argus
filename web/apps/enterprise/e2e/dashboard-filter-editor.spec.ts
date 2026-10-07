import { test, expect } from "@playwright/test";
test("query filter dialog cancels changes, applies to the draft and survives refresh", async ({
  page,
}) => {
  await page.addInitScript(() => localStorage.setItem("argus.locale", "zh-CN"));
  await page.goto("/login");
  await page.getByLabel("用户名").fill("root");
  await page.getByLabel("密码").fill("123456");
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page).not.toHaveURL(/login/);
  await page.goto("/dashboards");
  await page.getByRole("button", { name: "新建仪表盘", exact: true }).click();
  await page
    .getByRole("textbox", { name: "名称", exact: true })
    .fill("Filter editor acceptance");
  await page.getByRole("button", { name: "完成", exact: true }).click();
  await page
    .getByRole("button", { name: "添加统计图", exact: true })
    .first()
    .click();
  await page.getByRole("button", { name: "CPU 使用率", exact: true }).click();
  await page.getByRole("button", { name: "运行查询", exact: true }).click();
  await expect(page.getByText("最近运行结果", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "添加条件", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "查询筛选条件",
    exact: true,
  });
  await dialog.getByRole("button", { name: "添加条件", exact: true }).click();
  await dialog
    .getByRole("textbox", { name: "字段", exact: true })
    .fill("host_name");
  await dialog
    .getByRole("textbox", { name: "值", exact: true })
    .fill("host-web-11");
  await dialog.getByRole("button", { name: "取消", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  await expect(page.locator(".argus-panel-editor__filter-chip")).toHaveCount(0);
  await expect(page.getByText("最近运行结果", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "添加条件", exact: true }).click();
  await dialog.getByRole("button", { name: "添加条件", exact: true }).click();
  await expect(
    dialog.getByRole("button", { name: "应用", exact: true }),
  ).toBeDisabled();
  await dialog
    .getByRole("textbox", { name: "字段", exact: true })
    .fill("host_name");
  await dialog
    .getByRole("textbox", { name: "值", exact: true })
    .fill("host-web-11");
  await dialog.getByRole("button", { name: "应用", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  await expect(page.locator(".argus-panel-editor__filter-chip")).toHaveText(
    /host_name.*host-web-11/,
  );
  await expect(page.getByText("待运行", { exact: true })).toBeVisible();
  await expect(page.getByText("草稿已保存", { exact: false })).toBeVisible();
  await page.reload();
  await expect(page.locator(".argus-panel-editor__filter-chip")).toHaveText(
    /host_name.*host-web-11/,
  );
  await page
    .getByRole("button", { name: "移除 host_name 条件", exact: true })
    .click();
  await expect(page.locator(".argus-panel-editor__filter-chip")).toHaveCount(0);
});
