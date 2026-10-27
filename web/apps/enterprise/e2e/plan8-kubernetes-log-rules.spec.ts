import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`Plan8 Kubernetes log rule ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      const text = (zh: string, en: string) => (english ? en : zh);
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
      await page.goto("/monitoring");
      await page
        .getByRole("button", {
          name: text("新建监控", "New monitor"),
          exact: true,
        })
        .click();
      const types = page.getByRole("dialog", {
        name: text("选择监控类型", "Choose a monitoring type"),
      });
      await types
        .getByRole("tab", { name: text("日志", "Logs"), exact: true })
        .click();
      await types
        .getByRole("row")
        .filter({ has: page.getByText("Kubernetes logs", { exact: true }) })
        .getByRole("button")
        .click();
      const form = page.getByRole("dialog", {
        name: text("新建 Kubernetes logs 监控", "New Kubernetes logs monitor"),
        exact: true,
      });
      await form
        .getByRole("textbox", {
          name: new RegExp(text("监控名称", "Monitor name")),
        })
        .fill("Workload log rule");
      await form
        .getByRole("button", {
          name: new RegExp(text("日志归属", "Log ownership")),
        })
        .click();
      await page
        .getByRole("option", {
          name: text("基础设施组件日志", "Infrastructure component logs"),
          exact: true,
        })
        .click();
      await form
        .getByRole("button", {
          name: text("选择执行收集器", "Choose a Collector"),
          exact: true,
        })
        .click();
      const collectors = page.getByRole("dialog", {
        name: text("选择执行收集器", "Choose a Collector"),
        exact: true,
      });
      await collectors
        .getByRole("button", { name: /^(选择|Select) /, disabled: false })
        .first()
        .click();
      await form
        .getByRole("button", {
          name: text("选择命名空间", "Choose a namespace"),
          exact: true,
        })
        .click();
      const namespaces = page.getByRole("dialog", {
        name: text("选择命名空间", "Choose a namespace"),
        exact: true,
      });
      await namespaces
        .getByRole("button", { name: /^(选择|Select) production$/ })
        .click();
      await form
        .getByRole("button", {
          name: text("选择工作负载", "Choose a workload"),
          exact: true,
        })
        .click();
      const workloads = page.getByRole("dialog", {
        name: text("选择工作负载", "Choose a workload"),
        exact: true,
      });
      await workloads
        .getByRole("button", { name: /^(选择|Select) deployment-example$/ })
        .click();
      await form
        .getByRole("button", {
          name: text("选择容器", "Choose containers"),
          exact: true,
        })
        .click();
      const containers = page.getByRole("dialog", {
        name: text("选择容器", "Choose containers"),
        exact: true,
      });
      await expect(
        containers.getByRole("button", {
          name: /^(确认选择|Confirm selection)/,
        }),
      ).toBeDisabled();
      await containers
        .getByRole("button", { name: /^(选择|Select) main$/ })
        .click();
      await containers
        .getByRole("button", { name: /^(确认选择|Confirm selection)/ })
        .click();
      await expect(
        form.getByRole("button", { name: "main", exact: true }),
      ).toBeVisible();
      expect(
        (await new AxeBuilder({ page }).analyze()).violations.filter((v) =>
          ["critical", "serious"].includes(v.impact ?? ""),
        ),
      ).toEqual([]);
      await page.screenshot({
        path: info.outputPath("kubernetes-log-rule.png"),
        fullPage: true,
      });
      await form
        .getByRole("button", {
          name: text("预览采集配置", "Preview configuration"),
          exact: true,
        })
        .click();
      await page
        .getByRole("button", {
          name: text("确认变更", "Confirm changes"),
          exact: true,
        })
        .click();
      const row = page
        .getByRole("row")
        .filter({ has: page.getByText("Workload log rule", { exact: true }) });
      await expect(row).toBeVisible();
      await expect(row).toContainText(text("等待样本", "Waiting for samples"));
      await row.getByRole("switch").focus();
      await row.getByRole("switch").press("Space");
      await page
        .getByRole("button", {
          name: text("确认变更", "Confirm changes"),
          exact: true,
        })
        .click();
      await expect(row.getByRole("switch")).not.toBeChecked();
      expect(errors).toEqual([]);
    });
  }
