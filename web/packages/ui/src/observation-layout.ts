import type { EChartsOption } from "echarts";

type Settings = Record<string, unknown>;
const object = (value: unknown): Settings =>
  value && typeof value === "object" ? (value as Settings) : {};
const components = (value: unknown, apply: (settings: Settings) => Settings) =>
  Array.isArray(value)
    ? value.map((item) => apply(object(item)))
    : value === undefined
      ? undefined
      : apply(object(value));

/** Bound canvas text to the available plot area, preserving full names and data. */
export function layoutObservation(
  option: EChartsOption,
  sizes: {
    width: number;
    height: number;
    labelWidth: number;
    font: number;
    heading: number;
    padding: number;
    footer: number;
  },
): EChartsOption {
  const { width, height, font, padding, footer } = sizes;
  const labelWidth = Math.min(sizes.labelWidth, width * 0.28);
  const source = Array.isArray(option.series)
    ? option.series
    : option.series
      ? [option.series]
      : [];
  const gauges = source.filter((item) => object(item).type === "gauge");
  const category = (axis: Settings) =>
    axis.type !== "category"
      ? axis
      : {
          ...axis,
          axisLabel: {
            ...object(axis.axisLabel),
            width: labelWidth,
            overflow: "truncate",
          },
        };
  const series = components(option.series, (item) => {
    if (item.type === "pie")
      return {
        ...item,
        center: [width / 2, (height - footer) / 2],
        label: {
          ...object(item.label),
          width: labelWidth,
          overflow: "truncate",
          alignTo: "edge",
          edgeDistance: "8%",
        },
        labelLayout: {
          ...object(item.labelLayout),
          hideOverlap: true,
          moveOverlap: "shiftY",
        },
      };
    if (item.type !== "gauge") return item;
    const cell = width / Math.max(1, gauges.length);
    const radius = Math.max(
      1,
      Math.min((height - footer) * 0.375, cell / 2 - padding),
    );
    const detail = object(item.detail),
      format = detail.formatter;
    const detailFont = gauges.length > 1 ? font : sizes.heading;
    const lines =
      typeof format === "function" && /\s/.test(String(format(1))) ? 2 : 1;
    const detailOffset =
      radius * 0.85 +
      font * 0.6 +
      padding / 2 +
      (detailFont * 1.25 * lines) / 2;
    const captionWidth = Math.max(1, cell - padding * 2);
    return {
      ...item,
      name:
        item.name ??
        object(Array.isArray(item.data) ? item.data[0] : undefined).name,
      center: [
        Array.isArray(item.center) ? item.center[0] : "50%",
        (height - footer) / 2,
      ],
      radius,
      splitNumber:
        item.splitNumber ?? Math.max(2, Math.min(5, Math.floor(radius / font))),
      axisLabel: {
        ...object(item.axisLabel),
        fontSize: font,
        lineHeight: font * 1.2,
        distance: padding / 2,
        hideOverlap: true,
      },
      title: {
        ...object(item.title),
        fontSize: font,
        offsetCenter: [0, "85%"],
        width: captionWidth,
        overflow: "truncate",
      },
      detail: {
        ...detail,
        offsetCenter: [0, detailOffset],
        width: captionWidth,
        overflow: "truncate",
        fontSize: detailFont,
        lineHeight: detailFont * 1.25,
        ...(typeof format === "function"
          ? {
              formatter: (value: number) =>
                String(format(value)).replace(/\s+/, "\n"),
            }
          : {}),
      },
    };
  });
  return {
    ...option,
    xAxis: components(option.xAxis, category),
    yAxis: components(option.yAxis, category),
    ...(option.legend || gauges.length
      ? {
          legend: components(
            option.legend ?? { show: true, type: "scroll" },
            (item) => ({
              ...item,
              top: "auto",
              bottom: 0,
              left: padding,
              right: padding,
              tooltip: { ...object(item.tooltip), show: true },
              textStyle: {
                ...object(item.textStyle),
                width: Math.min(
                  sizes.labelWidth,
                  Math.max(1, width - padding * 2 - footer),
                ),
                overflow: "truncate",
              },
            }),
          ),
        }
      : {}),
    series,
  } as EChartsOption;
}
