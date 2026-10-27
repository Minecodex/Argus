import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`Plan8 UID-bound Pod log session ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      const errors: string[] = [];
      page.on("pageerror", (e) => errors.push(e.message));
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
      await page
        .getByRole("button", {
          name: english ? "All authorized namespaces" : "全部已授权命名空间",
          exact: true,
        })
        .click();
      await page
        .getByRole("dialog", {
          name: english ? "Choose a namespace" : "选择命名空间",
          exact: true,
        })
        .getByRole("button", { name: /^(Select|选择) production$/ })
        .click();
      await page
        .getByRole("button", {
          name: new RegExp(english ? "Resource type" : "资源类型"),
        })
        .click();
      await page.getByRole("option", { name: "Pod", exact: true }).click();
      const row = page
        .getByRole("row")
        .filter({ has: page.getByText("pod-example", { exact: true }) });
      await row
        .getByRole("button", {
          name: english ? "View logs" : "查看日志",
          exact: true,
        })
        .click();
      const dialog = page.getByRole("dialog", {
        name: english ? "Pod logs · pod-example" : "Pod 日志 · pod-example",
        exact: true,
      });
      await dialog
        .getByRole("spinbutton", {
          name: english ? "Initial tail lines" : "初始末尾行数",
          exact: true,
        })
        .fill("0");
      await dialog
        .getByRole("button", {
          name: english ? "Open log session" : "打开日志会话",
          exact: true,
        })
        .click();
      await expect(
        dialog.getByRole("button", {
          name: english ? "Open log session" : "打开日志会话",
          exact: true,
        }),
      ).toBeVisible();
      await dialog
        .getByRole("spinbutton", {
          name: english ? "Initial tail lines" : "初始末尾行数",
          exact: true,
        })
        .fill("100");
      await dialog
        .getByRole("button", {
          name: english ? "Open log session" : "打开日志会话",
          exact: true,
        })
        .click();
      await expect(dialog.locator("pre")).toContainText(
        "mock main current log",
      );
      await expect(dialog.locator("pre")).toContainText("[REDACTED]");
      await dialog
        .getByRole("switch", {
          name: english ? "Follow new logs" : "跟随新增日志",
          exact: true,
        })
        .press("Space");
      await expect(
        dialog.getByRole("switch", {
          name: english ? "Follow new logs" : "跟随新增日志",
          exact: true,
        }),
      ).not.toBeChecked();
      const refresh = dialog.getByRole("button", {
        name: english ? "Read new entries" : "读取新增",
        exact: true,
      });
      await expect(refresh).toBeEnabled();
      const paused = await dialog.locator("pre").textContent();
      // Wait beyond one polling interval: a paused session only checks access.
      await page.waitForTimeout(2300);
      await expect(dialog.locator("pre")).toHaveText(paused!);
      await refresh.click();
      await expect(dialog.locator("pre")).not.toHaveText(paused!);
      const violations = (
        await new AxeBuilder({ page }).analyze()
      ).violations.filter(
        (v) => v.impact === "critical" || v.impact === "serious",
      );
      expect(violations).toEqual([]);
      await page.screenshot({
        path: info.outputPath("pod-log-session.png"),
        fullPage: true,
      });
      const storage = await page.evaluate(() => JSON.stringify(localStorage));
      expect(storage).not.toContain("mock main current log");
      await dialog
        .getByRole("button", {
          name: english ? "Close log session" : "关闭日志会话",
          exact: true,
        })
        .click();
      await expect(dialog).toHaveCount(0);
      await row
        .getByRole("button", {
          name: english ? "View logs" : "查看日志",
          exact: true,
        })
        .click();
      await dialog
        .getByRole("switch", {
          name: english
            ? "Read the previously terminated container"
            : "读取上次终止容器日志",
          exact: true,
        })
        .press("Space");
      await dialog
        .getByRole("button", {
          name: english ? "Open log session" : "打开日志会话",
          exact: true,
        })
        .click();
      await expect(dialog.locator("pre")).toContainText("main previous");
      await expect(
        dialog.getByText(english ? "Bounded read complete" : "有界读取已完成", {
          exact: true,
        }),
      ).toBeVisible();
      await expect(
        dialog.getByRole("switch", {
          name: english ? "Follow new logs" : "跟随新增日志",
          exact: true,
        }),
      ).toBeDisabled();
      await page.keyboard.press("Escape");
      expect(errors).toEqual([]);
    });
  }
