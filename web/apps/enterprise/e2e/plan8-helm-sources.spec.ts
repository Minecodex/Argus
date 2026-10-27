import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`Plan8 Helm repository OCI ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      test.setTimeout(120_000);
      const text = (zh: string, en: string) => (english ? en : zh);
      const errors: string[] = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.setViewportSize({ width: 1440, height: 1000 });
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
      const open = async () => {
        await page.goto("/kubernetes/k8s-prod-east");
        await page
          .getByRole("button", {
            name: text("Helm 发布管理", "Helm releases"),
            exact: true,
          })
          .click();
        const workbench = page.getByRole("dialog", {
          name: text("Helm 发布管理", "Helm releases"),
          exact: true,
        });
        await workbench
          .getByRole("button", {
            name: text("选择命名空间", "Choose a namespace"),
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
        return workbench;
      };
      for (const kind of ["repository", "oci"] as const) {
        const workbench = await open();
        await workbench
          .getByRole("button", { name: text("安装", "Install"), exact: true })
          .click();
        const form = page.getByRole("dialog", {
          name: text("安装 Helm 发布", "Install Helm release"),
          exact: true,
        });
        await form
          .getByRole("textbox", {
            name: new RegExp(text("发布名称", "Release name")),
          })
          .fill(`source-${kind}`);
        await form.locator(".argus-select__trigger").click();
        await page
          .getByRole("option", {
            name:
              kind === "repository"
                ? text("Helm 仓库", "Helm repository")
                : text("OCI Registry", "OCI registry"),
            exact: true,
          })
          .click();
        await form
          .getByRole("textbox", {
            name: new RegExp(text("来源地址", "Source address")),
          })
          .fill(
            kind === "repository"
              ? "https://charts.example/team"
              : "oci://registry.example/team/sample",
          );
        if (kind === "repository")
          await form
            .getByRole("textbox", {
              name: new RegExp(text("Chart 名称", "Chart name")),
            })
            .fill("sample");
        await form
          .getByRole("textbox", {
            name: new RegExp(text("精确 Chart 版本", "Exact chart version")),
          })
          .fill("1.0.0");
        await form
          .getByRole("button", {
            name: text(
              "摘要、认证与网络设置",
              "Digest, authentication and network settings",
            ),
            exact: true,
          })
          .click();
        await form
          .getByRole("textbox", {
            name: new RegExp(text("仓库用户名", "Repository username")),
          })
          .fill("reader");
        await form
          .getByLabel(new RegExp(text("仓库密码", "Repository password")))
          .fill("source-private-portal-canary");
        await form
          .getByRole("textbox", {
            name: new RegExp(text("操作原因", "Reason")),
          })
          .fill("Verify protected Helm source");
        await form
          .getByRole("button", {
            name: text("验证并预览差异", "Validate and preview"),
            exact: true,
          })
          .click();
        await expect(form).toContainText("ConfigMap");
        await expect(form).toContainText("not-verified-in-mock");
        await expect(form).not.toContainText("source-private-portal-canary");
        expect(
          await page.evaluate(() => JSON.stringify(localStorage)),
        ).not.toContain("source-private-portal-canary");
        const accessibility = await new AxeBuilder({ page }).analyze();
        expect(
          accessibility.violations.filter(
            (v) => v.impact === "critical" || v.impact === "serious",
          ),
        ).toEqual([]);
        await page.screenshot({
          path: info.outputPath(`${kind}-preview.png`),
          fullPage: true,
        });
        await form
          .getByRole("button", {
            name: text("确认执行", "Confirm operation"),
            exact: true,
          })
          .click();
        await expect(form).not.toBeVisible();
        await open();
        await expect(
          workbench
            .getByRole("row")
            .filter({ has: page.getByText(`source-${kind}`, { exact: true }) }),
        ).toContainText("1.0.0");
      }
      expect(errors).toEqual([]);
    });
  }
