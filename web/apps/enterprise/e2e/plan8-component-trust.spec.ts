import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`Plan8 component trust ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      await page.setViewportSize({ width: 1600, height: 1100 });
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
      await page.goto("/kubernetes/k8s-prod-east#otlp-collector");
      const scheduler = page.getByRole("row").filter({
        has: page.getByText(english ? "Scheduler metrics" : "Scheduler 指标", {
          exact: true,
        }),
      });
      await scheduler
        .getByRole("button", {
          name: english ? "Certificate trust" : "证书信任",
          exact: true,
        })
        .click();
      const trust = page.getByRole("dialog", {
        name: english
          ? "Component metric certificate trust"
          : "组件指标证书信任",
        exact: true,
      });
      await trust
        .getByRole("textbox", {
          name: english ? "Certificate server name" : "证书服务名",
          exact: true,
        })
        .fill("scheduler.internal");
      const ca = trust.getByRole("textbox", {
        name: english
          ? "Trusted public certificates (PEM)"
          : "受信任的公开证书（PEM）",
        exact: true,
      });
      await ca.fill(
        "-----BEGIN PRIVATE KEY-----\nrejected\n-----END PRIVATE KEY-----",
      );
      await trust
        .getByRole("button", {
          name: english ? "Preview trust configuration" : "预览信任配置",
          exact: true,
        })
        .click();
      await expect(trust.getByRole("alert")).toBeVisible();
      await ca.fill("");
      expect(
        (await new AxeBuilder({ page }).include('[role="dialog"]').analyze())
          .violations,
      ).toEqual([]);
      await page.screenshot({ path: info.outputPath("component-trust.png") });
      await trust
        .getByRole("button", {
          name: english ? "Preview trust configuration" : "预览信任配置",
          exact: true,
        })
        .click();
      await expect(trust).not.toBeVisible();
      const confirm = async () => {
        const dialog = page.getByRole("dialog", {
          name: english
            ? "Preview own resource monitoring"
            : "自身监控配置预览",
          exact: true,
        });
        await dialog.getByRole("button", { name: /^(确认|Confirm)/ }).click();
        await expect(dialog).not.toBeVisible();
      };
      await confirm();
      await expect(scheduler.getByRole("switch")).not.toBeChecked();
      await scheduler.getByRole("switch").focus();
      await scheduler.getByRole("switch").press("Space");
      await confirm();
      await expect(scheduler.getByRole("switch")).toBeChecked();
      await scheduler.getByRole("switch").focus();
      await scheduler.getByRole("switch").press("Space");
      await confirm();
      await expect(scheduler.getByRole("switch")).not.toBeChecked();
      await scheduler
        .getByRole("button", {
          name: english ? "Certificate trust" : "证书信任",
          exact: true,
        })
        .click();
      await expect(
        trust.getByRole("textbox", {
          name: english ? "Certificate server name" : "证书服务名",
          exact: true,
        }),
      ).toHaveValue("scheduler.internal");
      await expect(ca).toHaveValue("");
    });
  }
