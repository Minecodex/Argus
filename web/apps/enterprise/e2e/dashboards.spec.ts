import { expect, test, type Page } from "@playwright/test";
test.use({ actionTimeout: 10000 });

for (const english of [false, true]) {
  test(`display options persist, publish and expand in ${english ? "English dark" : "Chinese light"}`, async ({
    page,
  }, testInfo) => {
    const tx = (zh: string, en: string) => (english ? en : zh);
    await createDashboard(page, "Display workbench", english);
    const add = async (title: string) => {
      await page
        .getByRole("button", {
          name: tx("添加统计图", "Add panel"),
          exact: true,
        })
        .first()
        .click();
      const editor = page.getByRole("dialog", {
        name: tx("添加统计图", "Add panel"),
        exact: true,
      });
      await editor.getByLabel(tx("统计图标题", "Panel title")).fill(title);
      return editor;
    };
    let editor = await add("CPU area");
    await editor
      .getByRole("combobox", {
        name: tx("图形样式", "Draw style"),
        exact: true,
      })
      .click();
    await page
      .getByRole("option", { name: tx("面积", "Area"), exact: true })
      .click();
    await editor.getByLabel(tx("堆叠序列", "Stack series")).check();
    await editor.getByLabel(tx("平滑折线", "Smooth line")).check();
    await editor.getByLabel(tx("显示最小值", "Display minimum")).fill("0");
    await editor.getByLabel(tx("显示最大值", "Display maximum")).fill("100");
    await editor
      .getByRole("button", {
        name: tx("添加阈值", "Add threshold"),
        exact: true,
      })
      .click();
    await editor
      .getByRole("spinbutton", {
        name: tx("阈值 1", "Threshold 1"),
        exact: true,
      })
      .fill("80");
    await editor
      .getByRole("button", { name: tx("完成", "Done"), exact: true })
      .click();
    await expect(
      page.getByText(tx("草稿已保存", "Draft saved"), { exact: false }),
    ).toBeVisible();
    await page.reload();
    await page
      .getByRole("button", {
        name: tx("编辑统计图 CPU area", "Edit panel CPU area"),
        exact: true,
      })
      .click();
    editor = page.getByRole("dialog", {
      name: tx("编辑统计图", "Edit panel"),
      exact: true,
    });
    await expect(
      editor.getByRole("combobox", {
        name: tx("图形样式", "Draw style"),
        exact: true,
      }),
    ).toContainText(tx("面积", "Area"));
    await expect(
      editor.getByLabel(tx("显示最小值", "Display minimum")),
    ).toHaveValue("0");
    await expect(
      editor.getByLabel(tx("堆叠序列", "Stack series")),
    ).toBeChecked();
    await expect(
      editor.getByRole("spinbutton", {
        name: tx("阈值 1", "Threshold 1"),
        exact: true,
      }),
    ).toHaveValue("80");
    await editor
      .getByText(tx("显示配置", "Display options"), { exact: true })
      .scrollIntoViewIfNeeded();
    await page.screenshot({
      path: testInfo.outputPath("display-editor.png"),
      fullPage: true,
    });
    await editor
      .getByRole("button", { name: tx("完成", "Done"), exact: true })
      .click();
    editor = await add("CPU sample mean");
    await editor
      .getByRole("combobox", { name: tx("图型", "Visualization"), exact: true })
      .click();
    await page
      .getByRole("option", { name: tx("数值", "Stat"), exact: true })
      .click();
    await expect(
      editor.getByRole("combobox", {
        name: tx("查询模式", "Query mode"),
        exact: true,
      }),
    ).toContainText(tx("时间序列", "Time series"));
    await editor
      .getByRole("combobox", {
        name: tx("显示计算", "Value calculation"),
        exact: true,
      })
      .click();
    await page
      .getByRole("option", {
        name: tx("样本平均值", "Sample mean"),
        exact: true,
      })
      .click();
    await editor
      .getByRole("button", {
        name: tx("添加阈值", "Add threshold"),
        exact: true,
      })
      .click();
    await editor
      .getByRole("combobox", {
        name: tx("颜色含义 1", "Color meaning 1"),
        exact: true,
      })
      .click();
    await page
      .getByRole("option", { name: tx("严重", "Critical"), exact: true })
      .click();
    await editor
      .getByRole("button", { name: tx("完成", "Done"), exact: true })
      .click();
    await page
      .getByRole("button", {
        name: tx("预览并发布", "Preview and publish"),
        exact: true,
      })
      .click();
    const review = page.getByRole("dialog", {
      name: tx("发布预览", "Publication preview"),
      exact: true,
    });
    await expect(review).toContainText("CPU sample mean");
    await review
      .getByRole("button", {
        name: tx("确认发布", "Confirm publication"),
        exact: true,
      })
      .click();
    await expect(page).toHaveURL(/\/dashboards\//);
    await expect(page.locator("[data-panel-id] canvas")).toBeVisible();
    await page
      .getByRole("button", {
        name: tx("数据时间与完整性", "Data time and completeness"),
        exact: true,
      })
      .first()
      .click();
    const freshness = page.getByRole("dialog", {
      name: tx("数据时间与完整性", "Data time and completeness"),
      exact: true,
    });
    await expect(freshness).toContainText(tx("查询完成", "Query completed"));
    await expect(freshness).toContainText(
      tx("摄入完整性: 未知", "Ingestion completeness: Unknown"),
    );
    await page.screenshot({
      path: testInfo.outputPath("data-freshness.png"),
      fullPage: true,
    });
    await page.keyboard.press("Escape");
    await expect(
      page
        .locator('.argus-observation-stat strong[data-tone="danger"]')
        .first(),
    ).toBeVisible();
    await page.screenshot({
      path: testInfo.outputPath("published-display.png"),
      fullPage: true,
    });
    await page
      .getByRole("button", {
        name: tx("展开 CPU sample mean", "Expand CPU sample mean"),
        exact: true,
      })
      .click();
    const expanded = page.getByRole("dialog", {
      name: "CPU sample mean",
      exact: true,
    });
    await expect(
      expanded.locator('strong[data-tone="danger"]').first(),
    ).toBeVisible();
    await expect(expanded).toContainText(
      tx("按样本平均，未按时长加权", "Sample mean, not time weighted"),
    );
  });
}

test("personal draft persists layout and publishes through one review", async ({
  page,
}, testInfo) => {
  await page.addInitScript(() => localStorage.setItem("argus.locale", "zh-CN"));
  await page.goto("/login");
  await page.getByLabel("用户名").fill("root");
  await page.getByLabel("密码").fill("123456");
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page).not.toHaveURL(/\/login/);
  await page.goto("/dashboards");
  await page.getByRole("button", { name: "新建仪表盘", exact: true }).click();
  const create = page.getByRole("dialog", { name: "新建仪表盘" });
  await create
    .getByRole("textbox", { name: "名称", exact: true })
    .fill("Production overview");
  await create.getByRole("button", { name: "完成", exact: true }).click();
  await expect(page).toHaveURL(/dashboard-drafts\//);
  await page
    .getByRole("button", { name: "添加统计图", exact: true })
    .first()
    .click();
  const panel = page.getByRole("dialog", { name: "添加统计图" });
  await panel.getByLabel("统计图标题").fill("CPU trend");
  await panel.getByRole("button", { name: "完成", exact: true }).click();
  const move = page.getByRole("button", { name: "移动统计图 CPU trend" });
  await move.focus();
  await page.keyboard.press("ArrowRight");
  await page.getByRole("button", { name: "调整统计图尺寸 CPU trend" }).focus();
  await page.keyboard.press("ArrowRight");
  await expect(page.getByText("草稿已保存", { exact: false })).toBeVisible();
  await page.reload();
  await expect(page.getByText("CPU trend", { exact: true })).toBeVisible();
  const tile = page.locator("[data-panel-id]").first();
  await expect(tile).toHaveCSS("grid-column-start", "2");
  await expect(tile).toHaveCSS("grid-column-end", "span 7");
  await page.getByRole("button", { name: "运行样本" }).click();
  await expect(tile.locator("canvas")).toBeVisible();
  await page.getByRole("button", { name: "预览并发布" }).click();
  const review = page.getByRole("dialog", { name: "发布预览" });
  await review.getByRole("button", { name: "确认发布", exact: true }).click();
  await expect(page).toHaveURL(/\/dashboards\//);
  await expect(
    page.getByRole("heading", { name: "Production overview" }),
  ).toBeVisible();
  await expect(page.locator("[data-panel-id] canvas")).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("published-dashboard.png"),
    fullPage: true,
  });
  await page.getByRole("button", { name: "编辑", exact: true }).click();
  await page.getByRole("button", { name: "编辑统计图 CPU trend" }).click();
  const edit = page.getByRole("dialog", { name: "编辑统计图" });
  await edit.getByLabel("统计图标题").fill("Draft only change");
  await edit.getByRole("button", { name: "完成", exact: true }).click();
  await page.getByRole("button", { name: "返回目录", exact: true }).click();
  await page
    .getByRole("button", { name: "Production overview", exact: true })
    .click();
  await expect(page.getByText("CPU trend", { exact: true })).toBeVisible();
  await expect(
    page.getByText("Draft only change", { exact: true }),
  ).not.toBeVisible();
});

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
  await expect(page).not.toHaveURL(/\/login/);
  await page.goto("/dashboards");
  await page
    .getByRole("button", {
      name: english ? "Create dashboard" : "新建仪表盘",
      exact: true,
    })
    .click();
  const dialog = page.getByRole("dialog");
  await dialog
    .getByRole("textbox", { name: english ? "Name" : "名称", exact: true })
    .fill(name);
  await dialog
    .getByRole("button", { name: english ? "Done" : "完成", exact: true })
    .click();
  await expect(page).toHaveURL(/dashboard-drafts\//);
}

