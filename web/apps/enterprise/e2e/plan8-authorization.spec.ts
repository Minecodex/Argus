import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { selectRowAction } from "./helpers/action-menu";

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`Plan8 application and namespace grants ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      const errors: string[] = [];
      page.on("pageerror", (error) => errors.push(error.message));
      await page.setViewportSize({ width: 1440, height: 1100 });
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
      const app = page.getByRole("dialog", {
        name: english ? "Create application" : "新建应用",
        exact: true,
      });
      await app
        .getByRole("textbox", {
          name: new RegExp(english ? "Application name" : "应用名称"),
        })
        .fill("Plan8 authorized application");
      await app
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
        page.getByRole("link", {
          name: "Plan8 authorized application",
          exact: true,
        }),
      ).toBeVisible();
      await page.goto("/kubernetes/k8s-prod-east");
      await expect(
        page.getByRole("button", {
          name: new RegExp(english ? "Resource type" : "资源类型"),
        }),
      ).toBeVisible();
      await expect(
        page.getByText("deployment-example", { exact: true }).first(),
      ).toBeVisible();
      await page.goto("/settings/org");
      const user = page.getByRole("row").filter({ hasText: "@chenxi" });
      const label = english ? "Data authorization" : "数据授权";
      await selectRowAction(user, label);
      const drawer = page.getByRole("dialog", {
        name: english ? /^Data authorization/ : /^数据授权/,
      });
      await drawer
        .getByRole("tab", {
          name: english ? "Applications" : "应用",
          exact: true,
        })
        .click();
      const appRow = drawer
        .getByRole("row")
        .filter({
          has: page.getByText("Plan8 authorized application", { exact: true }),
        });
      await appRow
        .getByRole("button", { name: english ? "Grant" : "授予", exact: true })
        .click();
      await drawer
        .getByRole("tab", {
          name: english ? "Namespaces" : "Namespace",
          exact: true,
        })
        .click();
      const namespaceRow = drawer
        .getByRole("row")
        .filter({ has: page.getByText("production", { exact: true }) });
      await expect(namespaceRow).toContainText("k8s-prod-east");
      await expect(namespaceRow.locator("code")).toHaveText(/[0-9a-f-]{36}/);
      await namespaceRow
        .getByRole("button", { name: english ? "Grant" : "授予", exact: true })
        .click();
      await drawer
        .getByRole("button", { name: english ? "Save" : "保存", exact: true })
        .click();
      await expect(drawer).not.toBeVisible();
      await selectRowAction(user, label);
      await drawer
        .getByRole("tab", {
          name: english ? "Applications" : "应用",
          exact: true,
        })
        .click();
      await expect(
        appRow.getByRole("button", {
          name: english ? "Revoke direct grant" : "撤销直接授权",
          exact: true,
        }),
      ).toBeVisible();
      await drawer
        .getByRole("tab", {
          name: english ? "Namespaces" : "Namespace",
          exact: true,
        })
        .click();
      await expect(
        namespaceRow.getByRole("button", {
          name: english ? "Revoke direct grant" : "撤销直接授权",
          exact: true,
        }),
      ).toBeVisible();
      const result = await new AxeBuilder({ page }).analyze();
      expect(
        result.violations.filter(
          (v) => v.impact === "critical" || v.impact === "serious",
        ),
      ).toEqual([]);
      await page.screenshot({
        path: info.outputPath("namespace-grants.png"),
        fullPage: true,
      });
      await namespaceRow
        .getByRole("button", {
          name: english ? "Revoke direct grant" : "撤销直接授权",
          exact: true,
        })
        .click();
      await drawer
        .getByRole("button", { name: english ? "Save" : "保存", exact: true })
        .click();
      await expect(drawer).not.toBeVisible();
      await selectRowAction(user, label);
      await drawer
        .getByRole("tab", {
          name: english ? "Namespaces" : "Namespace",
          exact: true,
        })
        .click();
      await expect(
        namespaceRow.getByRole("button", {
          name: english ? "Grant" : "授予",
          exact: true,
        }),
      ).toBeVisible();
      expect(errors).toEqual([]);
    });
  }
