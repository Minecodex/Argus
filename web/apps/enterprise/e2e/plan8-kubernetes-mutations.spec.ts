import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`Plan8 Kubernetes mutation ${english ? "en" : "zh"} ${theme}`, async ({
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
        .getByRole("tab", {
          name: english ? "Configuration" : "配置",
          exact: true,
        })
        .click();
      await page
        .getByRole("button", {
          name: english ? "Create object" : "新建对象",
          exact: true,
        })
        .click();
      const create = page
        .getByRole("dialog", { name: "Create ConfigMap", exact: true })
        .or(page.getByRole("dialog", { name: "新建 ConfigMap", exact: true }));
      const source = create.getByRole("textbox", {
        name: english
          ? "Object manifest (YAML / JSON)"
          : "对象清单（YAML / JSON）",
        exact: true,
      });
      await source.fill("apiVersion: v1\nkind: ConfigMap\nkind: Secret\n");
      await create
        .getByRole("button", {
          name: english ? "Format document" : "格式化",
          exact: true,
        })
        .click();
      await expect(create.getByRole("alert")).toContainText(
        english ? "valid YAML" : "有效",
      );
      await create
        .getByRole("textbox", {
          name: english
            ? "Object manifest (YAML / JSON)"
            : "对象清单（YAML / JSON）",
          exact: true,
        })
        .fill(
          JSON.stringify(
            {
              apiVersion: "v1",
              kind: "ConfigMap",
              metadata: { name: "plan8-ui-config", namespace: "production" },
              data: { mode: "reviewed" },
            },
            null,
            2,
          ),
        );
      await create.getByRole("radio", { name: "JSON", exact: true }).focus();
      await create
        .getByRole("radio", { name: "JSON", exact: true })
        .press("Space");
      await expect(
        create.getByRole("radio", { name: "JSON", exact: true }),
      ).toBeChecked();
      await expect(source).toHaveValue(/"apiVersion": "v1"/);
      await create.getByRole("radio", { name: "YAML", exact: true }).focus();
      await create
        .getByRole("radio", { name: "YAML", exact: true })
        .press("Space");
      await expect(source).toHaveValue(/apiVersion: v1/);
      const editorAccessibility = await new AxeBuilder({ page }).analyze();
      expect(
        editorAccessibility.violations.filter(
          (v) => v.impact === "critical" || v.impact === "serious",
        ),
      ).toEqual([]);
      await page.screenshot({
        path: info.outputPath("kubernetes-yaml-editor.png"),
        fullPage: true,
      });
      await create
        .getByRole("button", {
          name: english ? "Validate and preview" : "验证并预览差异",
          exact: true,
        })
        .click();
      await expect(create).toContainText("reviewed");
      await create
        .getByRole("button", {
          name: english ? "Confirm operation" : "确认执行",
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
          .filter({ has: page.getByText("plan8-ui-config", { exact: true }) }),
      ).toContainText(english ? "Applied" : "已生效");
      await page.keyboard.press("Escape");
      const row = page
        .getByRole("row")
        .filter({ has: page.getByText("plan8-ui-config", { exact: true }) });
      await expect(row).toBeVisible();
      await row
        .getByRole("button", {
          name: english ? "Manage object" : "管理对象",
          exact: true,
        })
        .click();
      const edit = page.getByRole("dialog", {
        name: english
          ? "Manage ConfigMap · plan8-ui-config"
          : "管理 ConfigMap · plan8-ui-config",
        exact: true,
      });
      await edit
        .getByRole("textbox", {
          name: english
            ? "Object manifest (YAML / JSON)"
            : "对象清单（YAML / JSON）",
          exact: true,
        })
        .fill('{"data":{"mode":"updated"}}');
      await edit
        .getByRole("button", {
          name: english ? "Validate and preview" : "验证并预览差异",
          exact: true,
        })
        .click();
      await expect(edit).toContainText("updated");
      const accessibility = await new AxeBuilder({ page }).analyze();
      expect(
        accessibility.violations.filter(
          (v) => v.impact === "critical" || v.impact === "serious",
        ),
      ).toEqual([]);
      await page.screenshot({
        path: info.outputPath("kubernetes-change-preview.png"),
        fullPage: true,
      });
      await edit
        .getByRole("button", {
          name: english ? "Confirm operation" : "确认执行",
          exact: true,
        })
        .click();
      await expect(
        history
          .getByRole("row")
          .filter({ has: page.getByText("plan8-ui-config", { exact: true }) }),
      ).toHaveCount(2);
      await page.keyboard.press("Escape");
      await row
        .getByRole("button", {
          name: english ? "View object" : "查看对象",
          exact: true,
        })
        .click();
      await expect(page.getByRole("dialog").locator("pre")).toContainText(
        '"mode": "updated"',
      );
      await page.keyboard.press("Escape");
      await page
        .getByRole("button", {
          name: new RegExp(english ? "Resource type" : "资源类型"),
        })
        .click();
      await page.getByRole("option", { name: "Secret", exact: true }).click();
      await page
        .getByRole("button", {
          name: english ? "Create object" : "新建对象",
          exact: true,
        })
        .click();
      const secretDialog = page.getByRole("dialog", {
        name: english ? "Create Secret" : "新建 Secret",
        exact: true,
      });
      await secretDialog
        .getByRole("textbox", {
          name: english
            ? "Object manifest (YAML / JSON)"
            : "对象清单（YAML / JSON）",
          exact: true,
        })
        .fill(
          JSON.stringify({
            apiVersion: "v1",
            kind: "Secret",
            metadata: { name: "plan8-private-ui", namespace: "production" },
            type: "Opaque",
          }),
        );
      await secretDialog
        .getByRole("button", {
          name: english ? "Add Secret key" : "添加 Secret key",
          exact: true,
        })
        .click();
      await secretDialog
        .getByRole("textbox", { name: "Secret key 1", exact: true })
        .fill("password");
      const canary = "plan8-ui-private-canary";
      await secretDialog
        .getByRole("textbox", {
          name: english ? "Secret value 1" : "Secret 值 1",
          exact: true,
        })
        .fill(canary);
      await secretDialog
        .getByRole("button", {
          name: english ? "Validate and preview" : "验证并预览差异",
          exact: true,
        })
        .click();
      await expect(secretDialog).not.toContainText(canary);
      await expect(secretDialog).toContainText("[REDACTED]");
      expect(
        await page.evaluate(
          (value) =>
            Object.values(localStorage).some((item) => item.includes(value)),
          canary,
        ),
      ).toBe(false);
      await secretDialog
        .getByRole("button", {
          name: english ? "Confirm operation" : "确认执行",
          exact: true,
        })
        .click();
      await expect(
        history
          .getByRole("row")
          .filter({ has: page.getByText("plan8-private-ui", { exact: true }) }),
      ).toContainText(english ? "Applied" : "已生效");
      expect(
        await page.evaluate(
          (value) =>
            Object.values(localStorage).some((item) => item.includes(value)),
          canary,
        ),
      ).toBe(false);
      expect(errors).toEqual([]);
    });
  }
