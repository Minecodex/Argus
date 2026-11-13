import { expect, test } from "@playwright/test";
test("KWR resource panel edits publish and cluster selection can reset", async ({
  page,
}, info) => {
  test.setTimeout(90000);
  await page.goto("/login");
  await page.locator('input[autocomplete="username"]').fill("root");
  await page.locator('input[autocomplete="current-password"]').fill("123456");
  await page.locator('form button[type="submit"]').click();
  await expect(page).not.toHaveURL(/login/);
  await page.goto("/dashboards");
  await page.getByRole("button", { name: "新建仪表盘", exact: true }).click();
  await page
    .getByRole("textbox", { name: "名称", exact: true })
    .fill("KWR resources");
  await page.getByRole("button", { name: "完成", exact: true }).click();
  await page
    .getByRole("button", { name: "添加统计图", exact: true })
    .first()
    .click();
  await page.getByRole("tab", { name: "资源与告警面板", exact: true }).click();
  await page.getByRole("button", { name: "资源状态摘要", exact: true }).click();
  await expect(page).toHaveURL(/panels/);
  await page.getByLabel("统计图标题").fill("KWR live resources");
  await page.getByRole("button", { name: /预览集群/ }).click();
  await page
    .getByRole("option", { name: "k8s-prod-east", exact: true })
    .click();
  await page.getByRole("button", { name: "运行查询", exact: true }).click();
  await expect(page.locator(".argus-resource-summary")).toBeVisible();
  await page.getByRole("button", { name: "返回", exact: true }).click();
  await page.getByRole("button", { name: "预览并发布", exact: true }).click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "确认发布", exact: true })
    .click();
  await expect(page).toHaveURL(/\/dashboards\//);
  await expect(page.locator(".argus-dashboard-tile")).toHaveCount(1);
  await page.goto("/kubernetes/k8s-prod-east");
  await page.getByRole("button", { name: "选择大盘", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "选择大盘", exact: true });
  await dialog.getByRole("searchbox").fill("KWR resources");
  await dialog
    .getByRole("row")
    .filter({ hasText: "KWR resources" })
    .getByRole("button", { name: "选择", exact: true })
    .click();
  await dialog.getByRole("button", { name: /^确认/ }).click();
  await expect(dialog).not.toBeVisible();
  await expect(page.locator(".argus-dashboard-tile")).toHaveCount(1);
  await expect(
    page.getByText("KWR live resources", { exact: true }),
  ).toBeVisible();
  await page.screenshot({
    path: info.outputPath("custom-overview.png"),
    fullPage: true,
  });
  await page.getByRole("button", { name: "选择大盘", exact: true }).click();
  await dialog
    .getByRole("button", { name: "恢复默认大盘", exact: true })
    .click();
  await dialog.getByRole("button", { name: /^确认/ }).click();
  await expect(dialog).not.toBeVisible();
  await expect(page.locator(".argus-dashboard-tile")).toHaveCount(12);
});
