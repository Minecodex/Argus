import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`Plan8 monitoring creation ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      const errors: string[] = [];
      page.on("pageerror", (error) => errors.push(error.message));
      await page.setViewportSize({ width: 1440, height: 1000 });
      await page.addInitScript(
        ({ english, theme }) => {
          localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
          localStorage.setItem("argus.theme", theme);
        },
        { english, theme },
      );
      await page.goto("/login");
      await page.locator('input[autocomplete="username"]').fill("root");
      await page
        .locator('input[autocomplete="current-password"]')
        .fill("123456");
      await page.locator('form button[type="submit"]').click();
      await expect(page).not.toHaveURL(/\/login/);
      await page.goto("/applications");
      await page
        .getByRole("button", {
          name: english ? "Create application" : "新建应用",
          exact: true,
        })
        .click();
      const appDialog = page.getByRole("dialog", {
        name: english ? "Create application" : "新建应用",
        exact: true,
      });
      await appDialog
        .getByRole("textbox", {
          name: new RegExp(english ? "Application name" : "应用名称"),
        })
        .fill("Orders monitor test");
      await appDialog
        .getByRole("button", {
          name: english ? "Preview changes" : "预览变更",
          exact: true,
        })
        .click();
      await page
        .getByRole("button", {
          name: english ? "Confirm changes" : "确认变更",
          exact: true,
        })
        .click();
      await expect(
        page.getByRole("link", { name: "Orders monitor test", exact: true }),
      ).toBeVisible();
      await page.goto("/monitoring");
      await page
        .getByRole("button", {
          name: english ? "New monitor" : "新建监控",
          exact: true,
        })
        .click();
      const types = page.getByRole("dialog", {
        name: english ? "Choose a monitoring type" : "选择监控类型",
      });
      await expect(
        types
          .getByRole("row")
          .filter({ has: page.getByText("ClickHouse", { exact: true }) })
          .getByRole("button"),
      ).toBeDisabled();
      await types
        .getByRole("row")
        .filter({ has: page.getByText("MySQL", { exact: true }) })
        .getByRole("button")
        .click();
      const form = page.getByRole("dialog", {
        name: english ? "New MySQL monitor" : "新建 MySQL 监控",
        exact: true,
      });
      await expect(form.getByRole("combobox")).toHaveCount(0);
      await form
        .getByRole("textbox", {
          name: new RegExp(english ? "Monitor name" : "监控名称"),
        })
        .fill("Orders MySQL");
      await form
        .getByRole("textbox", {
          name: new RegExp(english ? "Service endpoint" : "服务地址"),
        })
        .fill("mysql.example:3306");
      await form
        .getByRole("textbox", {
          name: new RegExp(english ? "Username" : "用户名"),
        })
        .fill("monitor");
      const privateValue = "plan8-private-password-never-persist";
      await form.locator('input[type="password"]').fill(privateValue);
      await form
        .getByRole("button", {
          name: english ? "Choose a Collector" : "选择执行收集器",
          exact: true,
        })
        .click();
      const collectors = page.getByRole("dialog", {
        name: english ? "Choose a Collector" : "选择执行收集器",
        exact: true,
      });
      await collectors
        .getByRole("button", { name: /^(选择|Select) / })
        .first()
        .click();
      await form
        .getByRole("button", {
          name: english ? "Choose applications" : "选择应用",
          exact: true,
        })
        .click();
      const applications = page.getByRole("dialog", {
        name: english ? "Choose applications" : "选择应用",
        exact: true,
      });
      await applications
        .getByRole("textbox", { name: english ? "Search list" : "搜索列表" })
        .fill("Orders monitor test");
      await applications
        .getByRole("button", { name: /^(选择|Select) Orders monitor test/ })
        .click();
      await applications
        .getByRole("button", { name: /^(确认选择|Confirm selection)/ })
        .click();
      await expect(
        form.getByText("Orders monitor test", { exact: true }),
      ).toBeVisible();
      await expect(
        form.getByRole("textbox", {
          name: english ? "Environment" : "环境",
          exact: true,
        }),
      ).toHaveCount(0);
      const a11y = await new AxeBuilder({ page }).analyze();
      expect(
        a11y.violations.filter(
          (v) => v.impact === "critical" || v.impact === "serious",
        ),
      ).toEqual([]);
      await page.screenshot({
        path: info.outputPath("monitoring-create.png"),
        fullPage: true,
      });
      await form
        .getByRole("button", {
          name: english ? "Preview configuration" : "预览采集配置",
          exact: true,
        })
        .click();
      expect(
        await page.evaluate(() => JSON.stringify(localStorage)),
      ).not.toContain(privateValue);
      await page
        .getByRole("button", {
          name: english ? "Confirm changes" : "确认变更",
          exact: true,
        })
        .click();
      const row = page
        .getByRole("row")
        .filter({ has: page.getByText("Orders MySQL", { exact: true }) });
      await expect(row).toBeVisible();
      await expect(
        row.getByText(english ? "Waiting for samples" : "等待样本", {
          exact: true,
        }),
      ).toBeVisible();
      await row.getByRole("switch").focus();
      await row.getByRole("switch").press("Space");
      await page
        .getByRole("button", {
          name: english ? "Confirm changes" : "确认变更",
          exact: true,
        })
        .click();
      await expect(row.getByRole("switch")).not.toBeChecked();
      expect(
        await page.evaluate(() => JSON.stringify(localStorage)),
      ).not.toContain(privateValue);
      await page.screenshot({
        path: info.outputPath("monitoring-list.png"),
        fullPage: true,
      });
      await row
        .getByRole("button", {
          name: english ? "Default dashboard" : "默认仪表盘",
          exact: true,
        })
        .click();
      await expect(page).toHaveURL(/\/dashboards\/[^?]+\?resource=/);
      await expect(
        page.getByRole("button", {
          name: english
            ? "Choose resources: Selected resources: 1"
            : "选择资源: 已选 1 个资源",
          exact: true,
        }),
      ).toBeVisible();
      await page.goto("/dashboards");
      const managed = page.getByRole("row").filter({
        has: page.getByRole("link", { name: "MySQL", exact: true }),
      });
      await expect(managed).toContainText(
        english ? "System default" : "系统默认",
      );
      await managed.getByRole("link", { name: "MySQL", exact: true }).click();
      await expect(
        page.getByRole("button", {
          name: english ? "Create personal copy" : "创建个人副本",
          exact: true,
        }),
      ).toBeVisible();
      await expect(
        page
          .getByText(english ? "Uptime" : "运行时长", { exact: true })
          .first(),
      ).toBeVisible();
      const firstPanel = page.locator('[data-panel-id="uptime"]');
      await expect(firstPanel).toBeVisible();
      expect((await firstPanel.boundingBox())?.height).toBeGreaterThan(240);
      await page.screenshot({
        path: info.outputPath("managed-native-default.png"),
        fullPage: true,
      });
      await page
        .getByRole("button", {
          name: english ? "Create personal copy" : "创建个人副本",
          exact: true,
        })
        .click();
      await expect(page).toHaveURL(/\/dashboard-drafts\//);
      expect(errors).toEqual([]);
    });
  }
