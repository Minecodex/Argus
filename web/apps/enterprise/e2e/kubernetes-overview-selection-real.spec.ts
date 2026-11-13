import { readFileSync, writeFileSync } from "node:fs";
import { expect, test } from "@playwright/test";
test.use({ trace: "off", video: "off", reducedMotion: "reduce" });
test("KWR actual native dashboard publish select and reset", async ({
  page,
}, info) => {
  test.skip(
    process.env.ARGUS_KWR_REAL !== "1",
    "Requires actual owned cluster",
  );
  test.setTimeout(180000);
  const root = process.env.ARGUS_KWR_ARTIFACTS!,
    lab = JSON.parse(readFileSync(root + "/lab-resources.json", "utf8"));
  await page.goto("/login");
  await page.locator('input[autocomplete="username"]').fill("kakj");
  await page
    .locator('input[autocomplete="current-password"]')
    .fill(process.env.ARGUS_KWR_PASSWORD!);
  await page.locator('form button[type="submit"]').click();
  await expect(page).not.toHaveURL(/login/);
  await page.goto("/dashboards");
  await page.getByRole("button", { name: "新建仪表盘", exact: true }).click();
  await page
    .getByRole("textbox", { name: "名称", exact: true })
    .fill("KWR publish acceptance");
  await page.getByRole("button", { name: "完成", exact: true }).click();
  await page
    .getByRole("button", { name: "添加统计图", exact: true })
    .first()
    .click();
  await page.getByRole("tab", { name: "资源与告警面板", exact: true }).click();
  await page.getByRole("button", { name: "资源状态摘要", exact: true }).click();
  await page
    .getByRole("textbox", { name: /统计图标题/ })
    .fill("KWR actual resources");
  await page.getByRole("button", { name: /预览集群/ }).click();
  await page
    .getByRole("option", { name: "KWR temporary acceptance", exact: true })
    .click();
  await page.getByRole("button", { name: "运行查询", exact: true }).click();
  await expect(page.locator(".argus-resource-summary")).toBeVisible({
    timeout: 30000,
  });
  await page.getByRole("button", { name: "返回", exact: true }).click();
  await page.getByRole("button", { name: "预览并发布", exact: true }).click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "确认发布", exact: true })
    .click();
  await expect(page).toHaveURL(/\/dashboards\//, { timeout: 30000 });
  const dashboardId = new URL(page.url()).pathname.split("/").at(-1)!;
  writeFileSync(
    root + "/real-custom-board.json",
    JSON.stringify({ id: dashboardId, name: "KWR publish acceptance" }),
  );
  await page.goto("/kubernetes/" + lab.cluster);
  await page.getByRole("button", { name: "选择大盘", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "选择大盘", exact: true });
  await dialog.getByRole("searchbox").fill("KWR publish acceptance");
  await dialog
    .getByRole("row")
    .filter({ hasText: "KWR publish acceptance" })
    .getByRole("button", { name: "选择", exact: true })
    .click();
  await dialog.getByRole("button", { name: /^确认/ }).click();
  await expect(dialog).not.toBeVisible({ timeout: 30000 });
  await expect(page.locator(".argus-dashboard-tile")).toHaveCount(1);
  await expect(
    page.getByText("KWR actual resources", { exact: true }),
  ).toBeVisible();
  await page.screenshot({
    path: info.outputPath("actual-selected-dashboard.png"),
    fullPage: true,
  });
  await page.getByRole("button", { name: "选择大盘", exact: true }).click();
  await dialog
    .getByRole("button", { name: "恢复默认大盘", exact: true })
    .click();
  await dialog.getByRole("button", { name: /^确认/ }).click();
  await expect(dialog).not.toBeVisible({ timeout: 30000 });
  await expect(page.locator(".argus-dashboard-tile")).toHaveCount(12);
  await page.screenshot({
    path: info.outputPath("actual-reset-dashboard.png"),
    fullPage: true,
  });
});
