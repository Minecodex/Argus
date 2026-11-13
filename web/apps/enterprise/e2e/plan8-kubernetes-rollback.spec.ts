import { selectRowAction } from "./helpers/action-menu";
import { selectWorkbenchNamespace } from "./helpers/kubernetes-workbench";
import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`Plan8 workload rollback ${english ? "en" : "zh"} ${theme}`, async ({
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
      await page.goto("/kubernetes/k8s-prod-east?tab=resources");
      await selectWorkbenchNamespace(page, "production");
      for (const [type, kind] of [
        ["deployment", "Deployment"],
        ["statefulset", "StatefulSet"],
        ["daemonset", "DaemonSet"],
      ]) {
        await page.getByRole("tab", { name: kind, exact: true }).click();
        const name = `${type}-example`;
        const row = page
          .getByRole("row")
          .filter({ has: page.getByText(name, { exact: true }) });
        await selectRowAction(row, english ? "Roll back" : "回滚");
        const dialog = page.getByRole("dialog", {
          name: `${english ? "Roll back" : "回滚"} · ${name}`,
          exact: true,
        });

        const choose = dialog.getByRole("button", {
          name: english ? "Choose a historical revision" : "选择历史修订",
          exact: true,
        });
        await choose.click();
        const picker = page.getByRole("dialog", {
          name: english ? "Choose a historical revision" : "选择历史修订",
          exact: true,
        });
        const search = picker.getByRole("textbox", {
          name: english ? "Search list" : "搜索列表",
          exact: true,
        });
        await search.fill("missing-history");
        await expect(picker).toContainText(
          english ? "No matching results" : "没有匹配的结果",
        );
        await page.keyboard.press("Escape");
        await expect(picker).not.toBeVisible();
        await expect(choose).toBeVisible();
        await choose.click();
        await search.fill(`${name}-revision-1`);
        await picker
          .getByRole("button", {
            name: `${english ? "Select" : "选择"} 1 · ${name}-revision-1`,
            exact: true,
          })
          .click();
        await expect(dialog).toContainText(`1 · ${name}-revision-1`);
        await dialog
          .getByRole("button", {
            name: english ? "Validate and preview" : "验证并预览差异",
            exact: true,
          })
          .click();
        await expect(dialog).toContainText("alpine:3.20");
        const accessibility = await new AxeBuilder({ page }).analyze();
        expect(
          accessibility.violations.filter(
            (item) => item.impact === "critical" || item.impact === "serious",
          ),
        ).toEqual([]);
        if (type === "deployment")
          await page.screenshot({
            path: info.outputPath("workload-rollback-preview.png"),
            fullPage: true,
          });
        await dialog
          .getByRole("button", {
            name: english ? "Confirm operation" : "确认执行",
            exact: true,
          })
          .click();
        await expect(
          page.locator(".argus-k8s-operation-feedback"),
        ).toContainText(english ? "Applied" : "已生效");
        await page
          .getByRole("button", {
            name: english ? "Operation history" : "操作记录",
            exact: true,
          })
          .click();
        const history = page.getByRole("dialog", {
          name: english ? "Operation history" : "操作记录",
          exact: true,
        });
        await expect(
          history
            .getByRole("row")
            .filter({ has: page.getByText(name, { exact: true }) }),
        ).toContainText(english ? "Applied" : "已生效");
        await page.keyboard.press("Escape");
        await row.getByRole("button", { name, exact: true }).click();
        await page
          .getByRole("dialog")
          .getByRole("tab", { name: "YAML / JSON", exact: true })
          .click();
        await expect(page.getByRole("dialog").locator("pre")).toContainText(
          "alpine:3.20",
        );
        await page.keyboard.press("Escape");
      }
      expect(errors).toEqual([]);
    });
  }
