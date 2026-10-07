import { expect, test, type Page } from "@playwright/test";
import { metricCharts } from "../src/components/dashboards/model";
test.use({ actionTimeout: 15000 });
async function createDashboard(page: Page, name: string, english = false) {
  await page.addInitScript(
    ({ english }) => {
      localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
      localStorage.setItem("argus.theme", english ? "dark" : "light");
    },
    { english },
  );
  await page.goto("/login");
  await page.getByLabel(english ? "Username" : "用户名").fill("root");
  await page.getByLabel(english ? "Password" : "密码").fill("123456");
  await page
    .getByRole("button", { name: english ? "Sign in" : "登录", exact: true })
    .click();
  await expect(page).not.toHaveURL(/login/);
  await page.goto("/dashboards");
  await page
    .getByRole("button", {
      name: english ? "Create dashboard" : "新建仪表盘",
      exact: true,
    })
    .click();
  await page
    .getByRole("textbox", { name: english ? "Name" : "名称", exact: true })
    .fill(name);
  await page
    .getByRole("button", { name: english ? "Done" : "完成", exact: true })
    .click();
}
async function add(page: Page, preset = "CPU 使用率", name = "CPU panel") {
  await page
    .getByRole("button", { name: "添加统计图", exact: true })
    .first()
    .click();
  await page.getByRole("button", { name: preset, exact: true }).click();
  await expect(page).toHaveURL(/panels/);
  await page.getByLabel("统计图标题").fill(name);
  return page;
}
async function done(page: Page) {
  await page.getByRole("button", { name: "完成", exact: true }).click();
  await expect(page).toHaveURL(/dashboard-drafts\/[^/]+$/);
}
async function publish(page: Page) {
  await page.getByRole("button", { name: "预览并发布", exact: true }).click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "确认发布", exact: true })
    .click();
  await expect(page).toHaveURL(/\/dashboards\//);
}
test("personal draft persists layout and publishes through one review", async ({
  page,
}) => {
  await createDashboard(page, "Draft workbench");
  await add(page);
  await done(page);
  const handle = page.getByRole("button", {
    name: "调整统计图尺寸 CPU panel",
    exact: true,
  });
  await handle.press("ArrowRight");
  await expect(page.getByText("草稿已保存", { exact: false })).toBeVisible();
  await page.reload();
  const tile = page.locator(".argus-dashboard-tile").first();
  await expect(tile).toHaveCSS("grid-column-end", "span 7");
  await publish(page);
  await expect(page.locator(".argus-dashboard-tile")).toHaveCount(1);
});
test("query changes mark preview stale, style changes retain it and complex statements stay in statement mode", async ({
  page,
}) => {
  await createDashboard(page, "Statement workbench");
  await add(page);
  await page.getByRole("button", { name: "运行查询", exact: true }).click();
  await expect(page.getByText("最近运行结果", { exact: true })).toBeVisible();
  await page.getByRole("radio", { name: "PromQL", exact: true }).press("Space");
  await expect(
    page.getByRole("radio", { name: "PromQL", exact: true }),
  ).toBeChecked();
  await page
    .getByRole("textbox", { name: "查询语句", exact: true })
    .fill("sum_over_time(system_cpu_utilization[13m]) / clamp_min(2, 1)");
  await expect(page.getByText("待运行", { exact: true })).toBeVisible();
  await page.getByRole("radio", { name: "构建器", exact: true }).press("Space");
  await expect(page.getByRole("alert")).toBeVisible();
  await expect(
    page.getByRole("radio", { name: "PromQL", exact: true }),
  ).toBeChecked();
  await expect(
    page.getByRole("textbox", { name: "查询语句", exact: true }),
  ).toHaveValue("sum_over_time(system_cpu_utilization[13m]) / clamp_min(2, 1)");
  await page.getByRole("button", { name: "完成", exact: true }).click();
  await page.reload();
  await page
    .getByRole("button", { name: "编辑统计图 CPU panel", exact: true })
    .click();
  await expect(
    page.getByRole("textbox", { name: "查询语句", exact: true }),
  ).toHaveValue("sum_over_time(system_cpu_utilization[13m]) / clamp_min(2, 1)");
});
test("eleven metric visualizations render and pointer layout survives reload", async ({
  page,
}) => {
  await createDashboard(page, "Metric gallery");
  await add(page);
  await page.getByRole("button", { name: "运行查询", exact: true }).click();
  await expect(page.getByText("最近运行结果", { exact: true })).toBeVisible();
  const types = [
    "时序曲线",
    "数值",
    "仪表盘",
    "条形仪表",
    "柱状图",
    "饼图",
    "状态时间轴",
    "热力图",
    "散点图",
    "表格",
  ];
  await page
    .getByRole("button", { name: "全部 11 类图型", exact: true })
    .click();
  for (const name of types) {
    await page
      .locator(".argus-panel-editor__settings")
      .getByRole("button", { name, exact: true })
      .click();
    await expect(page.getByText("最近运行结果", { exact: true })).toBeVisible();
  }
  await expect(
    page.getByRole("button", { name: "直方图", exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByRole("button", { name: "热力图", exact: true }),
  ).toBeEnabled();
  await page
    .locator(".argus-panel-editor__settings")
    .getByRole("button", { name: "时序曲线", exact: true })
    .click();
  await page.getByText("高级查询与过滤", { exact: true }).click();
  const metric = page
    .getByRole("combobox", { name: "指标", exact: true })
    .last();
  await metric.fill("http_request_duration_seconds_bucket");
  await page.getByRole("button", { name: "运行查询", exact: true }).click();
  for (const name of ["直方图", "热力图"]) {
    await page
      .locator(".argus-panel-editor__settings")
      .getByRole("button", { name, exact: true })
      .click();
    await page.getByRole("button", { name: "运行查询", exact: true }).click();
    await expect(page.getByText("最近运行结果", { exact: true })).toBeVisible();
  }
  expect(metricCharts).toHaveLength(11);
  await done(page);
  const grid = page.locator(".argus-dashboard-grid"),
    handle = page.getByRole("button", {
      name: "移动统计图 CPU panel",
      exact: true,
    });
  const box = (await handle.boundingBox())!,
    area = (await grid.boundingBox())!;
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(
    box.x + box.width / 2 + area.width / 12,
    box.y + box.height / 2,
    { steps: 5 },
  );
  await page.mouse.up();
  await expect(page.getByText("草稿已保存", { exact: false })).toBeVisible();
  await page.reload();
  await expect(page.locator(".argus-dashboard-tile").first()).toHaveCSS(
    "grid-column-start",
    "2",
  );
});
test("logs, trace and APM templates preserve signal capabilities in published details", async ({
  page,
}) => {
  await createDashboard(page, "Signals");
  for (const [preset, title] of [
    ["日志检索", "Log rows"],
    ["链路列表", "Trace rows"],
    ["应用服务", "Services"],
  ]) {
    await add(page, preset, title);
    await page.getByRole("button", { name: "运行查询", exact: true }).click();
    await expect(page.getByText("最近运行结果", { exact: true })).toBeVisible();
    await done(page);
  }
  await publish(page);
  await expect(page.locator(".argus-dashboard-tile")).toHaveCount(3);
  await expect(
    page.locator(".argus-dashboard-tile").filter({ hasText: "Services" }),
  ).toContainText("已接收");
});

test("dashboard access is granted and removed through the existing authorization drawer", async ({
  page,
}) => {
  test.setTimeout(60000);
  await createDashboard(page, "Shared application dashboard");
  await page.getByRole("button", { name: "预览并发布", exact: true }).click();
  await page
    .getByRole("dialog", { name: "发布预览" })
    .getByRole("button", { name: "确认发布", exact: true })
    .click();
  await expect(page).toHaveURL(/\/dashboards\//);
  await page.goto("/settings/org");
  const user = page.getByRole("row").filter({ hasText: "@chenxi" });
  await user.getByRole("button", { name: "数据授权", exact: true }).click();
  const drawer = page.getByRole("dialog", { name: /^数据授权/ });
  await drawer.getByRole("button", { name: "仪表盘", exact: true }).click();
  await drawer
    .getByRole("checkbox", {
      name: "Shared application dashboard",
      exact: true,
    })
    .locator('xpath=ancestor::*[@data-slot="checkbox-content"]')
    .click();
  await drawer.getByRole("button", { name: "批量移动", exact: true }).click();
  await drawer.getByRole("button", { name: "保存", exact: true }).click();
  await expect(drawer).not.toBeVisible();
  await user.getByRole("button", { name: "数据授权", exact: true }).click();
  await drawer.getByRole("button", { name: "仪表盘", exact: true }).click();
  const authorized = drawer
    .locator(".argus-dual-list__columns > section")
    .nth(1);
  await expect(authorized).toContainText("Shared application dashboard");
  await authorized
    .getByRole("checkbox", {
      name: "Shared application dashboard",
      exact: true,
    })
    .locator('xpath=ancestor::*[@data-slot="checkbox-content"]')
    .click();
  await drawer.getByRole("button", { name: "批量移除", exact: true }).click();
  await drawer.getByRole("button", { name: "保存", exact: true }).click();
  await expect(drawer).not.toBeVisible();
  await user.getByRole("button", { name: "数据授权", exact: true }).click();
  await drawer.getByRole("button", { name: "仪表盘", exact: true }).click();
  await expect(authorized).not.toContainText("Shared application dashboard");
});

for (const resource of [
  { path: "/hosts/host-web-11", first: "host-web-11", second: "host-web-12" },
  {
    path: "/kubernetes/k8s-prod-east",
    first: "k8s-prod-east",
    second: "k8s-staging",
  },
]) {
  test(`resource shortcut ${resource.first} confirms, changes scope and unlinks`, async ({
    page,
  }, testInfo) => {
    test.setTimeout(60000);
    await createDashboard(page, "Resource logs");
    await page
      .getByRole("button", { name: "添加统计图", exact: true })
      .first()
      .click();
    await page.getByRole("button", { name: "日志检索", exact: true }).click();
    await page.getByLabel("统计图标题").fill("Resource entries");
    await done(page);
    await page.getByRole("button", { name: "预览并发布", exact: true }).click();
    await page
      .getByRole("dialog", { name: "发布预览" })
      .getByRole("button", { name: "确认发布", exact: true })
      .click();
    await expect(page).toHaveURL(/\/dashboards\//);
    const dashboardUrl = page.url();
    await page.goto(resource.path);
    await page.getByRole("tab", { name: "仪表盘", exact: true }).click();
    await page
      .getByRole("button", { name: "管理仪表盘关联", exact: true })
      .click();
    const manage = page.getByRole("dialog", { name: "管理仪表盘关联" });
    await manage
      .getByRole("button", { name: "关联 Resource logs", exact: true })
      .click();
    const review = page.getByRole("dialog", { name: "关联变更预览" });
    await expect(review).toContainText(resource.first);
    await review.getByRole("button", { name: "确认执行", exact: true }).click();
    await expect(review).not.toBeVisible();
    await manage.getByRole("button", { name: "关闭", exact: true }).click();
    await page.screenshot({
      path: testInfo.outputPath("resource-dashboard-shortcut.png"),
      fullPage: true,
    });
    await page
      .getByRole("region", { name: "关联仪表盘" })
      .getByRole("link", { name: "Resource logs", exact: true })
      .click();
    await expect(page).toHaveURL(new RegExp(`resource=${resource.first}`));
    await expect(
      page.getByRole("button", { name: "资源范围", exact: false }),
    ).toBeVisible();
    await page.getByRole("button", { name: "资源范围", exact: true }).click();
    const scope = page.getByRole("dialog", { name: "选择资源" });
    await scope
      .getByRole("checkbox", { name: resource.first, exact: true })
      .locator('xpath=ancestor::*[@data-slot="checkbox-content"]')
      .click();
    await scope
      .getByRole("checkbox", { name: resource.second, exact: true })
      .locator('xpath=ancestor::*[@data-slot="checkbox-content"]')
      .click();
    await scope.getByRole("button", { name: "应用", exact: true }).click();
    await expect(
      page.getByRole("button", { name: "资源范围", exact: true }),
    ).toContainText(resource.second);
    await expect(
      page.getByRole("button", { name: "资源范围", exact: false }),
    ).toBeVisible();
    await page.goto(resource.path);
    await page.getByRole("tab", { name: "仪表盘", exact: true }).click();
    await page
      .getByRole("button", { name: "管理仪表盘关联", exact: true })
      .click();
    await manage
      .getByRole("button", { name: "解除关联 Resource logs", exact: true })
      .click();
    await review.getByRole("button", { name: "确认执行", exact: true }).click();
    await expect(review).not.toBeVisible();
    await manage.getByRole("button", { name: "关闭", exact: true }).click();
    await expect(
      page
        .getByRole("region", { name: "关联仪表盘" })
        .getByRole("link", { name: "Resource logs", exact: true }),
    ).toHaveCount(0);
    await page.goto(dashboardUrl);
    await expect(
      page.getByRole("heading", { name: "Resource logs", exact: true }),
    ).toBeVisible();
  });
}
