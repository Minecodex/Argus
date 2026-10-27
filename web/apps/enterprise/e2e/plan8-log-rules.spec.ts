import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`Plan8 file log rules ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      const label = (zh: string, en: string) => (english ? en : zh);
      const errors: string[] = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.setViewportSize({ width: 1440, height: 1050 });
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
      const confirm = async () =>
        page
          .getByRole("button", {
            name: label("确认变更", "Confirm changes"),
            exact: true,
          })
          .click();
      await page.goto("/applications");
      await page
        .getByRole("button", {
          name: label("新建应用", "Create application"),
          exact: true,
        })
        .click();
      const appDialog = page.getByRole("dialog", {
        name: label("新建应用", "Create application"),
        exact: true,
      });
      await appDialog
        .getByRole("textbox", {
          name: new RegExp(label("应用名称", "Application name")),
        })
        .fill("Log portal application");
      await appDialog
        .getByRole("button", {
          name: label("预览变更", "Preview changes"),
          exact: true,
        })
        .click();
      await confirm();
      await page
        .getByRole("link", { name: "Log portal application", exact: true })
        .click();
      await page
        .getByRole("button", {
          name: label("声明服务", "Declare service"),
          exact: true,
        })
        .click();
      const serviceDialog = page.getByRole("dialog", {
        name: label("声明服务", "Declare service"),
        exact: true,
      });
      await serviceDialog
        .getByRole("textbox", {
          name: english ? /^Service name\b/ : /服务名称/,
        })
        .fill("orders-log");
      await serviceDialog
        .getByRole("button", {
          name: label("预览变更", "Preview changes"),
          exact: true,
        })
        .click();
      await confirm();
      await expect(
        page
          .getByRole("row")
          .filter({ has: page.getByText("orders-log", { exact: true }) }),
      ).toBeVisible();
      await page.goto("/monitoring");
      const open = async (integration = "File logs") => {
        await page
          .getByRole("button", {
            name: label("新建监控", "New monitor"),
            exact: true,
          })
          .click();
        const picker = page.getByRole("dialog", {
          name: label("选择监控类型", "Choose a monitoring type"),
        });
        await picker
          .getByRole("tab", { name: label("日志", "Logs"), exact: true })
          .click();
        await picker
          .getByRole("row")
          .filter({ has: page.getByText(integration, { exact: true }) })
          .getByRole("button")
          .click();
        return page.getByRole("dialog", {
          name: label(`新建 ${integration} 监控`, `New ${integration} monitor`),
          exact: true,
        });
      };
      let form = await open();
      await form
        .getByRole("textbox", {
          name: new RegExp(label("监控名称", "Monitor name")),
        })
        .fill("Business file rule");
      await form
        .getByRole("button", {
          name: label("选择应用", "Choose applications"),
          exact: true,
        })
        .click();
      const apps = page.getByRole("dialog", {
        name: label("选择应用", "Choose applications"),
        exact: true,
      });
      await apps
        .getByRole("textbox", { name: label("搜索列表", "Search list") })
        .fill("Log portal application");
      await apps
        .getByRole("button", { name: /^(选择|Select) Log portal application/ })
        .click();
      await expect(apps).not.toBeVisible();
      await form
        .getByRole("button", {
          name: label("选择服务", "Choose a service"),
          exact: true,
        })
        .click();
      const services = page.getByRole("dialog", {
        name: label("选择服务", "Choose a service"),
        exact: true,
      });
      await services
        .getByRole("textbox", { name: label("搜索列表", "Search list") })
        .fill("orders-log");
      await services
        .getByRole("button", { name: /^(选择|Select) orders-log/ })
        .click();
      const chooseCollector = async () => {
        await form
          .getByRole("button", {
            name: label("选择执行收集器", "Choose a Collector"),
            exact: true,
          })
          .click();
        const collectors = page.getByRole("dialog", {
          name: label("选择执行收集器", "Choose a Collector"),
          exact: true,
        });
        await collectors
          .getByRole("button", { name: /^(选择|Select) / })
          .filter({ visible: true })
          .first()
          .click();
      };
      await chooseCollector();
      await form
        .getByRole("textbox", {
          name: new RegExp(label("采集路径", "Include paths")),
        })
        .fill("/var/log/orders/**/*.log");
      await form
        .getByRole("textbox", {
          name: new RegExp(label("排除路径", "Exclude paths")),
        })
        .fill("/var/log/orders/private/**");
      await expect(form.locator('input[type="password"]')).toHaveCount(0);
      await expect(
        form.getByRole("button", { name: /^(环境|Environment)/ }),
      ).toHaveCount(0);
      expect(
        (await new AxeBuilder({ page }).analyze()).violations.filter((v) =>
          ["critical", "serious"].includes(v.impact ?? ""),
        ),
      ).toEqual([]);
      await page.screenshot({
        path: info.outputPath("log-rule.png"),
        fullPage: true,
      });
      await form
        .getByRole("button", {
          name: label("预览采集配置", "Preview configuration"),
          exact: true,
        })
        .click();
      await confirm();
      const row = page
        .getByRole("row")
        .filter({ has: page.getByText("Business file rule", { exact: true }) });
      await expect(row).toBeVisible();
      await expect(row).toContainText(label("等待样本", "Waiting for samples"));
      await row.getByRole("switch").focus();
      await row.getByRole("switch").press("Space");
      await confirm();
      await expect(row.getByRole("switch")).not.toBeChecked();
      await open();
      await form
        .getByRole("button", {
          name: new RegExp(label("日志归属", "Log ownership")),
        })
        .click();
      await page
        .getByRole("option", {
          name: label("系统日志", "System logs"),
          exact: true,
        })
        .click();
      await expect(
        form.getByRole("button", {
          name: label("选择应用", "Choose applications"),
          exact: true,
        }),
      ).toHaveCount(0);
      await expect(
        form.getByRole("button", {
          name: label("选择服务", "Choose a service"),
          exact: true,
        }),
      ).toHaveCount(0);
      await form.press("Escape");
      form = await open("Journald");
      await form
        .getByRole("textbox", {
          name: new RegExp(label("监控名称", "Monitor name")),
        })
        .fill("System journal rule");
      await form
        .getByRole("button", {
          name: new RegExp(label("日志归属", "Log ownership")),
        })
        .click();
      await page
        .getByRole("option", {
          name: label("系统日志", "System logs"),
          exact: true,
        })
        .click();
      await chooseCollector();
      await form
        .getByRole("textbox", { name: /Systemd/ })
        .fill("nginx.service\npostgresql@main.service");
      await expect(form).toContainText("journalctl");
      await form
        .getByRole("button", {
          name: label("预览采集配置", "Preview configuration"),
          exact: true,
        })
        .click();
      await confirm();
      await expect(
        page
          .getByRole("row")
          .filter({
            has: page.getByText("System journal rule", { exact: true }),
          }),
      ).toBeVisible();
      expect(errors).toEqual([]);
    });
  }
