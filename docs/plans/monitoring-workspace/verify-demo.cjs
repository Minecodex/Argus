// Prototype E2E only. Does not contact a cluster, database or telemetry endpoint.
const { chromium, expect } = require("@playwright/test");
const AxeBuilder = require("@axe-core/playwright").default;
const fs = require("node:fs");
const path = require("node:path");
const base =
  process.env.ARGUS_DEMO_URL ||
  "http://127.0.0.1:8766/docs/plans/monitoring-workspace/demo.html";
const out = path.resolve(
  __dirname,
  "../../..",
  "artifacts/monitoring-workspace-forms-20261018",
);
fs.mkdirSync(out, { recursive: true });
const checks = [],
  errors = [];
async function check(name, fn) {
  await fn();
  checks.push(name);
  console.log("PASS " + name);
}
async function run() {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 1000 },
  });
  const page = await context.newPage();
  page.on("pageerror", (err) => errors.push(err.message));
  const action = (name, id) =>
    page
      .locator(`[data-action="${name}"]${id ? `[data-id="${id}"]` : ""}`)
      .first();
  const modal = page.locator("#modal");
  const navigate = async (id) => {
    await action("navigate", id).click();
    await expect(page.locator("main h1")).toBeVisible();
  };
  const next = async () => {
    await action("wizard-next").click();
  };
  const fill = async (name, value) => {
    await modal.locator(`[name="${name}"]`).fill(value);
  };
  const select = async (name, value) => {
    if (name === "app" || name === "service") {
      await action(
        name === "app" ? "select-wizard-apps" : "select-wizard-services",
      ).click();
      const button = page.locator(
        `#picker [data-action="picker-toggle"][data-id="${value}"]`,
      );
      if ((await button.getAttribute("aria-pressed")) !== "true")
        await button.click();
      await action("picker-apply").click();
      return;
    }
    await modal.locator(`[name="${name}"]`).selectOption(value);
  };
  const reset = async () => {
    await page.goto(base);
    await action("reset").click();
  };
  await reset();
  await check("initial monitoring table and no script errors", async () => {
    await expect(
      page.getByRole("heading", { name: "监控", exact: true }),
    ).toBeVisible();
    await expect(page.locator("main tbody tr")).toHaveCount(10);
    await page.screenshot({
      path: path.join(out, "01-monitoring.png"),
      fullPage: true,
    });
  });
  await check("collection toggle cancel and confirm are separate", async () => {
    await action("monitor-toggle", "m1").click();
    await action("close").last().click();
    await expect(action("monitor-toggle", "m1")).toHaveAttribute(
      "aria-checked",
      "true",
    );
    await action("monitor-toggle", "m1").click();
    await action("confirm").click();
    await expect(action("monitor-toggle", "m1")).toHaveAttribute(
      "aria-checked",
      "false",
    );
  });
  await reset();
  await check(
    "MySQL creation, validation failure and multiple shared applications",
    async () => {
      await action("new-monitor").click();
      await next();
      await fill("name", "验收 MySQL");
      await fill("endpoint", "db-demo.example.test:3306");
      await fill("authUsername", "demo_monitor");
      await fill("authPassword", "DEMO_TEST_NOT_A_PASSWORD");
      await action("select-wizard-apps").click();
      await action("picker-toggle", "order").click();
      await action("picker-toggle", "billing").click();
      await action("picker-apply").click();
      await next();
      await select("wizard-scenario", "auth");
      await action("wizard-test").click();
      await expect(action("wizard-next")).toBeDisabled();
      await expect(modal).toContainText("认证权限不足");
      await select("wizard-scenario", "ok");
      await action("wizard-test").click();
      await next();
      await expect(modal).toContainText("结算中心");
      await action("wizard-create").click();
      await expect(page.locator("main")).toContainText("验收 MySQL");
      await expect(page.locator("main")).toContainText("等待数据");
    },
  );
  await check("create application and register a service", async () => {
    await navigate("applications");
    await action("app-list").click();
    await action("new-app").click();
    await fill("name", "履约系统");
    await fill("owner", "履约组");
    await action("app-create").click();
    await action("new-service").click();
    await fill("name", "fulfilment-api");
    await action("service-create").click();
    await expect(page.locator("main")).toContainText("fulfilment-api");
  });
  await check(
    "trace requires application/service and issues a demo key",
    async () => {
      await navigate("traces");
      await action("new-trace").click();
      await next();
      await fill("name", "验收 Trace");
      await next();
      await expect(page.locator("#monitor-form")).toBeVisible();
      await select("app", "order");
      await select("service", "order-api");
      await next();
      await action("wizard-test").click();
      await next();
      await page.screenshot({ path: path.join(out, "02-trace-review.png") });
      await action("wizard-create").click();
      await expect(page.locator("#drawer")).toContainText("DEMO_ONLY_");
      await expect(page.locator("#drawer")).toContainText(
        "telemetry.demo.invalid",
      );
      await action("simulate-ingest").click();
      await page.locator('#drawer [data-action="close"]').click();
      await expect(page.locator("main")).toContainText("验收 Trace");
    },
  );
  await reset();
  await check(
    "Kubernetes business log rule follows workload and prevents overlap",
    async () => {
      await navigate("kubernetes");
      await action("k8s-tab", "logs").click();
      await action("k8s-log").click();
      await fill("name", "重复日志测试");
      await select("app", "order");
      await select("service", "order-api");
      await next();
      await action("wizard-test").click();
      await expect(modal).toContainText("范围重叠");
      await expect(action("wizard-next")).toBeDisabled();
      await action("wizard-back").click();
      await select("workload", "order-worker");
      await select("service", "order-worker");
      await page.screenshot({
        path: path.join(out, "03-kubernetes-log-form.png"),
      });
      await next();
      await action("wizard-test").click();
      await next();
      await action("wizard-create").click();
      await expect(page.locator("main")).toContainText("重复日志测试");
    },
  );
  await check("system file logs do not require an application", async () => {
    await navigate("hosts");
    await action("host-tab", "collection").click();
    await action("host-log").click();
    await fill("name", "系统文件日志");
    await select("logClass", "system");
    await fill("path", "/var/log/audit/*.log");
    await expect(modal.locator('[name="application-picker"]')).toHaveCount(0);
    await next();
    await action("wizard-test").click();
    await next();
    await action("wizard-create").click();
    await expect(page.locator("main")).toContainText("系统文件日志");
  });
  await check(
    "Kubernetes component toggles and unavailable endpoints",
    async () => {
      await navigate("kubernetes");
      await action("k8s-tab", "monitoring").click();
      await expect(action("component-toggle", "etcd")).toBeDisabled();
      await action("component-toggle", "scheduler").click();
      await action("confirm").click();
      await expect(action("component-toggle", "scheduler")).toHaveAttribute(
        "aria-checked",
        "true",
      );
      await page.screenshot({
        path: path.join(out, "04-kubernetes-components.png"),
        fullPage: true,
      });
    },
  );
  await check(
    "scale, YAML update, rollback and terminal are simulated",
    async () => {
      await action("k8s-tab", "resources").click();
      await action("k8s-kind", "Deployment").click();
      const row = page.locator("tbody tr").filter({
        has: page.getByRole("button", { name: "order-api", exact: true }),
      });
      await row.locator("summary").click();
      await action("k8s-scale", "w1").click();
      await fill("replicas", "4");
      await action("scale-preview").click();
      await action("confirm").click();
      await expect(row).toContainText("4 / 4");
      await row.locator("summary").click();
      await action("k8s-yaml", "w1").click();
      const text = await modal.locator('[name="yaml"]').inputValue();
      await fill("yaml", text.replace("order-api:1.8.2", "order-api:1.9.0"));
      await action("yaml-preview").click();
      await action("confirm").click();
      await expect(row).toContainText("order-api:1.9.0");
      await row.locator("summary").click();
      await action("k8s-rollback", "w1").click();
      await action("confirm").click();
      await expect(row).toContainText("order-api:1.8.2");
      await page.screenshot({
        path: path.join(out, "05-kubernetes-workloads.png"),
        fullPage: true,
      });
      await action("k8s-kind", "Pod").click();
      await page.locator("tbody tr").first().locator("summary").click();
      await action("pod-terminal", "w1:0").click();
      await page.locator('#terminal-form [name="command"]').fill("pwd");
      await page.locator('#terminal-form [type="submit"]').click();
      await expect(page.locator("#terminal-output")).toContainText("/app");
      await page.locator('#drawer [data-action="close"]').click();
    },
  );
  await check("namespace and workload creation", async () => {
    await action("k8s-kind", "Namespace").click();
    await action("k8s-create").click();
    await fill("name", "demo-acceptance");
    await action("k8s-create-preview").click();
    await action("confirm").click();
    await expect(page.locator("main")).toContainText("demo-acceptance");
    await action("k8s-kind", "Deployment").click();
    await action("k8s-create").click();
    await fill("name", "demo-api");
    await select("namespace", "demo-acceptance");
    await action("k8s-create-preview").click();
    await action("confirm").click();
    await expect(page.locator("main")).toContainText("demo-api");
  });
  await reset();
  await check(
    "dashboard dependency toggle, explicit empty scope and type intersection",
    async () => {
      await navigate("dashboards");
      await expect(page.locator("#scope-count")).toContainText("7");
      await page.locator('[name="dependencies"]').uncheck();
      await expect(page.locator("#scope-count")).toContainText("2");
      await expect(page.locator('[data-panel="database"]')).toContainText(
        "无匹配资源",
      );
      await action("select-scope").click();
      await modal.locator("#scope-all").uncheck();
      await action("scope-apply").click();
      await expect(page.locator("#scope-count")).toContainText("0");
      await expect(page.locator("main")).toContainText("条件交集为空");
      await action("dashboard-template", "application").click();
      await page.locator('[name="kind"]').selectOption("host");
      await expect(page.locator("#scope-count")).toContainText("0");
      await action("select-dashboard-app").click();
      await action("picker-all").click();
      await action("picker-apply").click();
      await expect(page.locator("#scope-count")).toContainText("3");
      await action("dashboard-template", "application").click();
      await page.screenshot({
        path: path.join(out, "06-dashboard-filters.png"),
        fullPage: true,
      });
    },
  );
  await check(
    "read-only role does not gain shared database or infrastructure access",
    async () => {
      await action("role").click();
      await expect(page.locator("#scope-count")).toContainText("2");
      await expect(page.locator('[data-panel="database"]')).toContainText(
        "无匹配资源",
      );
      await navigate("hosts");
      await expect(page.locator("main")).toContainText("未获得主机访问授权");
      await navigate("monitoring");
      await expect(action("new-monitor")).toBeDisabled();
      await action("role").click();
    },
  );
  await check(
    "trace to log correlation retains the exact trace identifier",
    async () => {
      await navigate("traces");
      await action("trace-detail", "trace-demo-order-4821").click();
      await action("trace-logs").click();
      await expect(page.locator('[name="log-query"]')).toHaveValue(
        "trace-demo-order-4821",
      );
      await expect(page.locator("main tbody tr")).toHaveCount(2);
    },
  );
  await check(
    "all 39 integration forms render their distinct input contracts",
    async () => {
      const catalog = await page.evaluate(() =>
        ArgusDemo.catalog.map((c) => ({
          id: c.id,
          category: c.category,
          mode: c.mode,
        })),
      );
      for (const c of catalog) {
        await navigate("monitoring");
        await action("new-monitor").click();
        await select("wizard-category", c.category);
        await action("wizard-type", c.id).click();
        await next();
        await expect(page.locator('#monitor-form [name="name"]')).toBeVisible();
        if (c.mode === "file")
          await expect(modal.locator('[name="path"]')).toBeVisible();
        if (c.mode === "k8slog")
          await expect(modal.locator('[name="workload"]')).toBeVisible();
        if (c.mode === "push")
          await expect(modal.locator('[name="service-picker"]')).toBeVisible();
        if (c.id === "oracle")
          await expect(modal.locator('[name="oracleService"]')).toBeVisible();
        if (c.id === "sqlserver")
          await expect(modal.locator('[name="sqlInstance"]')).toBeVisible();
        await modal.locator('[data-action="close"]').first().click();
      }
      checks.push(`catalog forms: ${catalog.length}`);
    },
  );
  await check("Grafana references cover every integration", async () => {
    await navigate("references");
    const missing = await page.evaluate(() =>
      ArgusDemo.catalog
        .filter(
          (c) => !ArgusDemo.references.some((r) => r.types.includes(c.id)),
        )
        .map((c) => c.id),
    );
    expect(missing).toEqual([]);
    await expect(page.locator("main tbody a")).toHaveCount(42);
    await page.screenshot({
      path: path.join(out, "07-grafana-references.png"),
      fullPage: true,
    });
  });
  for (const locale of ["zh", "en"])
    for (const theme of ["light", "dark"])
      for (const width of [1280, 1440, 1920]) {
        await check(`desktop layout ${locale} ${theme} ${width}`, async () => {
          await page.setViewportSize({ width, height: 960 });
          await page.evaluate(
            ({ locale, theme }) => {
              ArgusDemo.state.locale = locale;
              ArgusDemo.state.theme = theme;
              ArgusDemo.save();
              ArgusDemo.render();
            },
            { locale, theme },
          );
          for (const route of [
            "monitoring",
            "applications",
            "kubernetes",
            "dashboards",
            "references",
          ]) {
            await navigate(route);
            expect(
              await page.evaluate(
                () => document.documentElement.scrollWidth <= innerWidth + 1,
              ),
            ).toBe(true);
          }
        });
      }
  await page.setViewportSize({ width: 1440, height: 1000 });
  await check(
    "accessibility of monitoring, dashboard and create dialog",
    async () => {
      await page.evaluate(() => {
        ArgusDemo.state.locale = "zh";
        ArgusDemo.state.theme = "light";
        ArgusDemo.render();
      });
      for (const route of ["monitoring", "dashboards", "kubernetes"]) {
        await navigate(route);
        const results = await new AxeBuilder({ page }).analyze();
        const serious = results.violations.filter((x) =>
          ["critical", "serious"].includes(x.impact),
        );
        fs.writeFileSync(
          path.join(out, `axe-${route}.json`),
          JSON.stringify(results, null, 2),
        );
        expect(serious.map((x) => x.id)).toEqual([]);
      }
      await navigate("monitoring");
      await action("new-monitor").click();
      await next();
      const results = await new AxeBuilder({ page }).analyze();
      fs.writeFileSync(
        path.join(out, "axe-dialog.json"),
        JSON.stringify(results, null, 2),
      );
      expect(
        results.violations
          .filter((x) => ["critical", "serious"].includes(x.impact))
          .map((x) => x.id),
      ).toEqual([]);
    },
  );
  await modal.locator('[data-action="close"]').first().click();
  await check(
    "Kubernetes object filters remain open and resolve child scope",
    async () => {
      await navigate("dashboards");
      await action("dashboard-template", "kubernetes").click();
      await page.getByText("Kubernetes 对象筛选", { exact: true }).click();
      await page.locator('main [name="namespace"]').selectOption("commerce");
      await expect(page.locator("main [data-filter-details]")).toHaveAttribute(
        "open",
        "",
      );
      await page.locator('main [name="workload"]').selectOption("order-api");
      await expect(page.locator("#scope-count")).toContainText("1");
      await expect(
        page.locator('[data-panel="kubernetes"] .argus-chart'),
      ).toBeVisible();
    },
  );
  await check(
    "component dashboard has an explicit unavailable state",
    async () => {
      await navigate("kubernetes");
      await action("k8s-tab", "monitoring").click();
      await action("component-dashboard", "etcd").click();
      await expect(
        page.getByText("组件端点不可访问", { exact: true }),
      ).toHaveCount(2);
      await expect(page.locator("main .argus-chart")).toHaveCount(0);
    },
  );
  await check("DaemonSet YAML does not declare replicas", async () => {
    await navigate("kubernetes");
    await action("k8s-tab", "resources").click();
    await action("k8s-kind", "DaemonSet").click();
    await page.locator("tbody summary").first().click();
    await action("k8s-yaml", "w5").click();
    const yaml = await modal.locator('[name="yaml"]').inputValue();
    expect(yaml).not.toContain("replicas:");
    await fill(
      "yaml",
      yaml.replace("argus-otelcol:demo", "argus-otelcol:demo2"),
    );
    await action("yaml-preview").click();
    await action("confirm").click();
    await expect(page.locator("main")).toContainText("argus-otelcol:demo2");
  });
  await check("new application has no inherited sample telemetry", async () => {
    await navigate("applications");
    await action("app-list").click();
    await action("new-app").click();
    await fill("name", "空应用验收");
    await fill("owner", "平台组");
    await action("app-create").click();
    await action("app-tab", "overview").click();
    await expect(page.locator("main")).toContainText("尚无业务遥测");
    await expect(page.locator("main")).not.toContainText("mysql-orders");
    await action("new-service").click();
    await fill("name", "fulfilment-api");
    await action("service-create").click();
    await action("app-tab", "services").click();
    await action("service-access").click();
    await fill("name", "新服务 Trace");
    await next();
    await action("wizard-test").click();
    await next();
    await action("wizard-create").click();
    await expect(page.locator("#drawer")).toContainText(
      "OTEL_SERVICE_NAME=fulfilment-api",
    );
    await page.locator('#drawer [data-action="close"]').click();
  });
  await check("direct file preview and dark English catalog", async () => {
    const localPage = await browser.newPage({
      viewport: { width: 1440, height: 1000 },
    });
    localPage.on("pageerror", (err) => errors.push(err.message));
    const fileUrl = require("node:url").pathToFileURL(
      path.join(__dirname, "demo.html"),
    ).href;
    await localPage.goto(fileUrl);
    await expect(localPage.locator("main h1")).toHaveText("监控");
    await localPage
      .locator('[data-action="navigate"][data-id="applications"]')
      .click();
    await localPage.screenshot({
      path: path.join(out, "09-application.png"),
      fullPage: true,
    });
    await localPage.locator('[data-action="locale"]').click();
    await localPage.locator('[data-action="theme"]').click();
    await localPage
      .locator('[data-action="navigate"][data-id="monitoring"]')
      .click();
    await localPage.locator('[data-action="new-monitor"]').click();
    await localPage.screenshot({
      path: path.join(out, "08-dark-new-monitor.png"),
    });
    await localPage.close();
  });
  expect(errors).toEqual([]);
  fs.writeFileSync(
    path.join(out, "result.json"),
    JSON.stringify(
      {
        status: "passed",
        scope: "local_discussion_prototype",
        checks,
        errors,
        productionApiCalled: false,
        clusterMutated: false,
      },
      null,
      2,
    ),
  );
  await browser.close();
  console.log(`PASS ${checks.length} checks; evidence: ${out}`);
}
run().catch((err) => {
  fs.writeFileSync(
    path.join(out, "result.json"),
    JSON.stringify(
      { status: "failed", checks, errors, error: String(err) },
      null,
      2,
    ),
  );
  console.error(err);
  process.exit(1);
});
