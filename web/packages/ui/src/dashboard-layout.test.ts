import { expect, it } from "vitest";
import { arrangeDashboard } from "./dashboard-layout";
import {
  histogramObservations,
  metricObservations,
  lastObservation,
  stateIntervals,
  logAggregateObservations,
} from "./observation-data";
import { traceObservations } from "./trace-observations";
import { apmObservationGroups } from "./apm-red-chart";
it("pushes colliding panels down without overwriting their dimensions", () => {
  const items = ["a", "b", "c"].map((id, i) => ({
    id,
    layout: {
      x: i === 1 ? 6 : 0,
      y: i === 2 ? 4 : 0,
      w: 6,
      h: 4,
      min_w: 2,
      min_h: 2,
    },
  }));
  const next = arrangeDashboard(items, "a", { ...items[0]!.layout, w: 9 });
  expect(next.find((i) => i.id === "b")?.layout.y).toBe(4);
  expect(next.find((i) => i.id === "c")?.layout.y).toBe(4);
  expect(items[0]?.layout.w).toBe(6);
});
it("preserves missing samples and derives histogram bucket differences only within one series identity", () => {
  const values = metricObservations([
    {
      metric: { host: "a" },
      values: [
        [1, "2"],
        [2, "NaN"],
      ],
    },
  ]);
  expect(values[0]?.points).toEqual([
    [1000, 2],
    [2000, null],
  ]);
  const histogram = metricObservations([
    { metric: { le: "1", host: "a" }, value: [1, "2"] },
    { metric: { le: "+Inf", host: "a" }, value: [1, "5"] },
  ]);
  expect(histogramObservations(histogram)?.values).toEqual([2, 3]);
  histogram[1]!.labels.host = "b";
  expect(histogramObservations(histogram)).toBeNull();
});
it("does not replace the latest missing metric with an earlier healthy sample or bridge state gaps", () => {
  const [series] = metricObservations([
    {
      metric: { host: "a" },
      values: [
        [1, "1"],
        [2, "1"],
        [3, "NaN"],
        [4, "2"],
        [5, "NaN"],
      ],
    },
  ]);
  expect(lastObservation(series!)).toBeNull();
  expect(stateIntervals(series!)).toEqual([
    [1000, 3000, 1],
    [4000, 5000, 2],
  ]);
});
it("sorts KQL time buckets separately by explicit aggregation group", () => {
  const series = logAggregateObservations([
    { timestamp: "2026-01-01T00:00:02Z", group_value: "a", count: 3 },
    { timestamp: "2026-01-01T00:00:01Z", group_value: "b", count: 9 },
    { timestamp: "2026-01-01T00:00:01Z", group_value: "a", count: 2 },
  ]);
  expect(series.map((s) => [s.name, s.points.map((p) => p[1])])).toEqual([
    ["a", [2, 3]],
    ["b", [9]],
  ]);
});
it("keeps same-named APM services in different sources and resources in separate curves", () => {
  const row = {
    serviceName: "checkout",
    timestamp: "2026-01-01T00:00:00Z",
    samplesPerSecond: 1,
  };
  const groups = apmObservationGroups([
    { target: "a", row: { ...row, sourceId: "s1", resourceId: "r1" } },
    { target: "a", row: { ...row, sourceId: "s2", resourceId: "r1" } },
    { target: "a", row: { ...row, sourceId: "s1", resourceId: "r2" } },
  ]);
  expect(groups).toHaveLength(3);
});
it("trace hierarchy follows explicit cross-resource edges without merging colliding Span IDs", () => {
  const span = {
    traceId: "t",
    startTime: "2026-01-01T00:00:00Z",
    duration: 50,
    serviceName: "same",
  };
  const nodes = traceObservations({
    spans: [
      {
        ...span,
        sourceId: "a",
        resourceId: "r1",
        spanId: "root",
        parentSpanId: "",
      },
      {
        ...span,
        sourceId: "b",
        resourceId: "r2",
        spanId: "child",
        parentSpanId: "root",
      },
      {
        ...span,
        sourceId: "c",
        resourceId: "r3",
        spanId: "root",
        parentSpanId: "",
      },
    ],
    edges: [
      {
        childSpanId: "child",
        childSourceId: "b",
        childResourceId: "r2",
        parentSpanId: "root",
        parentSourceId: "a",
        parentResourceId: "r1",
      },
    ],
  });
  expect(nodes.map((n) => [n.row.sourceId, n.depth])).toEqual([
    ["a", 0],
    ["b", 1],
    ["c", 0],
  ]);
  expect(new Set(nodes.map((n) => n.key)).size).toBe(3);
});
