import { expect, test, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

async function login(page: Page, english: boolean, theme: string) {
  await page.addInitScript(
    ({ english, theme }) => {
      localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
      localStorage.setItem("argus.theme", theme);
    },
    { english, theme },
  );
  await page.goto("/login");
  await page.locator('input[autocomplete="username"]').fill("root");
  await page.locator('input[autocomplete="current-password"]').fill("123456");
  await page.locator('form button[type="submit"]').click();
  await expect(page).not.toHaveURL(/\/login/);
  await page.goto("/applications");
}

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`Plan8 application lifecycle ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      const errors: string[] = [];
      page.on("pageerror", (error) => errors.push(error.message));
      await page.setViewportSize({ width: 1440, height: 1000 });
      await login(page, english, theme);
      await expect(
        page.getByRole("heading", {
          name: english ? "Applications" : "应用",
          exact: true,
        }),
      ).toBeVisible();
      const createLabel = english ? "Create application" : "新建应用";
      const nameLabel = english ? "Application name" : "应用名称";
      const environmentLabel = english ? "Environment" : "环境";
      const previewLabel = english ? "Preview changes" : "预览变更";
      const confirmLabel = english ? "Confirm changes" : "确认变更";
      const names = [`Orders production ${theme}`, `Orders testing ${theme}`];
      for (let i = 0; i < names.length; i++) {
        await page
          .getByRole("button", { name: createLabel, exact: true })
          .click();
        const dialog = page.getByRole("dialog", {
          name: createLabel,
          exact: true,
        });
        await dialog
          .getByRole("textbox", { name: new RegExp(nameLabel) })
          .fill(names[i]!);
        if (i === 1) {
          await dialog
            .getByRole("button", { name: new RegExp(environmentLabel) })
            .click();
          await page
            .getByRole("option", {
              name: english ? "Testing" : "测试",
              exact: true,
            })
            .click();
        }
        await dialog
          .getByRole("button", { name: previewLabel, exact: true })
          .click();
        await page
          .getByRole("button", { name: confirmLabel, exact: true })
          .click();
        await expect(
          page.getByRole("link", { name: names[i], exact: true }),
        ).toBeVisible();
      }
      const prod = page.getByRole("link", { name: names[0], exact: true });
      const testApp = page.getByRole("link", { name: names[1], exact: true });
      expect(await prod.getAttribute("href")).not.toBe(
        await testApp.getAttribute("href"),
      );
      await testApp.click();
      await expect(
        page.getByRole("heading", { name: names[1], exact: true }),
      ).toBeVisible();
      await expect(
        page.getByText(english ? "No services discovered" : "尚未发现服务", {
          exact: true,
        }),
      ).toBeVisible();
      const editLabel = english ? "Edit application" : "编辑应用";
      await page.getByRole("button", { name: editLabel, exact: true }).click();
      const editor = page.getByRole("dialog", { name: editLabel, exact: true });
      await expect(
        editor.getByRole("button", { name: new RegExp(environmentLabel) }),
      ).toBeDisabled();
      const updatedName = names[1] + " updated";
      await editor
        .getByRole("textbox", { name: new RegExp(nameLabel) })
        .fill(updatedName);
      await editor
        .getByRole("button", { name: previewLabel, exact: true })
        .click();
      await page
        .getByRole("button", { name: confirmLabel, exact: true })
        .click();
      await expect(
        page.getByRole("heading", { name: updatedName, exact: true }),
      ).toBeVisible();
      await page
        .getByRole("button", {
          name: english ? "Declare service" : "声明服务",
          exact: true,
        })
        .click();
      const serviceDialog = page.getByRole("dialog", {
        name: english ? "Declare service" : "声明服务",
        exact: true,
      });
      await serviceDialog
        .getByRole("textbox", {
          name: english ? /^Service name\b/ : /服务名称/,
        })
        .fill("orders");
      await serviceDialog
        .getByRole("textbox", {
          name: new RegExp(english ? "Service namespace" : "服务命名空间"),
        })
        .fill("business");
      await serviceDialog
        .getByRole("button", { name: previewLabel, exact: true })
        .click();
      await page
        .getByRole("button", { name: confirmLabel, exact: true })
        .click();
      await expect(
        page
          .getByRole("row")
          .filter({ has: page.getByText("orders", { exact: true }) }),
      ).toContainText(english ? "No report received" : "尚未收到上报");
      const a11y = await new AxeBuilder({ page }).analyze();
      expect(
        a11y.violations.filter(
          (v) => v.impact === "critical" || v.impact === "serious",
        ),
      ).toEqual([]);
      await page.screenshot({
        path: info.outputPath("application-detail.png"),
        fullPage: true,
      });
      expect(errors).toEqual([]);
    });
  }
