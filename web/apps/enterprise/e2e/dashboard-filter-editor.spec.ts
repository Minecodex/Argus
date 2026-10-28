import { test, expect } from "@playwright/test";
import { selectTrigger } from "./helpers/select";
test("metric label conditions edit inline, persist and share compact query controls", async ({
  page,
}, info) => {
  await page.addInitScript(() => localStorage.setItem("argus.locale", "zh-CN"));
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/login");
  await page.getByLabel("用户名").fill("root");
  await page.getByLabel("密码").fill("123456");
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page).not.toHaveURL(/login/);
  await page.goto("/dashboards");
  await page.getByRole("button", { name: "新建仪表盘", exact: true }).click();
  await page
    .getByRole("textbox", { name: "名称", exact: true })
    .fill("Inline metric acceptance");
  await page.getByRole("button", { name: "完成", exact: true }).click();
  await page
    .getByRole("button", { name: "添加统计图", exact: true })
    .first()
    .click();
  await page.getByRole("button", { name: "CPU 使用率", exact: true }).click();
  const basics = page.locator(".argus-metric-query-basics > .argus-field");
  await expect(basics).toHaveCount(4);
  const boxes = await Promise.all(
    (await basics.all()).map((field) => field.boundingBox()),
  );
  expect(boxes[1]!.width).toBeLessThanOrEqual(260);
  expect(boxes[0]!.x).toBeLessThan(boxes[1]!.x);
  await page.getByRole("button", { name: "添加条件", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  const row = page.locator(".argus-metric-label-row");
  const field = row.getByRole("combobox", { name: /^标签\*?$/ });
  await field.fill("sta");
  await field.press("ArrowDown");
  await page.getByRole("option", { name: "state", exact: true }).click();
  const value = row.getByRole("combobox", { name: "值", exact: true });
  await value.fill("i");
  await value.press("ArrowDown");
  await page.getByRole("option", { name: "idle", exact: true }).click();
  await expect(page.getByText("草稿已保存", { exact: false })).toBeVisible();
  await page.reload();
  await expect(field).toHaveValue("state");
  await expect(value).toHaveValue("idle");
  await selectTrigger(page, "查询步长").click();
  await page.getByRole("option", { name: "固定", exact: true }).click();
  await page.getByRole("spinbutton", { name: /步长.*秒/ }).fill("30");
  await page.getByRole("button", { name: "运行查询", exact: true }).click();
  await expect(page.getByText("最近运行结果", { exact: true })).toBeVisible();
  await page.screenshot({
    path: info.outputPath("dashboard-inline-builder.png"),
    fullPage: true,
  });
  await page
    .getByRole("button", { name: "移除 state 条件", exact: true })
    .click();
  await expect(row).toHaveCount(0);
});
