import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`Plan8 application performance ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      const text = (zh: string, en: string) => (english ? en : zh);
      const errors: string[] = [];
      page.on("pageerror", (error) => errors.push(error.message));
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
      await page.goto("/applications");
      await page
        .getByRole("button", {
          name: text("新建应用", "Create application"),
          exact: true,
        })
        .click();
      const dialog = page.getByRole("dialog", {
        name: text("新建应用", "Create application"),
        exact: true,
      });
      await dialog
        .getByRole("textbox", {
          name: new RegExp(text("应用名称", "Application name")),
        })
        .fill("APM application");
      await dialog
        .getByRole("button", {
          name: text("预览变更", "Preview changes"),
          exact: true,
        })
        .click();
      const confirm = async () =>
        page
          .getByRole("button", {
            name: text("确认变更", "Confirm changes"),
            exact: true,
          })
          .click();
      await confirm();
      await page
        .getByRole("link", { name: "APM application", exact: true })
        .click();
      await page
        .getByRole("button", {
          name: text("声明服务", "Declare service"),
          exact: true,
        })
        .click();
      const declaration = page.getByRole("dialog", {
        name: text("声明服务", "Declare service"),
        exact: true,
      });
      await declaration
        .getByRole("textbox", {
          name: english ? /^Service name\b/ : /服务名称/,
        })
        .fill("orders-api");
      await declaration
        .getByRole("button", {
          name: text("预览变更", "Preview changes"),
          exact: true,
        })
        .click();
      await confirm();
      const region = page.getByRole("region", {
        name: text("应用性能", "Application performance"),
        exact: true,
      });
      await expect(region).toBeVisible();
      await expect(page.getByRole("dialog")).toHaveCount(0);
      await region
        .getByRole("radiogroup")
        .getByText(text("服务", "Services"), { exact: true })
        .click();
      await page.keyboard.press("ArrowRight");
      await expect(
        region.getByRole("radio", {
          name: text("运行实例", "Runtime instances"),
          exact: true,
        }),
      ).toBeChecked();
      for (const label of [
        text("服务", "Services"),
        text("运行实例", "Runtime instances"),
        text("接口", "Endpoints"),
        text("请求与耗时", "Requests and duration"),
        text("服务拓扑", "Service topology"),
        text("调用链", "Traces"),
        text("应用日志", "Application logs"),
      ]) {
        await region
          .getByRole("radiogroup")
          .getByText(label, { exact: true })
          .click();
        await expect(
          region.getByRole("radio", { name: label, exact: true }),
        ).toBeChecked();
        await expect(region.getByRole("alert")).toHaveCount(0);
      }
      await region
        .getByRole("button", {
          name: text("选择服务（当前全部）", "Choose service (currently all)"),
          exact: true,
        })
        .click();
      const picker = page.getByRole("dialog", {
        name: text("选择服务", "Choose a service"),
        exact: true,
      });
      await picker
        .getByRole("textbox", { name: text("搜索列表", "Search list") })
        .fill("orders-api");
      await picker
        .getByRole("button", { name: /^(选择|Select) orders-api/ })
        .click();
      await expect(
        region.getByRole("button", { name: "orders-api", exact: true }),
      ).toBeVisible();
      await region
        .getByRole("button", {
          name: text("清除服务筛选", "Clear service filter"),
          exact: true,
        })
        .click();
      await region
        .getByRole("button", {
          name: text("刷新数据", "Refresh data"),
          exact: true,
        })
        .click();
      expect(
        (await new AxeBuilder({ page }).analyze()).violations.filter((item) =>
          ["critical", "serious"].includes(item.impact ?? ""),
        ),
      ).toEqual([]);
      await page.screenshot({
        path: info.outputPath("application-performance.png"),
        fullPage: true,
      });
      expect(errors).toEqual([]);
    });
  }
