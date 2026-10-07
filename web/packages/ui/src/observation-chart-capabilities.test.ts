import { expect, it } from "vitest";
import {
  metricChartTypes,
  observationChartCapabilities,
  type ChartTarget,
} from "./observation-chart-capabilities";
const snapshot: ChartTarget = {
  id: "q",
  status: "success",
  result_type: "vector",
  data: { result: [{ metric: { service: "orders" }, value: [1, "2"] }] },
};
const range: ChartTarget = {
  ...snapshot,
  result_type: "matrix",
  data: {
    result: [
      {
        metric: { service: "orders" },
        values: [
          [1, "2"],
          [2, "3"],
        ],
      },
    ],
  },
};
const choices = (
  targets: ChartTarget[],
  mode = "instant",
  reducer?: "last" | "min",
) =>
  observationChartCapabilities(
    "metrics",
    metricChartTypes,
    targets,
    targets.map((t) => ({ id: t.id, query_mode: mode })),
    { reducer },
  );
it("classifies all eleven charts using the actual snapshot and range data", () => {
  const instant = choices([snapshot]);
  expect(instant).toHaveLength(11);
  expect(
    instant.filter((c) => c.state === "recommended").map((c) => c.type),
  ).toEqual(["stat", "bar", "table"]);
  expect(instant.find((c) => c.type === "timeseries")).toMatchObject({
    state: "incompatible",
    reason: "rangeRequired",
  });
  expect(
    choices([range], "range").find((c) => c.type === "timeseries")?.state,
  ).toBe("recommended");
});
it("checks every target instead of recommending from only the first query", () => {
  expect(
    choices([range, { ...snapshot, id: "second" }], "range").find(
      (c) => c.type === "heatmap",
    )?.state,
  ).toBe("incompatible");
  expect(
    choices([range, { ...range, id: "second", status: "error" }], "range").some(
      (c) => c.state === "recommended",
    ),
  ).toBe(false);
});
it("requires valid cumulative buckets without merging different label identities", () => {
  const buckets = {
    ...snapshot,
    data: {
      result: [
        { metric: { le: "1" }, value: [1, "2"] },
        { metric: { le: "+Inf" }, value: [1, "3"] },
      ],
    },
  };
  expect(choices([buckets]).find((c) => c.type === "histogram")?.state).toBe(
    "recommended",
  );
  expect(
    choices([buckets, { ...buckets, id: "other" }]).find(
      (c) => c.type === "histogram",
    )?.state,
  ).toBe("incompatible");
});
it("checks negative pie values using the same reducer as rendering", () => {
  const target = {
    ...range,
    data: {
      result: [
        {
          metric: {},
          values: [
            [1, "-2"],
            [2, "3"],
          ],
        },
      ],
    },
  };
  expect(
    choices([target], "range", "min").find((c) => c.type === "pie"),
  ).toMatchObject({ state: "incompatible", reason: "negativeSlices" });
  expect(
    choices([target], "range", "last").find((c) => c.type === "pie")?.state,
  ).toBe("compatible");
});
it("keeps unrun or empty results unverified and validates logs and trace shapes", () => {
  expect(
    observationChartCapabilities(
      "metrics",
      metricChartTypes,
      undefined,
      [],
    ).every((c) => c.state === "unverified"),
  ).toBe(true);
  const trace = observationChartCapabilities(
    "traces",
    ["trace_list", "trace_detail"],
    [{ id: "q", status: "success", result_type: "traces" }],
    [{ id: "q" }],
  );
  expect(trace.map((c) => c.state)).toEqual(["recommended", "incompatible"]);
  const logs = observationChartCapabilities(
    "logs",
    ["logs", "table", "timeseries"],
    [
      {
        id: "q",
        status: "success",
        result_type: "log_entries",
        data: { rows: [{ body: "hello" }] },
      },
    ],
    [{ id: "q" }],
  );
  expect(logs.map((c) => c.state)).toEqual([
    "recommended",
    "recommended",
    "incompatible",
  ]);
});
