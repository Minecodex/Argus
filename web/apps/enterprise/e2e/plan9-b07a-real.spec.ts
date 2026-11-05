import { readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { expect, test, type Page } from "@playwright/test";

test.use({ trace: "off", video: "off", reducedMotion: "reduce" });
const batch = (["B07a", "B07b", "B07c"] as const).find(
  (value) => process.env[`ARGUS_PLAN9_${value.toUpperCase()}_E2E`] === "1",
);
type ResourceMap = {
  applications: { orders: { id: string; name: string } };
  batch_dashboard: { id: string; name: string };
  dashboards: { redis: { id: string } };
  targets: { redis: { target_id: string; name: string } };
  alert_policies: Record<string, { id: string; name: string }>;
};
const mapping = () =>
  JSON.parse(
    readFileSync(process.env.ARGUS_PLAN9_RESOURCE_MAP!, "utf8"),
  ) as ResourceMap;
async function fits(page: Page) {
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
  ).toBe(true);
}
function businessRequest() {
  return JSON.parse(
    execFileSync(
      "python",
      [
        "-c",
        "import sys,json;sys.path.insert(0,'docs/plan9/runtime/B07a');from manage import Run;print(json.dumps(Run(batch=sys.argv[1]).request('browser')))",
        batch!,
      ],
      {
        cwd: process.env.ARGUS_PLAN9_RUNTIME_ROOT,
        encoding: "utf8",
        timeout: 20000,
      },
    ),
  ) as { request_id: string; status: number; response: { trace_id: string } };
}

