import { selectWorkbenchNamespace } from "./helpers/kubernetes-workbench";
import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`Plan8 automatic UID-bound Pod logs ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      const text = (zh: string, en: string) => (english ? en : zh);
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
      await selectWorkbenchNamespace(page, "production");
      await page.getByRole("tab", { name: "Pod", exact: true }).click();
      const row = page
        .getByRole("row")
        .filter({ has: page.getByText("pod-example", { exact: true }) });
      await row
        .getByRole("button", {
          name: text("查看日志", "View logs"),
          exact: true,
        })
        .click();
      const dialog = page.getByRole("dialog", {
        name: text("Pod 日志 · pod-example", "Pod logs · pod-example"),
        exact: true,
      });
      const output = dialog.getByRole("log");
      await expect(output).toContainText("mock main current log");
      await expect(output).toContainText("[REDACTED]");
      await expect(
        dialog.getByRole("button", {
          name: text("打开日志会话", "Open log session"),
          exact: true,
        }),
      ).toHaveCount(0);
      await dialog
        .getByRole("button", { name: text("暂停", "Pause"), exact: true })
        .click();
      const frozen = await output.textContent();
      await page.waitForTimeout(2300);
      await expect(output).toHaveText(frozen!);
      await dialog
        .getByRole("button", { name: text("继续", "Resume"), exact: true })
        .click();
      await expect(output).not.toHaveText(frozen!);
      await dialog
        .getByRole("button", {
          name: text("日志选项", "Log options"),
          exact: true,
        })
        .click();
      const tail = dialog.getByRole("spinbutton", {
        name: text("初始末尾行数", "Initial tail lines"),
        exact: true,
      });
      await tail.fill("0");
      await dialog
        .getByRole("button", {
          name: text("应用并重新读取", "Apply and reload"),
          exact: true,
        })
        .click();
      await expect(tail).toBeVisible();
      await tail.fill("120");
      await dialog
        .getByRole("button", {
          name: text("应用并重新读取", "Apply and reload"),
          exact: true,
        })
        .click();
      await expect(tail).not.toBeVisible();
      await expect(output).toContainText("mock main current log");
      expect(
        (await new AxeBuilder({ page }).analyze()).violations.filter(
          (v) => v.impact === "critical" || v.impact === "serious",
        ),
      ).toEqual([]);
      await page.screenshot({
        path: info.outputPath("automatic-pod-logs.png"),
        fullPage: true,
      });
      expect(
        await page.evaluate(() => JSON.stringify(localStorage)),
      ).not.toContain("mock main current log");
      await dialog
        .getByRole("switch", {
          name: text(
            "读取上次终止容器日志",
            "Read the previously terminated container",
          ),
          exact: true,
        })
        .press("Space");
      await expect(output).toContainText("main previous");
      await expect(
        dialog.getByText(text("有界读取已完成", "Bounded read complete"), {
          exact: true,
        }),
      ).toBeVisible();
      await dialog
        .getByRole("button", {
          name: text("关闭日志会话", "Close log session"),
          exact: true,
        })
        .click();
      await expect(dialog).not.toBeVisible();
      expect(errors).toEqual([]);
    });
  }
