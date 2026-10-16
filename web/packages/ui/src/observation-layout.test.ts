import { expect, it } from "vitest";
import { metricOption } from "./metric-options";
import { layoutObservation } from "./observation-layout";

const sizes = {
  width: 336,
  height: 260,
  labelWidth: 160,
  font: 12,
  heading: 20,
  padding: 12,
  footer: 48,
};
const series = ["orders", "warehouse", "archive"].map((name) => ({
  name: `db-prod-01 · ${name} · orders-api`,
  labels: {},
  row: {},
  points: [[1, 15] as [number, number]],
}));

it("shows a single sample inside the requested time window without inventing a trend", () => {
  const from = "2026-10-14T05:00:00Z",
    to = "2026-10-14T05:05:00Z";
  const point = {
    ...series[0]!,
    points: [[Date.parse(to), 7] as [number, number]],
  };
  expect(
    metricOption("timeseries", [point], true, "ms", 1, {}, [], { from, to }),
  ).toMatchObject({
    xAxis: { min: Date.parse(from), max: Date.parse(to) },
    series: [{ showSymbol: true, connectNulls: false, data: point.points }],
  });
});

it("keeps full bar identities and samples while reserving visible plot width", () => {
  const original = metricOption("bar", series, true, "connections", 1)!;
  const before = JSON.stringify(original);
  const arranged = layoutObservation(original, sizes) as unknown as {
    yAxis: { data: string[]; axisLabel: { width: number } };
    series: Array<{ data: Array<{ value: number }> }>;
  };
  expect(arranged.yAxis.data).toEqual(series.map((item) => item.name));
  expect(arranged.yAxis.axisLabel.width).toBeLessThan(sizes.width / 3);
  expect(
    arranged.series[0]!.data.map((item: { value: number }) => item.value),
  ).toEqual([15, 15, 15]);
  expect(JSON.stringify(original)).toBe(before);
});

it("fits multiple gauge arcs and captions, retaining units, bounds and hidden legend", () => {
  const arranged = layoutObservation(
    metricOption("gauge", series, false, "connections", 1, {
      min: 0,
      max: 20,
    })!,
    sizes,
  ) as unknown as {
    legend: { show: boolean };
    series: Array<{
      radius: number;
      min: number;
      max: number;
      data: Array<{ name: string; value: number }>;
      title: { width: number };
      detail: { fontSize: number; formatter(value: number): string };
    }>;
  };
  expect(arranged.legend.show).toBe(false);
  for (const [index, gauge] of arranged.series.entries()) {
    expect(gauge.radius * 2).toBeLessThan(sizes.width / series.length);
    expect(gauge.min).toBe(0);
    expect(gauge.max).toBe(20);
    expect(gauge.data[0]).toEqual({ name: series[index]!.name, value: 15 });
    expect(gauge.detail.formatter(15)).toBe("15.0\nconnections");
    expect(gauge.title.width).toBeLessThan(sizes.width / series.length);
    expect(gauge.detail.fontSize).toBeLessThanOrEqual(sizes.font);
  }
});