test.describe(`Plan9 ${batch} real SDK application`, () => {
  test.skip(!batch, "requires a normally enrolled and running B07 application");
  test.describe.configure({ timeout: 180000 });
  test.beforeEach(async ({ context, page }) => {
    page.setDefaultTimeout(15000);
    const session = JSON.parse(
      readFileSync(process.env.ARGUS_PLAN9_SESSION_PATH!, "utf8"),
    );
    await context.addCookies(
      session.cookies.map((cookie: object) => ({
        ...cookie,
        httpOnly: true,
        sameSite: "Lax",
      })),
    );
  });
  for (const locale of ["zh-CN", "en-US"])
    for (const theme of ["light", "dark"]) {
      test(`${locale} ${theme}: application, three signals, dashboards and alerts`, async ({
        page,
      }, info) => {
        const map = mapping(),
          app = map.applications.orders;
        const text = (zh: string, en: string) => (locale === "zh-CN" ? zh : en);
        const errors: string[] = [];
        page.on("pageerror", (error) => errors.push(error.message));
        await page.setViewportSize({ width: 1440, height: 1050 });
        await page.addInitScript(
          ({ locale, theme }) => {
            localStorage.setItem("argus.locale", locale);
            localStorage.setItem("argus.theme", theme);
          },
          { locale, theme },
        );
        const inventory = await page.request.get(
          "/api/v1/monitoring/collections?limit=100",
        );
        expect(inventory.status()).toBe(200);
        const retired = JSON.parse(
          readFileSync(process.env.ARGUS_PLAN9_RETIRED_MAP!, "utf8"),
        ) as {
          targets: Record<string, { target_id: string }>;
          hosts: Record<string, { host_id: string }>;
        };
        const oldIds = [
          ...Object.values(retired.targets).map((x) => x.target_id),
          ...Object.values(retired.hosts).map((x) => x.host_id),
        ];
        expect(
          (await inventory.json()).items.filter(
            (x: { target: { id: string } }) => oldIds.includes(x.target.id),
          ),
        ).toEqual([]);
        await page.goto("/applications");
        const apm = (root: string) =>
          page.waitForResponse(
            (r) =>
              r.url().endsWith("/enterprise/traces/graphql") &&
              r.request().method() === "POST" &&
              String(r.request().postDataJSON()?.query).includes(root),
          );
        const overview = [apm("queryAPMRED"), apm("queryAPMTopology")];
        await page.getByRole("link", { name: app.name, exact: true }).click();
        await expect(page).toHaveURL(new RegExp(`/applications/${app.id}`));
        const region = page.getByRole("region", {
          name: text("应用性能", "Application performance"),
          exact: true,
        });
        const tab = (zh: string, en: string) =>
          region.getByRole("tab", { name: text(zh, en), exact: true });
        await expect(region.getByRole("tab")).toHaveCount(5);
        const [red, topology] = await Promise.all(overview);
        expect(red!.status()).toBe(200);
        expect(topology!.status()).toBe(200);
        const trend = (await red!.json()).data.queryAPMRED;
        expect(trend.limited).toBe(false);
        expect(trend.rows.length).toBeGreaterThan(0);
        const graph = (await topology!.json()).data.queryAPMTopology;
        expect(graph.nodes).toHaveLength(2);
        expect(graph.edges).toHaveLength(1);
        await expect(
          region.locator(".argus-observation-chart canvas"),
        ).toHaveCount(2);
        await fits(page);
        await page.screenshot({
          path: info.outputPath(`application-${locale}-${theme}.png`),
          fullPage: true,
          animations: "disabled",
        });
        await tab("服务", "Services").click();
        const registry = region.getByRole("region", {
          name: text("服务", "Services"),
          exact: true,
        });
        await expect(registry.getByRole("table")).toContainText(
          "orders-gateway",
        );
        await expect(registry.getByRole("table")).toContainText(
          "orders-worker",
        );
        const serviceDetails = [
          apm("queryAPMInstances"),
          apm("queryAPMEndpoints"),
        ];
        await registry
          .getByRole("button", { name: "orders-gateway", exact: true })
          .click();
        for (const response of await Promise.all(serviceDetails)) {
          expect(response.status()).toBe(200);
          const view = Object.values((await response.json()).data) as {
            rows: unknown[];
          }[];
          expect(view[0]!.rows.length).toBeGreaterThan(0);
        }
        await region
          .getByRole("button", {
            name: text("清除服务筛选", "Clear service filter"),
            exact: true,
          })
          .click();
        const firstPage = apm("queryTraces");
        await tab("调用链", "Traces").click();
        const first = await firstPage;
        expect(first.status()).toBe(200);
        const firstData = (await first.json()).data.queryTraces;
        expect(firstData.traces.length).toBeGreaterThan(0);
        if (firstData.total > 50) {
          const secondPage = apm("queryTraces");
          await region
            .getByRole("button", {
              name: text("下一页", "Next page"),
              exact: true,
            })
            .click();
          const second = await secondPage;
          expect(second.status()).toBe(200);
          const secondData = (await second.json()).data.queryTraces;
          expect(second.request().postDataJSON().variables.pageNum).toBe(2);
          expect(second.request().postDataJSON().time_range).toEqual(
            first.request().postDataJSON().time_range,
          );
          expect(secondData.total).toBe(firstData.total);
          expect(secondData.traces.length).toBeGreaterThan(0);
          expect(secondData.traces[0].traceId).not.toBe(
            firstData.traces[0].traceId,
          );
        }
        await tab("SDK 接入", "SDK ingestion").click();
        await expect(
          region.getByRole("button", {
            name: text("时间范围", "Time range"),
            exact: true,
          }),
        ).toHaveCount(0);
        await expect(region.getByRole("table")).toContainText(
          batch === "B07c" ? "JAEGER" : "OTLP",
        );
        const receipt = businessRequest();
        expect(receipt.status).toBe(200);
        // Wait for the actual SDK log, then follow the product's log-to-trace link.
        const loadedLogs = page.waitForResponse(
          (r) =>
            r.url().endsWith("/enterprise/logs/query") &&
            r.request().method() === "POST",
        );
        await tab("日志", "Logs").click();
        expect((await loadedLogs).status()).toBe(200);
        await region
          .getByRole("textbox", {
            name: text("搜索当前结果", "Search current results"),
            exact: true,
          })
          .fill(receipt.request_id);
        const logRow = region
          .getByRole("article")
          .filter({ hasText: receipt.request_id })
          .first();
        for (
          let attempt = 0;
          attempt < 15 && (await logRow.count()) === 0;
          attempt++
        ) {
          await page.waitForTimeout(1000);
          const refreshed = page.waitForResponse(
            (r) =>
              r.url().endsWith("/enterprise/logs/query") &&
              r.request().method() === "POST",
          );
          await region
            .getByRole("button", {
              name: text("刷新数据", "Refresh data"),
              exact: true,
            })
            .click();
          expect((await refreshed).status()).toBe(200);
        }
        await expect(logRow).toBeVisible();
        const traceResponse = () =>
          page.waitForResponse(
            (r) =>
              r.url().endsWith("/enterprise/traces/graphql") &&
              r.request().method() === "POST" &&
              String(r.request().postDataJSON()?.query).includes("queryTraces"),
          );
        let loadedTrace = traceResponse();
        await logRow
          .getByRole("button", {
            name: text("关联查询", "Related queries"),
            exact: true,
          })
          .click();
        let traceData = (await (await loadedTrace).json()).data.queryTraces
          .traces;
        for (
          let attempt = 0;
          attempt < 10 &&
          !traceData.some(
            (t: { traceId: string; spanCount: number }) =>
              t.traceId === receipt.response.trace_id && t.spanCount >= 4,
          );
          attempt++
        ) {
          await page.waitForTimeout(1000);
          loadedTrace = traceResponse();
          await region
            .getByRole("button", {
              name: text("刷新数据", "Refresh data"),
              exact: true,
            })
            .click();
          traceData = (await (await loadedTrace).json()).data.queryTraces
            .traces;
        }
        expect(
          traceData.some(
            (t: { traceId: string; spanCount: number }) =>
              t.traceId === receipt.response.trace_id && t.spanCount >= 4,
          ),
        ).toBe(true);
        const traceRow = region
          .getByRole("row")
          .filter({ hasText: receipt.response.trace_id })
          .first();
        await expect(traceRow).toBeVisible();
        await traceRow
          .getByRole("button", {
            name: text("打开详情", "Open details"),
            exact: true,
          })
          .click();
        const detail = page.getByRole("dialog", {
          name: text("调用链详情", "Trace details"),
          exact: true,
        });
        await expect(detail).toContainText(receipt.response.trace_id);
        await expect(detail).toContainText("GET /orders");
        await expect(detail).toContainText("GET /work");
        await expect(detail).toContainText("Redis SET/GET");
        await page.screenshot({
          path: info.outputPath(`trace-${locale}-${theme}.png`),
          fullPage: true,
          animations: "disabled",
        });
        await page.keyboard.press("Escape");
        if (locale === "zh-CN" && theme === "light") {
          await region
            .getByRole("button", { name: "时间范围", exact: true })
            .click();
          const picker = page.getByRole("dialog", {
            name: "时间范围",
            exact: true,
          });
          await picker.getByRole("button", { name: /时间范围类型/ }).click();
          await page
            .getByRole("option", { name: "固定时间范围", exact: true })
            .click();
          await expect(
            picker.getByText("开始时间", { exact: false }),
          ).toBeVisible();
          const fixedQuery = apm("queryTraces");
          await picker
            .getByRole("button", { name: "应用", exact: true })
            .click();
          const fixed = await fixedQuery;
          expect(fixed.status()).toBe(200);
          const fixedRange = fixed.request().postDataJSON().time_range;
          const fixedLogs = page.waitForResponse(
            (r) =>
              r.url().endsWith("/enterprise/logs/query") &&
              r.request().method() === "POST",
          );
          await tab("日志", "Logs").click();
          expect((await fixedLogs).request().postDataJSON().time_range).toEqual(
            fixedRange,
          );
          const refreshLogs = page.waitForResponse(
            (r) =>
              r.url().endsWith("/enterprise/logs/query") &&
              r.request().method() === "POST",
          );
          await region
            .getByRole("button", { name: "刷新数据", exact: true })
            .click();
          expect(
            (await refreshLogs).request().postDataJSON().time_range,
          ).toEqual(fixedRange);
        }
        for (const board of [
          {
            id: map.batch_dashboard.id,
            resource: app.id,
            kind: "application",
            count: 10,
            name: "application-board",
          },
          {
            id: map.dashboards.redis.id,
            resource: map.targets.redis.target_id,
            kind: "monitoring_target",
            count: 11,
            name: "redis-board",
          },
        ]) {
          const loaded = page.waitForResponse(
            (r) =>
              r.url().includes(`/dashboards/${board.id}/execute`) &&
              r.request().method() === "POST",
          );
          await page.goto(
            `/dashboards/${board.id}?resource=${board.resource}&resource_type=${board.kind}`,
          );
          const response = await loaded;
          expect(response.status()).toBe(200);
          const data = await response.json();
          expect(data.panels).toHaveLength(board.count);
          expect(
            data.panels.map((p: { id: string; status: string }) => ({
              id: p.id,
              status: p.status,
            })),
            `actual panel states for ${board.name}`,
          ).toEqual(
            data.panels.map((p: { id: string }) => ({
              id: p.id,
              status: "success",
            })),
          );
          await expect(page.locator("[data-panel-id]")).toHaveCount(
            board.count,
          );
          await fits(page);
          await page.screenshot({
            path: info.outputPath(`${board.name}-${locale}-${theme}.png`),
            fullPage: true,
            animations: "disabled",
          });
          await page.locator("[data-panel-id]").last().scrollIntoViewIfNeeded();
          await page.screenshot({
            path: info.outputPath(
              `${board.name}-bottom-${locale}-${theme}.png`,
            ),
            fullPage: true,
            animations: "disabled",
          });
        }
        const policy = map.alert_policies.errors;
        await page.goto(`/alerts?tab=history&policy_id=${policy.id}`);
        await page.getByText(policy.name, { exact: true }).first().click();
        const alert = page.getByRole("dialog", { name: policy.name });
        await expect(alert.locator(".argus-chart svg")).toBeVisible();
        await alert.getByRole("tab", { name: /^时间线$|^Timeline$/ }).click();
        await expect(
          alert.locator(".argus-alert-timeline li").first(),
        ).toBeVisible();
        await expect(alert.locator(".argus-alert-timeline pre")).toHaveCount(0);
        await page.screenshot({
          path: info.outputPath(`alert-${locale}-${theme}.png`),
          fullPage: true,
          animations: "disabled",
        });
        expect(errors).toEqual([]);
      });
    }
  for (const width of [1280, 1920])
    test(`application dashboard fits ${width}`, async ({ page }, info) => {
      const map = mapping(),
        board = map.batch_dashboard;
      await page.setViewportSize({ width, height: 1050 });
      const loaded = page.waitForResponse(
        (r) =>
          r.url().includes(`/dashboards/${board.id}/execute`) &&
          r.request().method() === "POST",
      );
      await page.goto(
        `/dashboards/${board.id}?resource=${map.applications.orders.id}&resource_type=application`,
      );
      expect((await loaded).status()).toBe(200);
      await expect(page.locator("[data-panel-id]")).toHaveCount(10);
      await fits(page);
      await page.screenshot({
        path: info.outputPath(`application-dashboard-${width}.png`),
        fullPage: true,
        animations: "disabled",
      });
    });
});
