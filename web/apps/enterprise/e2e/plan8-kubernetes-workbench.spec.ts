import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`Plan8 Kubernetes inventory ${english ? "en" : "zh"} ${theme}`, async ({
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
      await page.goto("/kubernetes/k8s-prod-east");
      await expect(
        page.getByRole("button", {
          name: new RegExp(english ? "Resource type" : "资源类型"),
        }),
      ).toBeVisible();
      await page
        .getByRole("button", {
          name: english ? "All authorized namespaces" : "全部已授权命名空间",
          exact: true,
        })
        .click();
      const picker = page.getByRole("dialog", {
        name: english ? "Choose a namespace" : "选择命名空间",
        exact: true,
      });
      await picker
        .getByRole("button", { name: /^(Select|选择) production$/ })
        .click();
      await expect(
        page
          .getByRole("row")
          .filter({
            has: page.getByText("deployment-example", { exact: true }),
          }),
      ).toHaveCount(1);
      await page
        .getByRole("tab", {
          name: english ? "Configuration" : "配置",
          exact: true,
        })
        .click();
      await page
        .getByRole("button", {
          name: new RegExp(english ? "Resource type" : "资源类型"),
        })
        .click();
      await page.getByRole("option", { name: "Secret", exact: true }).click();
      const row = page
        .getByRole("row")
        .filter({ has: page.getByText("secret-example", { exact: true }) });
      await expect(row).toBeVisible();
      await row
        .getByRole("button", {
          name: english ? "View object" : "查看对象",
          exact: true,
        })
        .click();
      const dialog = page.getByRole("dialog", {
        name: "Secret · secret-example",
        exact: true,
      });
      await expect(
        dialog.getByText(
          english
            ? "Credentials and Secret contents are redacted."
            : "凭据和 Secret 内容已屏蔽。",
          { exact: true },
        ),
      ).toBeVisible();
      await expect(dialog.locator("pre")).toContainText("[REDACTED]");
      await expect(dialog.locator("pre")).toContainText(
        '"namespace": "production"',
      );
      await page.keyboard.press("Escape");
      const accessibility = await new AxeBuilder({ page }).analyze();
      expect(
        accessibility.violations.filter(
          (value) => value.impact === "critical" || value.impact === "serious",
        ),
      ).toEqual([]);
      await page.screenshot({
        path: info.outputPath("kubernetes-inventory.png"),
        fullPage: true,
      });
      expect(errors).toEqual([]);
    });
  }
