import type { EChartsOption } from "echarts";
import { resolveObservationColors } from "./observation-display";

type Settings = Record<string, unknown>;
const settings = (value: unknown): Settings =>
  value !== null && typeof value === "object" ? (value as Settings) : {};
const components = (
  value: unknown,
  apply: (v: Settings) => Settings,
): unknown =>
  Array.isArray(value)
    ? value.map((v) => apply(settings(v)))
    : value === undefined
      ? value
      : apply(settings(value));

// ECharts axis/legend defaults override global textStyle. Theme them explicitly,
// preserving the query renderer's axis shape, labels, bounds and formatters.
export function themeObservation(
  option: EChartsOption,
  color: (token: string) => string,
): EChartsOption {
  const text = color("--text-secondary"),
    primary = color("--text-primary"),
    border = color("--border-default");
  const axis = (v: Settings) => ({
    ...v,
    nameTextStyle: { color: text, ...settings(v.nameTextStyle) },
    axisLabel: { color: text, hideOverlap: true, ...settings(v.axisLabel) },
    axisLine: {
      ...settings(v.axisLine),
      lineStyle: { color: border, ...settings(settings(v.axisLine).lineStyle) },
    },
    splitLine: {
      ...settings(v.splitLine),
      lineStyle: {
        color: border,
        ...settings(settings(v.splitLine).lineStyle),
      },
    },
  });
  return resolveObservationColors(
    {
      ...option,
      xAxis: components(option.xAxis, axis),
      yAxis: components(option.yAxis, axis),
      legend: components(option.legend, (v) => ({
        ...v,
        textStyle: { color: text, ...settings(v.textStyle) },
        pageTextStyle: { color: text, ...settings(v.pageTextStyle) },
        pageIconColor: text,
        pageIconInactiveColor: border,
      })),
      visualMap: components(option.visualMap, (v) => ({
        ...v,
        textStyle: { color: text, ...settings(v.textStyle) },
      })),
      tooltip: {
        renderMode: "richText",
        trigger: "axis",
        ...settings(option.tooltip),
        backgroundColor: color("--bg-elevated"),
        borderColor: border,
        textStyle: {
          ...settings(settings(option.tooltip).textStyle),
          color: primary,
        },
      },
      series: components(option.series, (v) => ({
        ...v,
        label: { color: text, ...settings(v.label) },
        ...(v.markLine
          ? {
              markLine: {
                ...settings(v.markLine),
                label: {
                  color: primary,
                  textBorderWidth: 0,
                  ...settings(settings(v.markLine).label),
                },
              },
            }
          : {}),
        ...(v.type === "gauge"
          ? {
              axisLabel: { color: text, ...settings(v.axisLabel) },
              title: { color: text, ...settings(v.title) },
              detail: {
                color: settings(v.itemStyle).color ?? primary,
                ...settings(v.detail),
              },
            }
          : {}),
      })),
    },
    color,
  ) as EChartsOption;
}
