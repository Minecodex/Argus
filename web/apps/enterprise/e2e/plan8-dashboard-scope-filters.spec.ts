import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`Plan8 dashboard scope filters ${english ? "en" : "zh"} ${theme}`, async ({
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
      await page.goto("/applications");
      await page
        .getByRole("button", {
          name: text("新建应用", "Create application"),
          exact: true,
        })
        .click();
      const app = page.getByRole("dialog", {
        name: text("新建应用", "Create application"),
        exact: true,
      });
      await app
        .getByRole("textbox", {
          name: new RegExp(text("应用名称", "Application name")),
        })
        .fill("Scope orders production");
      await app
        .getByRole("button", {
          name: text("预览变更", "Preview changes"),
          exact: true,
        })
        .click();
      await page
        .getByRole("button", {
          name: text("确认变更", "Confirm changes"),
          exact: true,
        })
        .click();
      await expect(
        page.getByRole("link", {
          name: "Scope orders production",
          exact: true,
        }),
      ).toBeVisible();
      await page.goto("/dashboards");
      await page
        .getByRole("button", {
          name: text("新建仪表盘", "Create dashboard"),
          exact: true,
        })
        .click();
      await page
        .getByRole("textbox", { name: text("名称", "Name"), exact: true })
        .fill("Scope filter workspace");
      await page
        .getByRole("button", { name: text("完成", "Done"), exact: true })
        .click();
      await page
        .getByRole("button", {
          name: text("预览并发布", "Preview and publish"),
          exact: true,
        })
        .click();
      await page
        .getByRole("dialog")
        .getByRole("button", {
          name: text("确认发布", "Confirm publication"),
          exact: true,
        })
        .click();
      await expect(page).toHaveURL(/\/dashboards\//);
      const appButton = () =>
        page.getByRole("button", {
          name: new RegExp(`^${text("筛选应用", "Filter applications")}:`),
        });
      await appButton().click();
      const apps = page.getByRole("dialog", {
        name: text("筛选应用", "Filter applications"),
        exact: true,
      });
      await apps
        .getByRole("textbox", { name: text("搜索列表", "Search list") })
        .fill("Scope orders");
      await apps
        .getByRole("button", {
          name: /^(选择|Select) Scope orders production$/,
        })
        .click();
      expect(
        (await new AxeBuilder({ page }).analyze()).violations.filter((v) =>
          ["critical", "serious"].includes(v.impact ?? ""),
        ),
      ).toEqual([]);
      await page.screenshot({
        path: info.outputPath("dashboard-application-picker.png"),
        fullPage: true,
      });
      await apps
        .getByRole("button", { name: /^(确认选择|Confirm selection)/ })
        .click();
      await expect(appButton()).toContainText(
        text("已选 1 个应用", "Selected applications: 1"),
      );
      await appButton().click();
      await apps
        .getByRole("button", {
          name: text("清空选择", "Clear selection"),
          exact: true,
        })
        .click();
      await apps
        .getByRole("button", { name: /^(确认选择|Confirm selection)/ })
        .click();
      await expect(appButton()).toContainText(
        text("已选 0 个应用", "Selected applications: 0"),
      );
      await appButton().click();
      await apps
        .getByRole("button", {
          name: text("不限制应用", "Any application"),
          exact: true,
        })
        .click();
      await apps
        .getByRole("button", { name: /^(确认选择|Confirm selection)/ })
        .click();
      const resourcesButton = () =>
        page.getByRole("button", {
          name: new RegExp(`^${text("选择资源", "Choose resources")}:`),
        });
      await resourcesButton().click();
      const resources = page.getByRole("dialog", {
        name: text("选择资源", "Choose resources"),
        exact: true,
      });
      await resources
        .getByRole("tab", { name: text("命名空间", "Namespaces"), exact: true })
        .click();
      await resources
        .getByRole("textbox", { name: text("搜索列表", "Search list") })
        .fill("production");
      await resources
        .getByRole("button", { name: /^(选择|Select) production$/ })
        .first()
        .click();
      await resources
        .getByRole("button", { name: /^(确认选择|Confirm selection)/ })
        .click();
      await expect(resourcesButton()).toContainText(
        text("已选 1 个资源", "Selected resources: 1"),
      );
      await resourcesButton().click();
      await resources
        .getByRole("button", {
          name: text("清空选择", "Clear selection"),
          exact: true,
        })
        .click();
      await resources
        .getByRole("button", { name: /^(确认选择|Confirm selection)/ })
        .click();
      await expect(resourcesButton()).toContainText(
        text("已选 0 个资源", "Selected resources: 0"),
      );
      await page.screenshot({
        path: info.outputPath("dashboard-explicit-empty.png"),
        fullPage: true,
      });
      expect(errors).toEqual([]);
    });
  }
