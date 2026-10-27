const { chromium, expect } = require("@playwright/test");
const AxeBuilder = require("@axe-core/playwright").default;
const fs = require("node:fs"),
  path = require("node:path");
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
(async () => {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 1000 },
  });
  const page = await context.newPage();
  page.on("pageerror", (err) => errors.push(err.message));
  const a = (name, id) =>
    page
      .locator(`[data-action="${name}"]${id ? `[data-id="${id}"]` : ""}`)
      .first();
  const modal = page.locator("#modal"),
    picker = page.locator("#picker");
  const fill = (name, value) => modal.locator(`[name="${name}"]`).fill(value);
  const next = () => a("wizard-next").click();
  const validate = async () => {
    await next();
    await a("wizard-test").click();
    await next();
  };
  const choose = async (kind, ids) => {
    await a(
      kind === "apps" ? "select-wizard-apps" : "select-wizard-services",
    ).click();
    for (const id of ids) {
      const row = picker.locator(
        `[data-action="picker-toggle"][data-id="${id}"]`,
      );
      if ((await row.getAttribute("aria-pressed")) !== "true")
        await row.click();
    }
    await a("picker-apply").click();
  };
  await page.goto(
    "http://127.0.0.1:8766/docs/plans/monitoring-workspace/demo.html",
  );
  await a("reset").click();
  await check(
    "no environment, object or credential-reference selectors in creation",
    async () => {
      await a("new-monitor").click();
      await next();
      await expect(
        modal.locator(
          '[name="env"],[name="existing"],[name="secret"],select[name="app"],select[name="service"],input[name="apps"]',
        ),
      ).toHaveCount(0);
      await fill("name", "选择器验收");
      await fill("endpoint", "db-selector.example.test:3306");
      await fill("authUsername", "DEMO_PRIVATE_USERNAME_SENTINEL");
      await fill("authPassword", "DEMO_PRIVATE_PASSWORD_SENTINEL");
      await expect(modal.locator('[name="authPassword"]')).toHaveAttribute(
        "type",
        "password",
      );
      await expect(modal.locator(".argus-derived-environment")).toContainText(
        "未关联环境",
      );
    },
  );
  await check("search, pagination and cross-page selection", async () => {
    await a("select-wizard-apps").click();
    await expect(picker.locator("tbody tr")).toHaveCount(8);
    await a("picker-toggle", "order").click();
    await a("picker-next").click();
    await a("picker-toggle", "fulfilment").click();
    await picker.locator('[name="picker-query"]').fill("Analytics");
    await expect(picker.locator("tbody tr")).toHaveCount(1);
    await a("picker-toggle", "analytics").click();
    await expect(picker.locator("#picker-selection-count")).toContainText("3");
    await picker.locator('[name="picker-query"]').fill("");
    await expect(a("picker-prev")).toBeDisabled();
    await page.screenshot({
      path: path.join(out, "10-application-picker.png"),
    });
    const axe = await new AxeBuilder({ page }).analyze();
    expect(
      axe.violations
        .filter((v) => ["critical", "serious"].includes(v.impact))
        .map((v) => v.id),
    ).toEqual([]);
    await a("picker-apply").click();
    await expect(modal.locator('[name="application-picker"]')).toContainText(
      "3",
    );
    await expect(modal.locator(".argus-derived-environment")).toContainText(
      "生产 / 测试",
    );
    await expect(modal.locator('[name="authPassword"]')).toHaveValue(
      "DEMO_PRIVATE_PASSWORD_SENTINEL",
    );
  });
  await check(
    "cancel and Escape retain parent draft and committed selection",
    async () => {
      const before = await modal
        .locator('[name="application-picker"]')
        .innerText();
      await a("select-wizard-apps").click();
      await a("picker-clear").click();
      await a("picker-cancel").last().click();
      await expect(modal.locator('[name="application-picker"]')).toHaveText(
        before,
      );
      await expect(modal.locator('[name="name"]')).toHaveValue("选择器验收");
      await a("select-wizard-services").click();
      await expect(picker).not.toContainText("billing-api");
      await page.keyboard.press("Escape");
      await expect(picker).not.toBeVisible();
      await expect(modal).toBeVisible();
      await expect(modal.locator('[name="authPassword"]')).toHaveValue(
        "DEMO_PRIVATE_PASSWORD_SENTINEL",
      );
    },
  );
  let created;
  await check(
    "automatic managed credential excludes entered values from storage and preview",
    async () => {
      await page.screenshot({
        path: path.join(out, "11-inline-authentication.png"),
      });
      await validate();
      await expect(modal).not.toContainText("DEMO_PRIVATE_PASSWORD_SENTINEL");
      await expect(modal).not.toContainText("DEMO_PRIVATE_USERNAME_SENTINEL");
      await a("wizard-create").click();
      const saved = await page.evaluate(() => ({
        state: JSON.stringify(ArgusDemo.state),
        storage: localStorage.getItem(ArgusDemo.storageKey),
        monitor: ArgusDemo.state.monitors.find((m) => m.name === "选择器验收"),
        credentials: ArgusDemo.state.monitorCredentials,
        secrets: ArgusDemo.wizardSecrets,
      }));
      expect(saved.state + saved.storage).not.toContain(
        "DEMO_PRIVATE_PASSWORD_SENTINEL",
      );
      expect(saved.state + saved.storage).not.toContain(
        "DEMO_PRIVATE_USERNAME_SENTINEL",
      );
      created = saved.monitor;
      expect(created.managedCredentialId).toBeTruthy();
      expect(
        saved.credentials.find((c) => c.id === created.managedCredentialId),
      ).toMatchObject({
        purpose: "monitoring",
        ownerType: "collection",
        ownerId: created.id,
        configured: true,
      });
      expect(saved.monitor.env).toBeNull();
      await expect
        .poll(() => page.evaluate(() => ArgusDemo.wizardSecrets))
        .toBeNull();
    },
  );
  await check("editing keeps opaque credential with blank inputs", async () => {
    const row = page.locator("main tbody tr").filter({ hasText: "选择器验收" });
    await row.locator("summary").click();
    await a("monitor-edit", created.id).click();
    await expect(modal.locator('[name="authPassword"]')).toHaveValue("");
    await expect(modal).toContainText("留空沿用");
    await validate();
    await a("wizard-create").click();
    const record = await page.evaluate(
      (id) => ArgusDemo.state.monitorCredentials.find((c) => c.ownerId === id),
      created.id,
    );
    expect(record.id).toBe(created.managedCredentialId);
    expect(record.version).toBe(1);
  });
  await check(
    "resource-context creation reuses target without object dropdown",
    async () => {
      await a("navigate", "resources").click();
      const count = await page.evaluate(() => ArgusDemo.state.resources.length);
      await a("resource-detail", "db-order").click();
      await a("resource-monitor", "db-order").click();
      await expect(modal).toContainText("mysql-orders");
      await expect(modal.locator('[name="existing"]')).toHaveCount(0);
      await fill("name", "已有对象的额外采集");
      await fill("authUsername", "demo");
      await fill("authPassword", "DEMO_SECRET_ONLY");
      await validate();
      await a("wizard-create").click();
      const state = await page.evaluate(() => ({
        count: ArgusDemo.state.resources.length,
        target: ArgusDemo.state.monitors.find(
          (m) => m.name === "已有对象的额外采集",
        ).target,
      }));
      expect(state.count).toBe(count);
      expect(state.target).toBe("db-order");
    },
  );
  await check(
    "deleting a collection releases its own managed credential",
    async () => {
      await a("navigate", "monitoring").click();
      await a("category", "all").click();
      const row = page
        .locator("main tbody tr")
        .filter({ hasText: "选择器验收" });
      await row.locator("summary").click();
      await a("monitor-delete", created.id).click();
      await a("confirm").click();
      const remaining = await page.evaluate(
        (id) =>
          ArgusDemo.state.monitorCredentials.filter((c) => c.ownerId === id),
        created.id,
      );
      expect(remaining).toEqual([]);
    },
  );
  await check(
    "Trace environment inherits selected service including staging",
    async () => {
      await a("navigate", "traces").click();
      await a("new-trace").click();
      await next();
      await fill("name", "测试环境自动继承");
      await choose("apps", ["order"]);
      await choose("services", ["order-api-test"]);
      await expect(modal.locator(".argus-derived-environment")).toContainText(
        "测试",
      );
      await expect(modal.locator('[name="env"]')).toHaveCount(0);
      await page.screenshot({
        path: path.join(out, "12-service-environment.png"),
      });
      await validate();
      await a("wizard-create").click();
      await expect(page.locator("#drawer")).toContainText(
        "deployment.environment.name=staging",
      );
      await page.locator('#drawer [data-action="close"]').click();
    },
  );
  await check(
    "unlinked target is unclassified and global filters use inherited environments",
    async () => {
      await a("navigate", "monitoring").click();
      await a("category", "database").click();
      await a("new-monitor").click();
      await next();
      await fill("name", "未关联监控");
      await fill("endpoint", "unlinked.example.test:3306");
      await modal.locator('[name="authMode"]').selectOption("none");
      await validate();
      await a("wizard-create").click();
      const record = await page.evaluate(() => {
        const m = ArgusDemo.state.monitors.find((m) => m.name === "未关联监控");
        return {
          monitor: m,
          envs: ArgusDemo.environmentIds(ArgusDemo.resource(m.target)),
        };
      });
      expect(record.monitor.apps).toEqual([]);
      expect(record.monitor.env).toBeNull();
      expect(record.envs).toEqual([]);
      expect(record.monitor.managedCredentialId).toBeNull();
      await a("navigate", "dashboards").click();
      await a("dashboard-template", "database").click();
      await page.locator('main [name="kind"]').selectOption("database");
      await page.locator('main [name="env"]').selectOption("staging");
      await expect(page.locator("#scope-count")).toContainText("2");
      await page.locator('main [name="env"]').selectOption("all");
      await a("scope-explain").click();
      await expect(page.locator("#drawer")).toContainText("应用");
      await page.locator('#drawer [data-action="close"]').click();
    },
  );
  await check(
    "new pickers remain usable in both languages and themes",
    async () => {
      for (const locale of ["zh", "en"])
        for (const theme of ["light", "dark"]) {
          await page.evaluate(
            ({ locale, theme }) => {
              ArgusDemo.state.locale = locale;
              ArgusDemo.state.theme = theme;
              ArgusDemo.render();
            },
            { locale, theme },
          );
          await a("select-dashboard-app").click();
          await expect(picker).toBeVisible();
          expect(
            await page.evaluate(
              () => document.documentElement.scrollWidth <= innerWidth + 1,
            ),
          ).toBe(true);
          const axe = await new AxeBuilder({ page }).analyze();
          expect(
            axe.violations
              .filter((v) => ["critical", "serious"].includes(v.impact))
              .map((v) => v.id),
          ).toEqual([]);
          await a("picker-cancel").last().click();
        }
    },
  );
  expect(errors).toEqual([]);
  fs.writeFileSync(
    path.join(out, "selections-result.json"),
    JSON.stringify(
      {
        status: "passed",
        checks,
        errors,
        credentialsStored: false,
        scope: "local_prototype",
      },
      null,
      2,
    ),
  );
  await browser.close();
  console.log(`PASS ${checks.length} selection and credential checks`);
})().catch((err) => {
  fs.writeFileSync(
    path.join(out, "selections-result.json"),
    JSON.stringify(
      { status: "failed", checks, errors, error: String(err) },
      null,
      2,
    ),
  );
  console.error(err);
  process.exit(1);
});
