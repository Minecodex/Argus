import { expect, test } from "@playwright/test";

for (const english of [false, true])
  test(
    "role matrix exposes object telemetry access " +
      (english ? "English dark" : "Chinese light"),
    async ({ page }, testInfo) => {
      await page.addInitScript(
        ({ english }) => {
          localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
          localStorage.setItem("argus.theme", english ? "dark" : "light");
        },
        { english },
      );
      await page.goto("/login");
      await page.getByLabel(english ? "Username" : "用户名").fill("root");
      await page.getByLabel(english ? "Password" : "密码").fill("123456");
      await page
        .getByRole("button", {
          name: english ? "Sign in" : "登录",
          exact: true,
        })
        .click();
      await expect(page).not.toHaveURL(/\/login/);
      await page.goto("/settings/org");
      await page
        .getByRole("tab", { name: english ? "Roles" : "角色", exact: true })
        .click();
      await page
        .getByRole("button", {
          name: english ? "New role" : "新建角色",
          exact: true,
        })
        .click();
      const drawer = page.getByRole("dialog");
      const dashboard = drawer
        .locator(".argus-perm-matrix__row")
        .filter({
          has: page.locator(".argus-perm-matrix__resource", {
            hasText: english ? "Telemetry Dashboard" : "仪表盘",
          }),
        });
      await expect(
        dashboard.getByRole("checkbox", {
          name: english ? "Read" : "查看",
          exact: true,
        }),
      ).toBeVisible();
      await expect(
        dashboard.getByRole("checkbox", {
          name: english ? "Manage" : "管理",
          exact: true,
        }),
      ).toBeVisible();
      await expect(
        drawer.getByText(english ? "Read sensitive fields" : "查看敏感字段", {
          exact: true,
        }),
      ).toHaveCount(0);
      await expect(
        drawer.getByText(english ? "Telemetry Query" : "遥测查询", {
          exact: true,
        }),
      ).toHaveCount(0);
      await drawer
        .getByRole("textbox", { name: english ? "Name" : "名称", exact: true })
        .fill("Object reader");
      await dashboard
        .getByRole("checkbox", { name: english ? "Read" : "查看", exact: true })
        .click();
      await drawer
        .getByRole("button", { name: english ? "Submit" : "提交", exact: true })
        .click();
      await expect(drawer).not.toBeVisible();
      await expect(
        page.getByText("Object reader", { exact: true }),
      ).toBeVisible();
      await page.screenshot({
        path: testInfo.outputPath("object-telemetry-permissions.png"),
        fullPage: true,
      });
    },
  );
