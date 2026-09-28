import type { EChartsOption } from "echarts";
import {
  formatObservation,
  histogramObservations,
  stateIntervals,
  type ObservationSeries,
} from "./observation-data";
import {
  observationTone,
  reduceObservation,
  type ObservationDisplay,
  type ObservationThreshold,
} from "./observation-display";
export function metricOption(
  type: string,
  series: ObservationSeries[],
  legend: boolean,
  unit = "",
  decimals = 2,
  display: ObservationDisplay = {},
  thresholds: ObservationThreshold[] = [],
): EChartsOption | undefined {
  const formatted = (value: number) => formatObservation(value, unit, decimals);
  const valueColor = (value: number | null) => {
    const tone = observationTone(value, thresholds);
    return tone ? `var(--${tone})` : undefined;
  };
  const markLine = thresholds.length
    ? {
        silent: true,
        symbol: "none",
        data: thresholds.map((t) => ({
          yAxis: t.value,
          lineStyle: { color: `var(--${t.tone})` },
          label: {
            formatter: formatted(t.value),
            position: "insideEndTop" as const,
          },
        })),
      }
    : undefined;
  const base: EChartsOption = {
    legend: { show: legend, type: "scroll" },
    xAxis: { type: "time", axisLabel: { hideOverlap: true } },
    yAxis: {
      type: "value",
      scale: true,
      min: display.min,
      max: display.max,
      axisLabel: {
        formatter: (value: number) => formatObservation(value, unit, decimals),
      },
    },
    dataZoom: [{ type: "inside" }],
  };
  const values = series.map((s) => ({
    name: s.name,
    value: reduceObservation(s, display.reducer),
  }));
  if (type === "gauge") {
    let min = display.min ?? Math.min(0, ...values.map((v) => v.value ?? 0));
    let max =
      display.max ??
      (unit === "percent_ratio"
        ? 1
        : unit === "percent"
          ? 100
          : Math.max(1, ...values.map((v) => v.value ?? 0)));
    // A single configured bound must still leave a non-empty automatic range.
    if (max <= min) {
      if (display.max === undefined)
        max = min + Math.max(1, Math.abs(min) * 0.1);
      else min = max - Math.max(1, Math.abs(max) * 0.1);
    }
    return {
      series: values.map((v, index) => ({
        type: "gauge",
        center: [`${((index + 0.5) * 100) / values.length}%`, "55%"],
        radius: `${Math.min(75, 150 / values.length)}%`,
        min,
        max,
        itemStyle: { color: valueColor(v.value) },
        data: v.value === null ? [] : [{ name: v.name, value: v.value }],
        detail: {
          formatter: (value: number) =>
            formatObservation(value, unit, decimals),
        },
      })),
      tooltip: {
        trigger: "item",
        renderMode: "richText",
        valueFormatter: (value) => formatted(Number(value)),
      },
    };
  }
  if (type === "pie")
    return {
      legend: { show: legend, type: "scroll" },
      tooltip: {
        trigger: "item",
        renderMode: "richText",
        valueFormatter: (value) => formatted(Number(value)),
      },
      series: [
        {
          type: "pie",
          radius: ["30%", "65%"],
          data: values
            .filter((v) => v.value !== null && v.value >= 0)
            .map((v) => ({ name: v.name, value: v.value! })),
        },
      ],
    };
  if (type === "bar" || type === "bar_gauge")
    return {
      xAxis: {
        type: "value",
        min: display.min,
        max: display.max,
        axisLabel: {
          formatter: (value: number) =>
            formatObservation(value, unit, decimals),
        },
      },
      yAxis: {
        type: "category",
        data: values.map((v) => v.name),
        inverse: true,
      },
      series: [
        {
          type: "bar",
          data: values.map((v) => ({
            value: v.value,
            itemStyle: { color: valueColor(v.value) },
          })),
          showBackground: type === "bar_gauge",
          markLine: thresholds.length
            ? {
                silent: true,
                symbol: "none",
                data: thresholds.map((t) => ({
                  xAxis: t.value,
                  lineStyle: { color: `var(--${t.tone})` },
                  label: { formatter: formatted(t.value) },
                })),
              }
            : undefined,
        },
      ],
    };
  if (type === "histogram") {
    const data = histogramObservations(series);
    return data
      ? {
          xAxis: { type: "category", data: data.names },
          yAxis: { type: "value" },
          series: [{ type: "bar", data: data.values }],
        }
      : undefined;
  }
  if (type === "heatmap") {
    const times = Array.from(
      new Set(series.flatMap((s) => s.points.map((p) => p[0]))),
    ).sort((a, b) => a - b);
    const timeIndex = new Map(times.map((time, index) => [time, index]));
    const data = series.flatMap((s, y) =>
      s.points
        .filter((p) => p[1] !== null)
        .map(([time, value]) => [timeIndex.get(time)!, y, value as number]),
    );
    return {
      xAxis: {
        type: "category",
        data: times.map((t) => new Date(t).toLocaleTimeString()),
      },
      yAxis: { type: "category", data: series.map((s) => s.name) },
      visualMap: {
        show: true,
        min:
          display.min ??
          data.reduce((min, d) => Math.min(min, d[2]!), data[0]?.[2] ?? 0),
        max:
          display.max ??
          data.reduce((max, d) => Math.max(max, d[2]!), data[0]?.[2] ?? 1),
        orient: "horizontal",
        bottom: 0,
      },
      series: [{ type: "heatmap", data }],
    };
  }
  if (type === "state_timeline")
    return {
      ...base,
      yAxis: { type: "category", data: series.map((s) => s.name) },
      series: series.map((s, index) => ({
        type: "custom",
        name: s.name,
        encode: { x: [0, 1], y: 2, tooltip: [3] },
        data: stateIntervals(s).map(([start, end, value]) => ({
          value: [start, end, index, value],
          itemStyle: { color: valueColor(value) },
        })),
        renderItem: (_params, api) => {
          const start = api.coord([api.value(0), api.value(2)]),
            end = api.coord([api.value(1), api.value(2)]);
          const size = api.size!([0, 1]) as number[];
          return {
            type: "rect",
            shape: {
              x: start[0],
              y: start[1]! - size[1]! * 0.3,
              width: Math.max(1, end[0]! - start[0]!),
              height: size[1]! * 0.6,
            },
            style: {
              fill: api.visual("color") as string,
              opacity: thresholds.length
                ? 1
                : 0.45 + Math.abs(Number(api.value(3)) % 3) * 0.2,
            },
          };
        },
      })),
    };

  return {
    ...base,
    series: series.map((s, index) => ({
      type:
        type === "scatter"
          ? "scatter"
          : display.draw_style === "bar"
            ? "bar"
            : "line",
      name: s.name,
      data: s.points,
      showSymbol: false,
      connectNulls: false,
      smooth: display.smooth ?? false,
      stack: display.stack ? "samples" : undefined,
      areaStyle: display.draw_style === "area" ? {} : undefined,
      markLine: index === 0 ? markLine : undefined,
      emphasis: { focus: "series" },
    })),
  };
}
