import { expect, test } from "@playwright/test";
import { readFileSync } from "node:fs";
import AxeBuilder from "@axe-core/playwright";
import { selectTrigger } from "./helpers/select";
const path = process.env.ARGUS_MODEL_GOVERNANCE_FIXTURE;
const fixture = path
  ? (JSON.parse(readFileSync(path, "utf8")) as {
      api: string;
      token: string;
      csrf: string;
      dashboardId: string;
      modelId: string;
      month: string;
    })
  : undefined;
test.skip(
  !fixture,
  "Requires isolated real API, PostgreSQL, ClickHouse and mTLS metrics service",
);

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`real model governance ${english ? "en" : "zh"} ${theme}`, async ({
      page,
      context,
    }, info) => {
      const tx = (zh: string, en: string) => (english ? en : zh);
      await context.addCookies([
        {
          name: "argus_enterprise_session",
          value: fixture!.token,
          domain: "127.0.0.1",
          path: "/",
          httpOnly: true,
          sameSite: "Strict",
        },
        {
          name: "argus_enterprise_csrf",
          value: fixture!.csrf,
          domain: "127.0.0.1",
          path: "/",
          sameSite: "Strict",
        },
      ]);
      await page.addInitScript(
        ({ english, theme }) => {
          localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
          localStorage.setItem("argus.theme", theme);
        },
        { english, theme },
      );
      await page.setViewportSize({ width: 1440, height: 1000 });
      const errors: string[] = [];
      page.on("pageerror", (error) => errors.push(error.message));
      await page.goto("/settings/aiops?tab=models");
      await expect(
        page.getByRole("button", {
          name: tx("查看治理仪表盘", "View governance dashboard"),
          exact: true,
        }),
      ).toBeVisible();
      await expect(
        page.getByRole("tab", {
          name: tx("治理仪表盘", "Governance dashboard"),
          exact: true,
        }),
      ).toHaveCount(0);
      await expect(
        page.getByRole("tab", { name: tx("模型", "Models"), exact: true }),
      ).toHaveCount(0);
      await page
        .getByRole("button", {
          name: tx("查看治理仪表盘", "View governance dashboard"),
          exact: true,
        })
        .click();
      await expect(page).toHaveURL(
        new RegExp(`/dashboards/${fixture!.dashboardId}`),
      );
      await expect(
        page.getByRole("heading", {
          name: tx("模型治理", "Model governance"),
          exact: true,
        }),
      ).toBeVisible();
      await expect(page.getByRole("button", { name: /复制|Copy/ })).toHaveCount(
        0,
      );
      await page.goto(
        `/dashboards/${fixture!.dashboardId}?model=${fixture!.modelId}&month=${fixture!.month}`,
      );
      await expect(
        page.getByRole("textbox", { name: tx("月份", "Month"), exact: true }),
      ).toHaveValue(fixture!.month);
      await expect(
        page.locator(".argus-dashboard-panel-content").first(),
      ).not.toContainText(/暂无|No data/);
      const snapshot = await page.request.post(
        `${fixture!.api}/dashboards/${fixture!.dashboardId}/execute`,
        {
          data: {
            resource_ids: [],
            variables: {
              model: { all: false, values: [fixture!.modelId] },
              month: { all: false, values: [fixture!.month] },
            },
            local_values: {},
          },
        },
      );
      expect(snapshot.status()).toBe(200);
      const execution = await snapshot.json();
      expect(execution.model_governance.pending).toBe(0);
      if (!execution.model_governance.usage_complete) {
        await expect(
          page.getByText(tx("用量记录不完整", "Incomplete usage"), {
            exact: true,
          }),
        ).toBeVisible();
      }
      const requests = execution.panels.find(
        (panel: { id: string }) => panel.id === "requests",
      );
      expect(Number(requests.targets[0].data[0].value[1])).toBeGreaterThan(0);
      const amount = execution.panels.find(
        (panel: { id: string }) => panel.id === "amount",
      );
      if (Number(amount.targets[0].data[0].value[1]) > 0) {
        await expect
          .poll(async () =>
            Number(
              (
                await page
                  .locator(".argus-dashboard-panel-content")
                  .nth(3)
                  .innerText()
              )
                .replaceAll(",", "")
                .trim(),
            ),
          )
          .toBeGreaterThan(0);
      }
      await page
        .getByRole("button", { name: tx("刷新", "Refresh"), exact: true })
        .click();
      await expect(
        page.getByRole("button", { name: tx("刷新", "Refresh"), exact: true }),
      ).toBeEnabled();
      await selectTrigger(page, tx("模型", "Model")).click();
      await page
        .getByRole("option", {
          name: tx("全部模型", "All models"),
          exact: true,
        })
        .click();
      await expect(
        page.getByRole("button", { name: tx("刷新", "Refresh"), exact: true }),
      ).toBeEnabled();
      await page.screenshot({
        path: info.outputPath("model-governance.png"),
        animations: "disabled",
      });
      const a11y = await new AxeBuilder({ page }).analyze();
      expect(
        a11y.violations.filter((v) =>
          ["serious", "critical"].includes(v.impact ?? ""),
        ),
      ).toEqual([]);
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth > innerWidth,
        ),
      ).toBe(false);
      await page
        .getByRole("link", { name: tx("仪表盘", "Dashboards"), exact: true })
        .click();
      await expect(
        page.getByRole("link", {
          name: tx("模型治理", "Model governance"),
          exact: true,
        }),
      ).toBeVisible();
      expect(errors).toEqual([]);
    });
  }
