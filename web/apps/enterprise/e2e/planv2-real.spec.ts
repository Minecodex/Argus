import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { createMfaLogin } from "./helpers/mfa-login";

test.use({ actionTimeout: 10000, navigationTimeout: 30000 });

const enabled = process.env.ARGUS_PLANV2_E2E === "1";
const dashboardId = process.env.ARGUS_PLANV2_DASHBOARD_ID ?? "";
const clusterId = process.env.ARGUS_PLANV2_CLUSTER_ID ?? "";
const login = createMfaLogin("enterprise");
for (const locale of ["zh-CN", "en-US"])
  for (const theme of ["light", "dark"]) {
    const tx = (zh: string, en: string) => (locale === "zh-CN" ? zh : en);
    test.describe(`PlanV2 real self-monitoring ${locale} ${theme}`, () => {
      test.skip(!enabled, "PlanV2 Kubernetes environment is not active");
      test.describe.configure({ mode: "serial", timeout: 180000 });
      test.beforeEach(async ({ page }) => {
        await page.addInitScript(
          ({ locale, theme }) => {
            localStorage.setItem("argus.locale", locale);
            localStorage.setItem("argus.theme", theme);
          },
          { locale, theme },
        );
        await login(
          page,
          "/login",
          process.env.ARGUS_PLANV2_USERNAME ?? "",
          process.env.ARGUS_PLANV2_PASSWORD ?? "",
        );
      });

      test("shows real OTLP and native SDK data, frozen drilldowns and variable dependencies", async ({
        page,
      }, testInfo) => {
        const executed = page.waitForResponse(
          (r) =>
            r.url().includes(`/dashboards/${dashboardId}/execute`) &&
            r.status() === 200,
        );
        await page.goto(`/dashboards/${dashboardId}?resource=${clusterId}`);
        const data = await (await executed).json();
        expect(data.panels).toHaveLength(18);
        expect(
          data.panels.every((p: { status: string }) => p.status === "success"),
        ).toBe(true);
        expect(JSON.stringify(data)).not.toContain("mock-dashboard");
        await expect(page.locator("[data-panel-id]")).toHaveCount(18);
        for (const [source, service] of [
          ["otlp", "argus-server"],
          ["skywalking", "argus-selfcheck-skywalking"],
          ["jaeger", "argus-selfcheck-jaeger"],
        ]) {
          await expect(
            page.locator(`[data-panel-id="${source}_services"]`),
          ).toContainText(service!, { timeout: 20000 });
          await expect(
            page
              .locator(`[data-panel-id="${source}_services"]`)
              .getByRole("columnheader", {
                name: tx("样本错误率", "Sample error rate"),
                exact: true,
              }),
          ).toBeVisible();
          await page
            .locator(`[data-panel-id="${source}_services"]`)
            .screenshot({
              path: testInfo.outputPath(`${source}-services.png`),
              animations: "disabled",
            });
        }
        await page
          .getByRole("button", {
            name: tx("数据时间与完整性", "Data time and completeness"),
            exact: true,
          })
          .first()
          .click();
        const freshness = page.getByRole("dialog", {
          name: tx("数据时间与完整性", "Data time and completeness"),
        });
        await expect(freshness.locator("time")).toHaveCount(2);
        await expect(freshness).toContainText(
          tx("摄入完整性: 未知", "Ingestion completeness: Unknown"),
        );
        await page.keyboard.press("Escape");

        await page
          .getByRole("button", { name: "Environment", exact: true })
          .click();
        const choices = page.getByRole("dialog", {
          name: "Environment",
          exact: true,
        });
        await choices
          .getByRole("checkbox", { name: tx("全部", "All"), exact: true })
          .check();
        const changed = page.waitForResponse(
          (r) =>
            r.url().includes(`/dashboards/${dashboardId}/execute`) &&
            r.status() === 200,
        );
        await choices
          .getByRole("button", { name: tx("应用", "Apply"), exact: true })
          .click();
        const request = (await changed).request().postDataJSON();
        expect(request.panel_ids).toHaveLength(6);
        expect(
          request.panel_ids.every((id: string) => id.startsWith("otlp_")),
        ).toBe(true);

        const trace = page.locator('[data-panel-id="jaeger_trace_list"]');
        await trace
          .getByRole("button", {
            name: tx("打开详情", "Open details"),
            exact: true,
          })
          .first()
          .click();
        const drill = page.getByRole("dialog").last();
        const buttons = drill
          .getByRole("button")
          .filter({ hasText: /trace|链路/i });
        await expect(buttons.first()).toBeVisible();
        const fetched = page.waitForResponse(
          (r) =>
            r.url().includes(`/dashboards/${dashboardId}/drilldown`) &&
            r.status() === 200,
        );
        await buttons.first().click();
        const detail = await (await fetched).json();
        expect(detail.result.status).toBe("success");
        expect(
          detail.sources.every((s: { type: string }) => s.type === "jaeger"),
        ).toBe(true);
        await page.screenshot({
          path: testInfo.outputPath("native-trace-detail.png"),
          fullPage: true,
          animations: "disabled",
        });
        await page.keyboard.press("Escape");
        await page
          .getByRole("heading", { name: "Argus self monitoring", exact: true })
          .scrollIntoViewIfNeeded();
        await page.screenshot({
          path: testInfo.outputPath("self-monitoring.png"),
          fullPage: true,
          animations: "disabled",
        });
        await accessible(page);
      });

      test("recovers a real personal draft, samples and publishes one reviewed revision", async ({
        page,
      }, testInfo) => {
        await page.goto(`/dashboards/${dashboardId}?resource=${clusterId}`);
        await page
          .getByRole("button", { name: tx("编辑", "Edit"), exact: true })
          .click();
        await expect(page).toHaveURL(/dashboard-drafts/);
        const description = `Native self monitoring verified ${locale} ${theme}`;
        await page
          .getByRole("button", {
            name: tx("基本信息", "Basic information"),
            exact: true,
          })
          .click();
        await page.getByLabel(tx("描述", "Description")).fill(description);
        const saved = page.waitForResponse(
          (r) =>
            r.request().method() === "PATCH" &&
            r.url().includes("/dashboard-drafts/"),
          { timeout: 30000 },
        );
        await page
          .getByRole("dialog")
          .getByRole("button", { name: tx("完成", "Done"), exact: true })
          .click();
        const savedResponse = await saved;
        expect(savedResponse.status()).toBe(200);
        expect((await savedResponse.json()).description).toBe(description);
        await expect(
          page.getByText(tx("草稿已保存", "Draft saved"), { exact: false }),
        ).toBeVisible({ timeout: 15000 });
        await page.reload();
        await page
          .getByRole("button", {
            name: tx("基本信息", "Basic information"),
            exact: true,
          })
          .click();
        await expect(page.getByLabel(tx("描述", "Description"))).toHaveValue(
          description,
        );
        await page.keyboard.press("Escape");
        const sample = page.waitForResponse(
          (r) => r.url().includes("/sample"),
          { timeout: 30000 },
        );
        await page
          .getByRole("button", {
            name: tx("运行样本", "Run sample"),
            exact: true,
          })
          .click();
        const sampleResponse = await sample;
        expect(sampleResponse.status()).toBe(200);
        expect((await sampleResponse.json()).validation.valid).toBe(true);
        await page
          .getByRole("button", {
            name: tx("预览并发布", "Preview and publish"),
            exact: true,
          })
          .click();
        const review = page.getByRole("dialog", {
          name: tx("发布预览", "Publication preview"),
          exact: true,
        });
        await expect(review).toContainText(description);
        await review
          .getByRole("button", {
            name: tx("确认发布", "Confirm publication"),
            exact: true,
          })
          .click();
        await expect(page).toHaveURL(new RegExp(`/dashboards/${dashboardId}`));
        await expect(
          page.getByText(description, { exact: false }),
        ).toBeVisible();
        await page
          .getByRole("button", {
            name: tx("历史版本", "Revision history"),
            exact: true,
          })
          .click();
        const history = page.getByRole("dialog", {
          name: tx("历史版本", "Revision history"),
          exact: true,
        });
        await expect(history.getByText(/^R1 ·/)).toBeVisible();
        await expect(
          history
            .getByRole("button", {
              name: tx("查看此版本", "View this revision"),
              exact: true,
            })
            .first(),
        ).toBeVisible();
        await page.screenshot({
          path: testInfo.outputPath("real-revision-history.png"),
          fullPage: true,
          animations: "disabled",
        });
        await page.keyboard.press("Escape");
      });
    });
  }
async function accessible(page: Page) {
  const report = await new AxeBuilder({ page }).analyze();
  expect(
    report.violations.filter(
      (v) => v.impact === "serious" || v.impact === "critical",
    ),
  ).toEqual([]);
}
