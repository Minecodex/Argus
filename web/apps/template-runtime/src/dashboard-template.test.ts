// @vitest-environment jsdom
import { afterEach, expect, it } from "vitest";
import { fixtures, presentTool } from "./test-support";
afterEach(() => document.body.replaceChildren());
const data = fixtures.bindings.find(
  (row) => row.Tool === "telemetry.dashboard.query",
)!.Data;
it("keeps frozen parameters above charts and preserves every chart option", () => {
  const charts = presentTool("telemetry.dashboard.query");
  expect(charts).toHaveLength(11);
  const params = document.querySelector(".argus-template-parameters")!,
    grid = document.querySelector(".argus-dashboard-grid")!;
  expect(params.textContent).toContain("db-prod-01");
  expect(params.textContent).toContain("Asia/Shanghai");
  expect(params.textContent).toContain("orders");
  expect(
    params.compareDocumentPosition(grid) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
  for (const chart of charts) {
    expect(chart.range).toEqual({ from: data.from, to: data.to });
    expect(chart.decimals).toBe(1);
    expect(chart.legend).toBe(true);
    expect(chart.unit).toBe("connections");
    expect(chart.thresholds).toEqual([{ value: 17, tone: "warning" }]);
  }
  expect(charts.map((chart) => chart.type)).toContain("histogram");
  expect(JSON.stringify(charts)).toContain("db-prod-01 · orders · orders-api");
  expect(JSON.stringify(charts)).toContain(
    "orders-production · orders · orders-api",
  );
  expect(JSON.stringify(data)).not.toContain("display_name");
});
it("keeps effective parameters on failure, without charts or endless spinner", () => {
  const charts = presentTool(
    "telemetry.dashboard.query",
    { ...data, status: "failed", error_code: "QUERY_UNAVAILABLE" },
    "en-US",
  );
  expect(charts).toHaveLength(0);
  expect(document.body.textContent).toContain("db-prod-01");
  expect(document.body.textContent).toContain("Query failed");
  expect(document.body.textContent).toContain("Reload");
  expect(document.querySelector(".argus-template-spinner")).toBeNull();
});
it("renders a corrected window independently and retains historical input", () => {
  presentTool("telemetry.dashboard.query", data);
  const original = document.querySelector(
    ".argus-template-parameters",
  )!.textContent;
  presentTool("telemetry.dashboard.query", {
    ...data,
    from: "2026-10-13T00:30:00Z",
  });
  expect(
    document.querySelector(".argus-template-parameters")!.textContent,
  ).not.toBe(original);
  presentTool("telemetry.dashboard.query", data);
  expect(
    document.querySelector(".argus-template-parameters")!.textContent,
  ).toBe(original);
});
