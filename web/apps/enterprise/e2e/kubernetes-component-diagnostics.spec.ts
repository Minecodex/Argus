import { expect, test } from "@playwright/test";

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`component scrape diagnosis ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }) => {
      test.setTimeout(60_000);
      const text = (zh: string, en: string) => (english ? en : zh);
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
        has: page.getByText(text("Scheduler 指标", "Scheduler metrics"), {
          exact: true,
        }),
      });
      await scheduler.getByRole("switch").focus();
      await scheduler.getByRole("switch").press("Space");
      const confirmation = page.getByRole("dialog", {
        name: text("自身监控配置预览", "Preview own resource monitoring"),
        exact: true,
      });
      await confirmation
        .getByRole("button", { name: /^(确认|Confirm)/ })
        .click();
      await expect(confirmation).not.toBeVisible();
      await expect(scheduler.getByRole("switch")).toBeChecked();
      const failure = text(
        "组件端点抓取失败，请检查端点、证书与读取权限",
        "Component scrape failed; check endpoint, certificate and read access",
      );
      for (const enabled of [true, false]) {
        await page.evaluate((enabled) => {
          const key = "argus-mock:db-v14";
          const db = JSON.parse(localStorage.getItem(key)!) as {
            monitoring: Record<
              string,
              { collections?: Array<{ integration: string }> }
            >;
          };
          const row = Object.values(db.monitoring)
            .flatMap((state) => state.collections ?? [])
            .find(
              (item) => item.integration === "kubernetes_scheduler_metrics",
            );
          if (!row)
            throw new Error("Expected the configured scheduler collection");
          Object.assign(row, {
            desired_enabled: enabled,
            apply_status: "effective",
            health: "failed",
            freshness: "stale",
            error_code: "COLLECTION_SCRAPE_FAILED",
          });
          localStorage.setItem(key, JSON.stringify(db));
        }, enabled);
        await page.reload();
        if (enabled) {
          await expect(scheduler).toContainText(text("失败", "Failed"));
          await expect(
            scheduler.getByText(failure, { exact: true }),
          ).toBeVisible();
        } else {
          await expect(scheduler.getByRole("switch")).not.toBeChecked();
          await expect(
            scheduler.getByText(text("未开启", "Off"), { exact: true }),
          ).toBeVisible();
          await expect(
            scheduler.getByText(failure, { exact: true }),
          ).toHaveCount(0);
          await expect(
            scheduler.getByText(text("失败", "Failed"), { exact: true }),
          ).toHaveCount(0);
        }
      }
    });
  }
