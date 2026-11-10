// @vitest-environment jsdom
import { afterEach, expect, it } from "vitest";
import type { TooltipComponentFormatterCallbackParams } from "echarts";
import {
  fitObservationTooltip,
  floatingObservationTooltip,
} from "./observation-tooltip";
import { observationTooltipContent } from "./observation-tooltip-content";
import { metricObservations } from "./observation-data";
import { uniqueObservationNames } from "./observation-series-name";
import { themeObservation } from "./observation-theme";

type Item = Exclude<TooltipComponentFormatterCallbackParams, unknown[]>;
const item = (name: string, value: number | null): Item => ({
  componentType: "series",
  componentSubType: "line",
  componentIndex: 0,
  seriesName: name,
  name: "",
  dataIndex: 0,
  data: [1, value],
  value: [1, value],
  color: "#123456",
  $vars: [],
});
afterEach(() => {
  document.documentElement.lang = "en-US";
});

it("uses varying business labels while preserving full labels, samples and source identities", () => {
  const input = ["deployment", "pod_gc"].map((name, i) => ({
    metric: {
      __name__: "workqueue_depth",
      name,
      resource_k8s_namespace_name: "kube-system",
      k8s_pod_name: "kube-controller-manager-node",
      k8s_pod_uid: "08784e06-f5ce-485a-9633-71e551ac1426",
      argus_source_id: "source",
    },
    values: [[1, String(i)]],
  }));
  const before = structuredClone(input);
  const series = metricObservations(input);
  expect(series.map((s) => s.name)).toEqual(["deployment", "pod_gc"]);
  expect(series.map((s) => s.labels.k8s_pod_uid)).toEqual([
    input[0]!.metric.k8s_pod_uid,
    input[1]!.metric.k8s_pod_uid,
  ]);
  expect(series.map((s) => s.points)).toEqual([[[1000, 0]], [[1000, 1]]]);
  expect(series[0]!.row).toBe(input[0]);
  expect(input).toEqual(before);
});

it("disambiguates same-name Pod generations and query targets without merging their data", () => {
  const series = metricObservations(
    ["old-pod-uid", "new-pod-uid"].map((uid, i) => ({
      metric: {
        __name__: "memory",
        resource_k8s_namespace_name: "prod",
        resource_k8s_pod_name: "orders-0",
        resource_k8s_pod_uid: uid,
      },
      value: [1, String(i + 1)],
    })),
  );
  expect(new Set(series.map((s) => s.name)).size).toBe(2);
  expect(
    series.every(
      (s) =>
        s.name.startsWith("prod · orders-0 · ") && !s.name.includes("pod-uid"),
    ),
  ).toBe(true);
  expect(series.map((s) => s.points[0]![1])).toEqual([1, 2]);
  const targets = uniqueObservationNames(
    ["a", "b"].map((target) => ({ ...series[0]!, name: "Memory", target })),
  );
  expect(targets[0]!.name).not.toBe(targets[1]!.name);
});

it.each(["zh-CN", "en-US"])(
  "creates bounded-layout rows with literal untrusted text and correct zero/missing formatting in %s",
  (locale) => {
    document.documentElement.lang = locale;
    const attack = '<img src=x onerror="window.tooltipInjected=true">';
    const rows = Array.from({ length: 120 }, (_, i) =>
      item(i ? `queue-${i}` : attack, i === 1 ? null : 0),
    );
    const content = observationTooltipContent(
      rows,
      document,
      (value) => `${value} ms`,
    );
    expect(content.querySelectorAll('[role="listitem"]')).toHaveLength(120);
    expect(content.textContent).toContain(attack);
    expect(content.querySelector("img")).toBeNull();
    expect(
      content.querySelectorAll(".argus-chart-tooltip__value")[0]!.textContent,
    ).toBe("0 ms");
    expect(
      content.querySelectorAll(".argus-chart-tooltip__value")[1]!.textContent,
    ).toBe("—");
    expect(
      content.querySelector('[role="list"]')?.getAttribute("tabindex"),
    ).toBe("0");
    expect(
      content.querySelector(".argus-chart-tooltip__heading")!.textContent,
    ).toContain(locale === "en-US" ? "120 series" : "120 条序列");
  },
);

it("fits tooltip coordinates in the viewport when charts sit at any edge or inside a scrolled container", () => {
  for (const origin of [
    [0, 0],
    [1000, 700],
    [-100, -300],
    [200, 900],
  ]) {
    const size = [440, 320],
      viewport = [1280, 1000],
      gap = 12;
    const point = fitObservationTooltip(
      [800, 500],
      size,
      origin,
      viewport,
      gap,
    );
    for (const axis of [0, 1]) {
      expect(point[axis]! + origin[axis]!).toBeGreaterThanOrEqual(gap);
      expect(point[axis]! + origin[axis]! + size[axis]!).toBeLessThanOrEqual(
        viewport[axis]! - gap,
      );
    }
  }
  const custom = () => "trusted formatter";
  expect(floatingObservationTooltip({ formatter: custom }).formatter).toBe(
    custom,
  );
  expect(floatingObservationTooltip().enterable).toBe(true);
});

it("retains native multidimensional and item tooltip content and value formatters", () => {
  const formatter = () => "value with unit";
  for (const type of ["pie", "heatmap", "gauge", "graph", "custom"] as const) {
    const themed = themeObservation(
      { tooltip: { valueFormatter: formatter }, series: [{ type }] },
      (token) => token,
    );
    expect(themed.tooltip).toMatchObject({
      renderMode: "html",
      formatter: undefined,
      valueFormatter: formatter,
    });
  }
});