test("field exploration and request error rate survive editor refresh", async ({
  page,
}, testInfo) => {
  await createDashboard(page, "Query authoring");
  await page
    .getByRole("button", { name: "添加统计图", exact: true })
    .first()
    .click();
  let panel = page.getByRole("dialog", { name: "添加统计图", exact: true });
  await panel.getByLabel("统计图标题").fill("Request ratio");
  await panel.getByRole("combobox", { name: "运算", exact: true }).click();
  await page.getByRole("option", { name: "请求错误率", exact: true }).click();
  await panel
    .getByRole("combobox", { name: "指标", exact: true })
    .fill("http_requests_total");
  await panel.getByRole("combobox", { name: "来源类型", exact: true }).click();
  await page.getByRole("option", { name: "otlp", exact: true }).click();
  await panel.getByRole("combobox", { name: "显示单位", exact: true }).click();
  await page
    .getByRole("option", { name: "比例百分比（0–1）", exact: true })
    .click();
  const errors = panel.getByRole("region", { name: "错误请求条件" });
  await expect(
    errors.getByRole("textbox", { name: "值", exact: true }),
  ).toHaveValue("5..");
  await panel.getByRole("button", { name: "完成", exact: true }).click();
  await page
    .getByRole("button", { name: "添加统计图", exact: true })
    .first()
    .click();
  panel = page.getByRole("dialog", { name: "添加统计图", exact: true });
  await panel.getByLabel("统计图标题").fill("Observed logs");
  await panel.getByRole("combobox", { name: "信号", exact: true }).click();
  await page.getByRole("option", { name: "logs", exact: true }).click();
  await panel
    .locator("summary")
    .filter({ hasText: /^字段探索$/ })
    .click();
  const explorer = panel.getByRole("region", { name: "字段探索" });
  await explorer.getByRole("button", { name: "读取字段", exact: true }).click();
  await explorer.getByRole("button", { name: "更多字段", exact: true }).click();
  await expect(
    explorer.getByRole("button", {
      name: "structured_metadata.field_29",
      exact: true,
    }),
  ).toBeVisible();
  await explorer
    .getByRole("textbox", { name: "搜索字段", exact: true })
    .fill("service");
  await explorer.getByRole("button", { name: "读取字段", exact: true }).click();
  await explorer
    .getByRole("button", { name: "service_name", exact: true })
    .click();
  await explorer
    .getByRole("button", {
      name: "添加条件 service_name = checkout",
      exact: true,
    })
    .click();
  await expect(
    panel.getByRole("textbox", { name: "字段", exact: true }),
  ).toHaveValue("service_name");
  await page.screenshot({
    path: testInfo.outputPath("field-explorer.png"),
    fullPage: true,
    animations: "disabled",
  });
  await panel.getByRole("button", { name: "完成", exact: true }).click();
  await expect(page.getByText("草稿已保存", { exact: false })).toBeVisible();
  await page.reload();
  await page
    .getByRole("button", { name: "编辑统计图 Request ratio", exact: true })
    .click();
  panel = page.getByRole("dialog", { name: "编辑统计图", exact: true });
  await expect(
    panel.getByRole("combobox", { name: "运算", exact: true }).first(),
  ).toHaveText(/请求错误率/);
  await expect(
    panel.getByRole("combobox", { name: "显示单位", exact: true }),
  ).toHaveText(/比例百分比/);
  await expect(
    panel
      .getByRole("region", { name: "错误请求条件" })
      .getByRole("textbox", { name: "值", exact: true }),
  ).toHaveValue("5..");
  await panel
    .getByRole("region", { name: "错误请求条件" })
    .scrollIntoViewIfNeeded();
  await page.screenshot({
    path: testInfo.outputPath("error-rate-builder.png"),
    fullPage: true,
    animations: "disabled",
  });
  await panel.getByRole("button", { name: "完成", exact: true }).click();
  await page
    .getByRole("button", { name: "编辑统计图 Observed logs", exact: true })
    .click();
  await expect(
    panel.getByRole("textbox", { name: "字段", exact: true }),
  ).toHaveValue("service_name");
  await expect(
    panel.getByRole("textbox", { name: "值", exact: true }),
  ).toHaveValue("checkout");
});

