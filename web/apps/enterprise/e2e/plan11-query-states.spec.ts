import { expect, test } from "@playwright/test";
import { writeFileSync } from "node:fs";
import {
  dashboardBinding,
  openSealedDashboard,
} from "./helpers/sealed-dashboard";

const cases = [
  {
    id: "empty-metric",
    type: "timeseries",
    signal: "metrics",
    targets: [
      {
        id: "A",
        status: "success",
        result_type: "matrix",
        data: { result: [] },
      },
    ],
  },
  {
    id: "empty-log",
    type: "logs",
    signal: "logs",
    targets: [
      { id: "A", status: "success", result_type: "log_entries", data: [] },
    ],
  },
  {
    id: "empty-trace",
    type: "trace_list",
    signal: "traces",
    targets: [
      {
        id: "A",
        status: "success",
        result_type: "trace_list",
        data: { queryTraces: { total: 0, traces: [] } },
      },
    ],
  },
  {
    id: "empty-span",
    type: "trace_detail",
    signal: "traces",
    targets: [
      {
        id: "A",
        status: "success",
        result_type: "trace_detail",
        data: { queryTrace: { spans: [], edges: [] } },
      },
    ],
  },
  {
    id: "empty-topology",
    type: "apm_topology",
    signal: "traces",
    targets: [
      {
        id: "A",
        status: "success",
        result_type: "apm_topology",
        data: { queryAPMTopology: { nodes: [], edges: [] } },
      },
    ],
  },
  {
    id: "failed-metric",
    type: "timeseries",
    signal: "metrics",
    targets: [
      {
        id: "A",
        status: "error",
        result_type: "matrix",
        data: null,
        code: "QUERY_UNAVAILABLE",
      },
    ],
  },
  {
    id: "budget-trace",
    type: "trace_list",
    signal: "traces",
    targets: [
      {
        id: "A",
        status: "skipped_budget",
        result_type: "trace_list",
        data: null,
        code: "QUERY_BUDGET_EXCEEDED",
      },
    ],
  },
  { id: "never-queried", type: "timeseries", signal: "metrics", targets: [] },
  {
    id: "partial-metric",
    type: "stat",
    signal: "metrics",
    targets: [
      {
        id: "A",
        status: "partial",
        result_type: "vector",
        data: {
          result: [
            {
              metric: { __name__: "observed_requests" },
              value: [1791853200, "12"],
            },
          ],
        },
      },
      {
        id: "B",
        status: "error",
        code: "QUERY_UNAVAILABLE",
        result_type: "vector",
        data: null,
      },
    ],
  },
].map((panel, index) => ({
  ...panel,
  name: panel.id,
  unit: "",
  decimals: 0,
  layout: { x: 0, y: index, w: 12, h: 1 },
}));

for (const english of [false, true])
  for (const dark of [false, true]) {
    test(`sealed query empty error partial ${english ? "en" : "zh"} ${dark ? "dark" : "light"}`, async ({
      page,
    }, info) => {
      await page.setViewportSize({ width: 1440, height: 900 });
      const data = {
        ...dashboardBinding.Data,
        status: "partial",
        panels: cases,
      };
      const { content, frame } = await openSealedDashboard(
        page,
        english,
        dark,
        data,
      );
      await expect(
        content.locator(".argus-template-note--warning"),
      ).toContainText(english ? "Partial result" : "结果不完整");
      await expect(content.locator(".argus-dashboard-panel")).toHaveCount(
        cases.length,
      );
      const panel = (id: string) =>
        content.locator(".argus-dashboard-panel").filter({
          has: content.getByRole("heading", { name: id, exact: true }),
        });
      for (const id of [
        "empty-metric",
        "empty-log",
        "empty-trace",
        "empty-span",
        "empty-topology",
      ]) {
        await expect(panel(id).locator(".argus-observation-empty")).toHaveText(
          english ? "No data in this range" : "当前范围没有数据",
        );
        await expect(panel(id).locator("canvas")).toHaveCount(0);
      }
      for (const [id, code] of [
        ["failed-metric", "QUERY_UNAVAILABLE"],
        ["budget-trace", "QUERY_BUDGET_EXCEEDED"],
      ]) {
        await expect(panel(id).getByRole("status")).toHaveText(
          `${english ? "Query incomplete" : "查询未完成"} · ${code}`,
        );
        await expect(panel(id).locator("canvas")).toHaveCount(0);
      }
      await expect(
        panel("never-queried").locator(".argus-observation-empty"),
      ).toHaveText(english ? "Not queried yet" : "尚未查询");
      const partial = panel("partial-metric");
      await expect(
        partial.locator(".argus-observation-stat strong"),
      ).toHaveText("12");
      const facts = partial.locator("details").filter({
        has: content.locator("summary").filter({
          hasText: english
            ? "Panel parameters and execution"
            : "面板参数与执行信息",
        }),
      });
      await facts.locator("summary").click();
      await expect(facts).toContainText("A / partial");
      await expect(facts).toContainText("B / error");
      await expect(facts).toContainText("QUERY_UNAVAILABLE");
      await expect(content.locator(".argus-template-spinner")).toHaveCount(0);
      await expect(content.locator(".argus-template-parameters")).toContainText(
        "db-prod-01",
      );
      await frame.scrollIntoViewIfNeeded();
      await page.screenshot({ path: info.outputPath("query-states.png") });
      writeFileSync(
        info.outputPath("receipt.json"),
        JSON.stringify(
          {
            passed: true,
            controlled_presentation: true,
            source_hash: dashboardBinding.Hash,
            locale: english ? "en-US" : "zh-CN",
            theme: dark ? "dark" : "light",
            states: cases.map((panel) => panel.id),
            real_collection: false,
          },
          null,
          2,
        ),
      );
    });
    for (const status of ["failed", "cancelled"]) {
      test(`sealed dashboard ${status} ${english ? "en" : "zh"} ${dark ? "dark" : "light"}`, async ({
        page,
      }, info) => {
        const { content } = await openSealedDashboard(page, english, dark, {
          ...dashboardBinding.Data,
          status,
          error_code: "QUERY_UNAVAILABLE",
        });
        await expect(
          content.locator(".argus-template-note--error"),
        ).toContainText(
          status === "failed"
            ? english
              ? "Query failed"
              : "查询失败"
            : english
              ? "Query cancelled"
              : "查询已取消",
        );
        await expect(content.locator(".argus-dashboard-panel")).toHaveCount(0);
        await expect(content.locator("canvas")).toHaveCount(0);
        await expect(content.locator(".argus-template-spinner")).toHaveCount(0);
        await expect(
          content.locator(".argus-template-parameters"),
        ).toContainText("db-prod-01");
        await content
          .getByRole("button", {
            name: english ? "Reload" : "重新加载",
            exact: true,
          })
          .click();
        await expect(
          content.locator(".argus-template-note--error"),
        ).toBeVisible();
        await page.screenshot({ path: info.outputPath(`${status}.png`) });
      });
    }
  }
