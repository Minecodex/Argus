import { expect, test } from "@playwright/test";
import { selectTrigger } from "./helpers/select";
import { selectRowAction } from "./helpers/action-menu";

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`Model thinking depth ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      const tx = (zh: string, en: string) => (english ? en : zh);
      await page.setViewportSize({ width: 1440, height: 1000 });
      await page.addInitScript(
        ({ english, theme }) => {
          localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
          localStorage.setItem("argus.theme", theme);
        },
        { english, theme },
      );
      await page.goto("/login");
      await page.getByLabel(tx("用户名", "Username")).fill("root");
      await page.getByLabel(tx("密码", "Password")).fill("123456");
      await page
        .getByRole("button", { name: tx("登录", "Sign in"), exact: true })
        .click();
      await expect(page).not.toHaveURL(/login/);
      await page.goto("/settings/aiops?tab=models");
      await page
        .getByRole("button", { name: tx("添加模型", "Add model"), exact: true })
        .click();
      const drawer = page.getByRole("dialog");
      const depth = () =>
        selectTrigger(drawer, tx("思考深度", "Thinking depth"));
      await expect(depth()).toBeDisabled();
      await selectTrigger(drawer, tx("API 协议", "API protocol")).click();
      await page
        .getByRole("option", { name: "Chat Completions", exact: true })
        .click();
      await expect(depth()).toBeEnabled();
      await depth().click();
      await page
        .getByRole("option", { name: tx("中", "Medium"), exact: true })
        .click();
      await drawer.getByLabel(tx("名称", "Name")).fill("Reasoning fixture");
      await drawer
        .getByLabel(tx("API 地址", "API URL"))
        .fill("https://models.example.test/v1");
      await drawer
        .getByLabel(tx("API Key", "API key"))
        .fill("mock-reasoning-only");
      await drawer
        .getByLabel(tx("模型 ID", "Model ID"))
        .fill("reasoning-fixture");
      await drawer
        .getByText(tx("容量与计费设置", "Capacity and pricing settings"), {
          exact: true,
        })
        .click();
      await drawer
        .getByLabel(tx("每百万输入 Token 金额", "Input amount per 1M tokens"))
        .fill("1");
      await drawer
        .getByLabel(tx("每百万输出 Token 金额", "Output amount per 1M tokens"))
        .fill("2");
      await drawer
        .getByRole("button", {
          name: tx("测试并创建", "Test and create"),
          exact: true,
        })
        .click();
      await expect(drawer).toHaveCount(0);
      let row = page.getByRole("row").filter({ hasText: "Reasoning fixture" });
      await expect(row).toContainText(tx("中", "Medium"));
      await selectRowAction(row, tx("编辑", "Edit"));
      await expect(depth()).toContainText(tx("中", "Medium"));
      await expect(drawer.getByLabel(tx("API Key", "API key"))).toHaveValue("");
      await selectTrigger(drawer, tx("API 协议", "API protocol")).click();
      await page
        .getByRole("option", { name: "Responses", exact: true })
        .click();
      await expect(depth()).toBeDisabled();
      await expect(depth()).toContainText(tx("供应商默认", "Provider default"));
      await selectTrigger(drawer, tx("API 协议", "API protocol")).click();
      await page
        .getByRole("option", { name: "Chat Completions", exact: true })
        .click();
      await expect(depth()).toBeEnabled();
      await depth().click();
      await page
        .getByRole("option", {
          name: tx("供应商默认", "Provider default"),
          exact: true,
        })
        .click();
      await drawer
        .getByRole("button", { name: tx("保存", "Save"), exact: true })
        .click();
      await expect(drawer).toHaveCount(0);
      await page.reload();
      row = page.getByRole("row").filter({ hasText: "Reasoning fixture" });
      await expect(row).toContainText(tx("供应商默认", "Provider default"));
      await selectRowAction(row, tx("编辑", "Edit"));
      await expect(depth()).toContainText(tx("供应商默认", "Provider default"));
      await depth().click();
      await expect(
        page.getByRole("option", { name: tx("最高", "Maximum"), exact: true }),
      ).toBeVisible();
      await page.screenshot({
        path: info.outputPath("thinking-depth.png"),
        animations: "disabled",
      });
    });
  }
