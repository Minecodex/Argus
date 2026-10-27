import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`Plan8 SDK key lifecycle ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
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
      await page.goto("/applications");
      await page
        .getByRole("button", {
          name: english ? "Create application" : "新建应用",
          exact: true,
        })
        .click();
      const create = page.getByRole("dialog", {
        name: english ? "Create application" : "新建应用",
        exact: true,
      });
      await create
        .getByRole("textbox", {
          name: new RegExp(english ? "Application name" : "应用名称"),
        })
        .fill("SDK Orders");
      await create
        .getByRole("button", {
          name: english ? "Preview changes" : "预览变更",
          exact: true,
        })
        .click();
      await page
        .getByRole("button", {
          name: english ? "Confirm changes" : "确认变更",
          exact: true,
        })
        .click();
      await page.getByRole("link", { name: "SDK Orders", exact: true }).click();
      await page
        .getByRole("button", {
          name: english ? "New SDK endpoint" : "新建 SDK 接入",
          exact: true,
        })
        .click();
      const form = page.getByRole("dialog", {
        name: english ? "New SDK endpoint" : "新建 SDK 接入",
        exact: true,
      });
      await form
        .getByRole("textbox", {
          name: new RegExp(english ? "Endpoint name" : "接入名称"),
        })
        .fill("Orders OTLP");
      await expect(
        form.getByRole("textbox", {
          name: english ? "Service name" : "服务名称",
          exact: true,
        }),
      ).toHaveCount(0);
      await expect(
        form.getByRole("textbox", {
          name: english ? "Environment" : "环境",
          exact: true,
        }),
      ).toHaveCount(0);
      await form
        .getByRole("button", {
          name: english ? "Choose a Collector" : "选择执行收集器",
          exact: true,
        })
        .click();
      await page
        .getByRole("dialog", {
          name: english ? "Choose a Collector" : "选择执行收集器",
          exact: true,
        })
        .getByRole("button", { name: /^(选择|Select) / })
        .first()
        .click();
      await form
        .getByRole("button", {
          name: english ? "Preview changes" : "预览变更",
          exact: true,
        })
        .click();
      await page
        .getByRole("button", {
          name: english ? "Confirm changes" : "确认变更",
          exact: true,
        })
        .click();
      const endpoint = page
        .getByRole("row")
        .filter({ has: page.getByText("Orders OTLP", { exact: true }) });
      await expect(endpoint).toBeVisible();
      await expect(
        endpoint.getByText(english ? "Waiting for samples" : "等待样本", {
          exact: true,
        }),
      ).toBeVisible();
      await endpoint.getByRole("button", {
        name: english ? "SDK configuration" : "接入配置", exact: true,
      }).click();
      const guide = page.getByRole("dialog", {
        name: `Orders OTLP · ${english ? "SDK configuration" : "接入配置"}`,
        exact: true,
      });
      await expect(guide.getByRole("textbox", {
        name: english ? "TLS server name" : "TLS 服务器名称", exact: true,
      })).toHaveValue(/^collector-.+\.argus\.telemetry$/);
      await expect(guide.getByRole("textbox", {
        name: english ? "Application environment (inherited)" : "应用环境（自动继承）", exact: true,
      })).toHaveAttribute("readonly", "");
      await guide.getByText("HTTP", { exact: true }).click();
      await expect(guide.getByRole("textbox", {
        name: english ? "SDK endpoint" : "SDK 推送地址", exact: true,
      })).toHaveValue(/^https:\/\/collector-.+:24318$/);
      await guide.getByRole("textbox", {
        name: english ? "Example service name" : "示例服务名", exact: true,
      }).fill("orders-api");
      await expect(guide.getByRole("region", {
        name: english ? "SDK configuration example" : "SDK 配置示例", exact: true,
      })).toContainText("OTEL_SERVICE_NAME='orders-api'");
      await expect(guide).toContainText("ARGUS_APPLICATION_INGEST_KEY");
      const guideAccessibility = await new AxeBuilder({ page }).analyze();
      expect(guideAccessibility.violations.filter((v) => v.impact === "serious" || v.impact === "critical")).toEqual([]);
      await page.screenshot({ path: info.outputPath("ingest-connection-guide.png"), fullPage: true });
      await page.keyboard.press("Escape");
      await expect(guide).toHaveCount(0);
      await endpoint
        .getByRole("button", {
          name: english ? "Ingestion keys" : "接入 Key",
          exact: true,
        })
        .click();
      await page
        .getByRole("button", {
          name: english ? "Issue key" : "签发 Key",
          exact: true,
        })
        .click();
      await page
        .getByRole("button", {
          name: english ? "Confirm changes" : "确认变更",
          exact: true,
        })
        .click();
      await page
        .getByRole("button", {
          name: english ? "Reveal key" : "领取 Key",
          exact: true,
        })
        .click();
      const secret = page.getByRole("textbox", {
        name: new RegExp(english ? "SDK write key" : "SDK 写入 Key"),
      });
      await expect(secret).toBeVisible();
      const value = await secret.inputValue();
      expect(value).toMatch(/^argus_app\.[^.]+\.[A-Za-z0-9_-]{43}$/);
      expect(
        await page.evaluate(() => JSON.stringify(localStorage)),
      ).not.toContain(value);
      await page.keyboard.press("Escape");
      await expect(secret).toHaveCount(0);
      await expect(
        page.getByRole("button", {
          name: english ? "Reveal key" : "领取 Key",
          exact: true,
        }),
      ).toHaveCount(0);
      await page
        .getByRole("button", {
          name: english ? "Revoke key" : "撤销 Key",
          exact: true,
        })
        .click();
      await page
        .getByRole("button", {
          name: english ? "Confirm changes" : "确认变更",
          exact: true,
        })
        .click();
      await expect(
        page.getByText(english ? "Revoked" : "已撤销", { exact: true }),
      ).toBeVisible();
      const accessibility = await new AxeBuilder({ page }).analyze();
      expect(
        accessibility.violations.filter(
          (v) => v.impact === "serious" || v.impact === "critical",
        ),
      ).toEqual([]);
      await page.screenshot({
        path: info.outputPath("ingest-key-revoked.png"),
        fullPage: true,
      });
      expect(
        await page.evaluate(() => JSON.stringify(localStorage)),
      ).not.toContain(value);
    });
  }
