import { expect, it } from "vitest";
import { histogramObservations, metricObservations } from "./observation-data";
import {
  observationTone,
  reduceObservation,
  resolveObservationColors,
} from "./observation-display";
import { metricOption } from "./metric-options";
import { themeObservation } from "./observation-theme";

it("reduces returned samples without merging sources, filling missing values or inventing a counter increase", () => {
  const series = metricObservations([
    {
      metric: { service: "same", argus_source_id: "a" },
      values: [
        [1, "2"],
        [2, "4"],
        [100, "NaN"],
      ],
    },
    {
      metric: { service: "same", argus_source_id: "b" },
      values: [
        [1, "100"],
        [100, "NaN"],
      ],
    },
  ]);
  expect(series.map((s) => reduceObservation(s))).toEqual([null, null]);
  expect(series.map((s) => reduceObservation(s, "mean"))).toEqual([3, 100]);
  expect(
    ["min", "max", "sum"].map((r) =>
      reduceObservation(series[0]!, r as "min" | "max" | "sum"),
    ),
  ).toEqual([2, 4, 6]);
  const missing = metricObservations([{ values: [[1, "NaN"]] }])[0]!;
  for (const r of ["last", "mean", "min", "max", "sum"] as const)
    expect(reduceObservation(missing, r)).toBeNull();
});

it("applies inclusive raw-value thresholds and resolves token colors without dropping callbacks", () => {
  const thresholds = [
    { value: 0.5, tone: "warning" },
    { value: 0.9, tone: "danger" },
  ] as const;
  expect(
    [null, 0.49, 0.5, 0.899, 0.9].map((v) =>
      observationTone(v, [...thresholds]),
    ),
  ).toEqual([undefined, undefined, "warning", "warning", "danger"]);
  const formatter = () => "value";
  expect(
    resolveObservationColors(
      { color: "var(--warning)", nested: [{ formatter }] },
      (t) => `resolved:${t}`,
    ),
  ).toEqual({ color: "resolved:--warning", nested: [{ formatter }] });
});

it("keeps data gaps and query samples intact when applying area, stack, bounds and threshold lines", () => {
  const series = metricObservations([
    {
      values: [
        [1, "0.2"],
        [2, "NaN"],
        [3, "0.8"],
      ],
    },
  ]);
  const before = structuredClone(series);
  const option = metricOption(
    "timeseries",
    series,
    false,
    "percent_ratio",
    1,
    { min: 0, max: 1, draw_style: "area", stack: true, smooth: true },
    [{ value: 0.8, tone: "danger" }],
  );
  expect(option).toMatchObject({
    legend: { show: false },
    yAxis: { min: 0, max: 1 },
    series: [
      {
        type: "line",
        areaStyle: {},
        stack: "samples",
        smooth: true,
        connectNulls: false,
        data: [
          [1000, 0.2],
          [2000, null],
          [3000, 0.8],
        ],
        markLine: { data: [{ yAxis: 0.8, label: { formatter: "80.0%" } }] },
      },
    ],
  });
  expect(series).toEqual(before);
  expect(
    metricOption("timeseries", series, true, "", 2, { draw_style: "bar" }),
  ).toMatchObject({ series: [{ type: "bar" }] });
});

it("uses the selected reduction in gauges, bars and pie without rounding the underlying value", () => {
  const series = metricObservations([
    {
      metric: { host: "a" },
      values: [
        [1, "1"],
        [2, "4"],
      ],
    },
  ]);
  expect(
    metricOption(
      "gauge",
      series,
      true,
      "",
      0,
      { reducer: "mean", min: 0, max: 10 },
      [{ value: 2, tone: "warning" }],
    ),
  ).toMatchObject({
    series: [
      {
        min: 0,
        max: 10,
        data: [{ value: 2.5 }],
        itemStyle: { color: "var(--warning)" },
      },
    ],
  });
  expect(
    metricOption("bar_gauge", series, true, "", 2, {
      reducer: "sum",
      min: 0,
      max: 10,
    }),
  ).toMatchObject({
    xAxis: { min: 0, max: 10 },
    series: [{ data: [{ value: 5 }], showBackground: true }],
  });
  expect(
    metricOption("pie", series, true, "", 0, { reducer: "mean" }),
  ).toMatchObject({ series: [{ data: [{ value: 2.5 }] }] });
  expect(
    metricOption("gauge", series, true, "", 0, { min: 100 }),
  ).toMatchObject({ series: [{ min: 100, max: 110 }] });
});

it("handles empty heatmaps without infinite ranges and retains null gauge samples", () => {
  expect(metricOption("heatmap", [], true)).toMatchObject({
    visualMap: { min: 0, max: 1 },
    series: [{ data: [] }],
  });
  const series = metricObservations([
    {
      values: [
        [1, "4"],
        [2, "NaN"],
      ],
    },
  ]);
  expect(metricOption("gauge", series, true)).toMatchObject({
    series: [{ data: [] }],
  });
});

it("rejects duplicate or malformed histogram bounds and buckets from different snapshots", () => {
  for (const [bound, time] of [
    ["invalid", 1],
    ["1", 1],
    ["+Inf", 2],
  ] as const) {
    const series = metricObservations([
      { metric: { le: "1" }, value: [1, "2"] },
      { metric: { le: bound }, value: [time, "5"] },
    ]);
    expect(histogramObservations(series)).toBeNull();
  }
});

it("themes axes, legends and gauge labels without overriding formatters, bounds or explicit threshold colors", () => {
  const formatter = (value: number) => `${value}%`;
  const option = {
    xAxis: [{ type: "time" as const }],
    yAxis: { min: 0, max: 1, axisLabel: { formatter } },
    legend: { show: true },
    series: [
      {
        type: "gauge" as const,
        itemStyle: { color: "var(--warning)" },
        detail: { formatter },
      },
    ],
  };
  const before = JSON.stringify(option);
  const themed = themeObservation(option, (token) => `resolved:${token}`);
  expect(themed).toMatchObject({
    xAxis: [
      {
        type: "time",
        axisLabel: { color: "resolved:--text-secondary", hideOverlap: true },
      },
    ],
    yAxis: {
      min: 0,
      max: 1,
      axisLabel: { color: "resolved:--text-secondary", formatter },
    },
    legend: { textStyle: { color: "resolved:--text-secondary" } },
    series: [{ detail: { color: "resolved:--warning", formatter } }],
  });
  expect(JSON.stringify(option)).toBe(before);
});
