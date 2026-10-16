// @vitest-environment jsdom
import { afterEach, expect, it } from "vitest";
import { presentTool } from "./test-support";
afterEach(() => document.body.replaceChildren());
it("bounds the first chart render before the authoritative page arrives", () => {
  const items = Array.from({ length: 350 }, (_, index) => ({
    metric: {
      __name__: "cpu",
      resource_id: "host",
      instance: String(index),
      unit: "%",
    },
    values: [
      [1791810000, "3"],
      [1791810030, "4"],
    ],
  }));
  const charts = presentTool("telemetry.promql.query", {
    items,
    resources: [{ id: "host", name: "host-01" }],
  });
  const targets = charts[0]!.targets as { data: { result: unknown[] } }[];
  expect(targets[0]!.data.result).toHaveLength(25);
  expect(items).toHaveLength(350);
});
it("renders named instant values with explicit unknown units and no trend", () => {
  const charts = presentTool("telemetry.promql.query", {
    query_context: { query: "cpu * 100" },
    resources: [{ name: "argus-test-host" }],
    data: {
      resultType: "vector",
      result: [{ metric: { __name__: "cpu" }, value: [1791810000, "3.5234"] }],
    },
  });
  expect(charts).toHaveLength(0);
  expect(document.body.textContent).toContain("argus-test-host");
  expect(document.body.textContent).toContain("cpu * 100");
  expect(document.body.textContent).toContain("3.523");
  expect(document.body.textContent).toContain("单位");
  expect(document.body.textContent).toContain("未识别");
  expect(document.body.textContent).toContain("单点");
});
it("separates host/database relationships and reliable units in chart targets", () => {
  const charts = presentTool("telemetry.promql.query");
  expect(charts).toHaveLength(1);
  expect(charts[0]!.unit).toBe("connections");
  expect(charts[0]!.legend).toBe(true);
  expect(document.body.textContent).toContain("database=orders");
  expect(document.body.textContent).toContain("database=warehouse");
  expect(document.body.textContent).toContain("db-prod-01");
  expect(document.body.textContent).toContain("orders-production");
});
it("does not infer health from an empty query and displays partial completeness", () => {
  presentTool("telemetry.promql.query", { items: [], partial: true });
  expect(document.body.textContent).toContain("没有数据");
  expect(document.body.textContent).toContain("不完整");
});
it("uses the sealed latest value and count rather than the first preview points", () => {
  presentTool("telemetry.promql.query", {
    items: [
      {
        metric: { __name__: "cpu", unit: "%" },
        values: [
          [1, "1"],
          [999, "49"],
        ],
        argus_statistics: {
          last: 49,
          min: 0,
          max: 999,
          mean: 25,
          sample_count: 1000,
        },
      },
    ],
  });
  expect(document.body.textContent).toContain("49 %");
  expect(document.body.textContent).toContain("1000");
});