test("panel conversion preserves statements and nested detail edits until publication", async ({
  page,
}) => {
  await createDashboard(page, "Conversion workbench");
  await page
    .getByRole("button", { name: "添加统计图", exact: true })
    .first()
    .click();
  let panel = page.getByRole("dialog", { name: "添加统计图", exact: true });
  await panel.getByLabel("统计图标题").fill("Convertible metric");
  await panel.getByRole("button", { name: "完成", exact: true }).click();
  await page.getByRole("button", { name: "生成标准下钻", exact: true }).click();
  await page
    .getByRole("button", { name: "编辑统计图 Convertible metric", exact: true })
    .click();
  panel = page.getByRole("dialog", { name: "编辑统计图", exact: true });
  await panel.getByRole("combobox", { name: "编辑来源", exact: true }).click();
  await page.getByRole("option", { name: "查询语句", exact: true }).click();
  const expression = panel.getByRole("textbox", {
    name: "查询语句",
    exact: true,
  });
  await expect(expression).toHaveValue("system_cpu_utilization{}");
  await expression.fill("system_cpu_utilization + 1");
  await panel.getByRole("combobox", { name: "编辑来源", exact: true }).click();
  await page.getByRole("option", { name: "构建器", exact: true }).click();
  await expect(panel.getByRole("alert")).toContainText(
    "模拟模式只演示基本指标转换",
  );
  await expect(expression).toHaveValue("system_cpu_utilization + 1");
  await expression.fill("system_cpu_utilization{}");
  await panel.getByRole("combobox", { name: "编辑来源", exact: true }).click();
  await page.getByRole("option", { name: "构建器", exact: true }).click();
  await expect(
    panel.getByRole("combobox", { name: "指标", exact: true }),
  ).toHaveValue("system_cpu_utilization");
  await panel
    .locator("summary")
    .filter({ hasText: /^下钻详情/ })
    .click();
  await panel
    .getByRole("textbox", { name: "入口名称", exact: true })
    .fill("Inspect total");
  await panel
    .getByRole("button", { name: "编辑详情查询", exact: true })
    .click();
  const detail = page.getByRole("dialog", {
    name: "编辑详情查询",
    exact: true,
  });
  await detail.getByRole("combobox", { name: "运算", exact: true }).click();
  await page.getByRole("option", { name: "求和", exact: true }).click();
  await detail.getByRole("button", { name: "完成", exact: true }).click();
  await expect(panel).toBeVisible();
  await panel.getByRole("button", { name: "完成", exact: true }).click();
  await expect(page.getByText("草稿已保存", { exact: false })).toBeVisible();
  await page.reload();
  await page
    .getByRole("button", { name: "编辑统计图 Convertible metric", exact: true })
    .click();
  await panel
    .locator("summary")
    .filter({ hasText: /^下钻详情/ })
    .click();
  await expect(
    panel.getByRole("textbox", { name: "入口名称", exact: true }),
  ).toHaveValue("Inspect total");
  await panel
    .getByRole("button", { name: "编辑详情查询", exact: true })
    .click();
  await expect(
    detail.getByRole("combobox", { name: "运算", exact: true }),
  ).toHaveText(/求和/);
  await detail.getByRole("button", { name: "完成", exact: true }).click();
  await panel.getByRole("button", { name: "完成", exact: true }).click();
  await page.getByRole("button", { name: "预览并发布", exact: true }).click();
  await page
    .getByRole("dialog", { name: "发布预览" })
    .getByRole("button", { name: "确认发布", exact: true })
    .click();
  await expect(page).toHaveURL(/\/dashboards\//);
  await page.getByRole("button", { name: "查看查询", exact: true }).click();
  const publishedQueries = page.getByRole("dialog", {
    name: "Convertible metric · 查看查询",
  });
  await expect(publishedQueries).toContainText("detail_main");
  await expect(publishedQueries).toContainText('"operation": "sum"');
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
    .check();
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
    .check();
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
    const panel = page.getByRole("dialog", { name: "添加统计图" });
    await panel.getByLabel("统计图标题").fill("Resource entries");
    await panel.getByRole("combobox", { name: "信号", exact: true }).click();
    await page.getByRole("option", { name: "logs", exact: true }).click();
    await panel.getByRole("button", { name: "完成", exact: true }).click();
    await page.getByRole("button", { name: "预览并发布", exact: true }).click();
    await page
      .getByRole("dialog", { name: "发布预览" })
      .getByRole("button", { name: "确认发布", exact: true })
      .click();
    await expect(page).toHaveURL(/\/dashboards\//);
    const dashboardUrl = page.url();
    await page.goto(resource.path);
    await page
      .getByRole("button", { name: "管理仪表盘关联", exact: true })
      .click();
    const manage = page.getByRole("dialog", { name: "管理仪表盘关联" });
    await manage
      .getByRole("combobox", { name: "选择仪表盘", exact: true })
      .click();
    await page
      .getByRole("option", { name: "Resource logs", exact: true })
      .click();
    await manage.getByRole("button", { name: "预览关联", exact: true }).click();
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
      page.getByRole("cell", { name: resource.first, exact: true }),
    ).toBeVisible();
    await page.getByRole("button", { name: /资源范围:/ }).click();
    const scope = page.getByRole("dialog", { name: "选择资源" });
    await scope
      .getByRole("checkbox", { name: resource.first, exact: true })
      .uncheck();
    await scope
      .getByRole("checkbox", { name: resource.second, exact: true })
      .check();
    await scope.getByRole("button", { name: "应用", exact: true }).click();
    await expect(
      page.getByRole("cell", { name: resource.second, exact: true }),
    ).toBeVisible();
    await page.goto(resource.path);
    await page
      .getByRole("button", { name: "管理仪表盘关联", exact: true })
      .click();
    await manage.getByRole("button", { name: "解除关联", exact: true }).click();
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

test("eleven metric visualizations render and pointer layout survives reload", async ({
  page,
}, testInfo) => {
  test.setTimeout(90000);
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await createDashboard(page, "Metrics gallery");
  const types = [
    "时序曲线",
    "数值",
    "仪表盘",
    "条形仪表",
    "柱状图",
    "饼图",
    "直方图",
    "热力图",
    "状态时间轴",
    "散点图",
    "表格",
  ];
  for (const [index, type] of types.entries()) {
    await page
      .getByRole("button", { name: "添加统计图", exact: true })
      .first()
      .click();
    const dialog = page.getByRole("dialog", { name: "添加统计图" });
    await dialog.getByLabel("统计图标题").fill(`Metric ${index + 1}`);
    await dialog.getByRole("combobox", { name: "图型", exact: true }).click();
    await page.getByRole("option", { name: type, exact: true }).click();
    await dialog.getByRole("button", { name: "完成", exact: true }).click();
  }
  const move = page.getByRole("button", {
    name: "移动统计图 Metric 1",
    exact: true,
  });
  await move.scrollIntoViewIfNeeded();
  const box = await move.boundingBox();
  await page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2);
  await page.mouse.down();
  await page.mouse.move(
    box!.x + box!.width / 2 + 100,
    box!.y + box!.height / 2,
    { steps: 6 },
  );
  await page.mouse.up();
  await expect(page.getByText("草稿已保存", { exact: false })).toBeVisible();
  await page.reload();
  await expect(page.locator("[data-panel-id]").first()).not.toHaveCSS(
    "grid-column-start",
    "1",
  );
  await page.getByRole("button", { name: "运行样本" }).click();
  for (let i = 0; i < types.length; i++) {
    const tile = page.locator("[data-panel-id]").nth(i);
    if (i === 1)
      await expect(tile.locator("strong").nth(1)).toContainText(/\d/);
    else if (i === 10) await expect(tile.locator("tbody tr")).toHaveCount(2);
    else await expect(tile.locator("canvas").first()).toBeVisible();
  }
  await page.screenshot({
    path: testInfo.outputPath("metrics-gallery.png"),
    fullPage: true,
  });
  expect(errors).toEqual([]);
});

test("English dark trace and APM details expose sample statistics", async ({
  page,
}, testInfo) => {
  test.setTimeout(90000);
  await createDashboard(page, "Application health", true);
  for (const chart of ["Trace detail", "APM RED", "Service topology"]) {
    await page
      .getByRole("button", { name: "Add panel", exact: true })
      .first()
      .click();
    const dialog = page.getByRole("dialog", { name: "Add panel" });
    await dialog.getByLabel("Panel title").fill(chart);
    await dialog.getByRole("combobox", { name: "Signal", exact: true }).click();
    await page.getByRole("option", { name: "traces", exact: true }).click();
    await dialog
      .getByRole("combobox", { name: "Visualization", exact: true })
      .click();
    await page.getByRole("option", { name: chart, exact: true }).click();
    if (chart === "Trace detail")
      await dialog.getByLabel("Trace ID", { exact: false }).fill("mock-trace");
    await dialog.getByRole("button", { name: "Done", exact: true }).click();
  }
  await page.getByRole("button", { name: "Run sample", exact: true }).click();
  const trace = page.locator("[data-panel-id]").first();
  await expect(
    trace.getByRole("button", { name: "checkout / GET /orders", exact: true }),
  ).toBeVisible();
  await trace
    .getByRole("button", { name: "checkout / GET /orders", exact: true })
    .click();
  await expect(
    trace.getByRole("region", { name: "Span details" }),
  ).toBeVisible();
  await expect(
    page.getByRole("combobox", { name: "RED metric" }),
  ).toBeVisible();
  await expect(
    page
      .getByText("Received sample statistics; no extrapolation", {
        exact: true,
      })
      .first(),
  ).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("trace-apm-dark.png"),
    fullPage: true,
  });
});
