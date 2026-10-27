import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`Plan8 CronJob manual run ${english ? "en" : "zh"} ${theme}`, async ({
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
      await page
        .getByRole("button", {
          name: text("全部已授权命名空间", "All authorized namespaces"),
          exact: true,
        })
        .click();
      await page
        .getByRole("dialog", {
          name: text("选择命名空间", "Choose a namespace"),
          exact: true,
        })
        .getByRole("button", { name: /^(Select|选择) production$/ })
        .click();
      await page
        .getByRole("button", {
          name: new RegExp(text("资源类型", "Resource type")),
        })
        .click();
      await page.getByRole("option", { name: "CronJob", exact: true }).click();
      const row = page
        .getByRole("row")
        .filter({ has: page.getByText("cronjob-example", { exact: true }) });
      await row
        .getByRole("button", {
          name: text("运行一次", "Run once"),
          exact: true,
        })
        .click();
      const dialog = page.getByRole("dialog", {
        name: text(
          "从 cronjob-example 创建一次 Job",
          "Create a Job from cronjob-example",
        ),
        exact: true,
      });
      await expect(dialog).toContainText(
        text(
          "暂停或定时计划不会阻止手动创建",
          "Suspension and schedules do not prevent manual creation",
        ),
      );
      await dialog
        .getByRole("textbox", {
          name: new RegExp(text("Job 名称", "Job name")),
        })
        .fill("plan8-manual-run");
      await dialog
        .getByRole("button", {
          name: text("验证并预览差异", "Validate and preview"),
          exact: true,
        })
        .click();
      await expect(dialog).toContainText("plan8-manual-run");
      await expect(dialog).toContainText("cronjob-example");
      expect(
        (await new AxeBuilder({ page }).analyze()).violations.filter(
          (item) => item.impact === "critical" || item.impact === "serious",
        ),
      ).toEqual([]);
      await page.screenshot({
        path: info.outputPath("cronjob-manual-preview.png"),
        fullPage: true,
      });
      await dialog
        .getByRole("button", {
          name: text("确认执行", "Confirm operation"),
          exact: true,
        })
        .click();
      const history = page.getByRole("dialog", {
        name: text("操作记录", "Operation history"),
        exact: true,
      });
      await expect(
        history
          .getByRole("row")
          .filter({ has: page.getByText("plan8-manual-run", { exact: true }) }),
      ).toContainText(text("已生效", "Applied"));
      await page.keyboard.press("Escape");
      await page
        .getByRole("button", {
          name: new RegExp(text("资源类型", "Resource type")),
        })
        .click();
      await page.getByRole("option", { name: "Job", exact: true }).click();
      const job = page
        .getByRole("row")
        .filter({ has: page.getByText("plan8-manual-run", { exact: true }) });
      await expect(job).toContainText("CronJob/cronjob-example");
      expect(errors).toEqual([]);
    });
  }
