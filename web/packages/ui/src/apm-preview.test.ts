import { expect, it } from "vitest";
import {
  observationChartRows,
  observationRows,
  observationEnvelopes,
} from "./observation-data";
import { apmObservationGroups } from "./apm-red-chart";
const point = (second: number) => ({
  serviceName: "orders",
  resourceId: "host",
  sourceId: "source",
  timestamp: new Date(1800000000000 + second * 1000).toISOString(),
  samplesPerSecond: second,
});
it("uses whole-window chart points independently of exact table rows", () => {
  const rows = Array.from({ length: 600 }, (_, i) => point(i));
  const result = {
    rows,
    chart_preview: { series: [{ points: [rows[0], rows[599]] }] },
  };
  expect(observationRows(result)).toHaveLength(600);
  expect(observationChartRows(result)).toEqual([point(0), point(599)]);
  expect(observationChartRows({ rows })).toHaveLength(600);
});
it("keeps aliased roots separate even when their resource and service match", () => {
  const result = {
    data: {
      one: {
        basis: "samples",
        limited: false,
        rows: [point(0)],
        chart_preview: { series: [{ points: [point(0)] }] },
      },
      two: {
        basis: "samples",
        limited: true,
        rows: [point(1)],
        chart_preview: { series: [{ points: [point(1)] }] },
      },
    },
  };
  const rows = observationChartRows(result).map((row) => ({
    row,
    target: "same",
  }));
  expect(apmObservationGroups(rows)).toHaveLength(2);
  expect(observationEnvelopes(result).map((row) => row.limited)).toEqual([
    false,
    true,
  ]);
});
it("does not replace an explicitly unavailable preview with a cropped raw curve", () => {
  expect(
    observationChartRows({
      rows: [point(0)],
      chart_preview: { series: [], omitted_series: 1 },
    }),
  ).toEqual([]);
});
