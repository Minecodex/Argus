import type { DashboardPanel } from "../dashboard";

export function validateMockDisplay(
  p: DashboardPanel,
  path: string,
  add: (path: string, message: string) => void,
) {
  if (!Number.isInteger(p.decimals) || p.decimals < 0 || p.decimals > 8)
    add(`${path}.decimals`, "Decimal places must be between 0 and 8");
  if (new TextEncoder().encode(p.unit).length > 32)
    add(`${path}.unit`, "Unit exceeds 32 bytes");
  if (p.thresholds.length > 16)
    add(`${path}.thresholds`, "At most 16 thresholds");
  if (
    p.thresholds.length &&
    ![
      "stat",
      "gauge",
      "bar_gauge",
      "bar",
      "timeseries",
      "scatter",
      "state_timeline",
    ].includes(p.type)
  )
    add(`${path}.thresholds`, "Thresholds are not supported by this chart");
  p.thresholds.forEach((t, i) => {
    if (
      !Number.isFinite(t.value) ||
      (i > 0 && t.value <= p.thresholds[i - 1]!.value)
    )
      add(
        `${path}.thresholds[${i}].value`,
        "Thresholds must be finite and strictly increasing",
      );
    if (!["info", "success", "warning", "danger"].includes(t.tone))
      add(`${path}.thresholds[${i}].tone`, "Unsupported threshold tone");
  });
  const d = p.display;
  if (!d) return;
  if (
    p.signal === "traces" ||
    p.type === "logs" ||
    (p.signal === "logs" && p.type === "table")
  )
    add(`${path}.display`, "Display options require a numeric chart");
  if (
    d.reducer &&
    (!["last", "min", "max", "mean", "sum"].includes(d.reducer) ||
      !["stat", "gauge", "bar_gauge", "bar", "pie", "table"].includes(p.type))
  )
    add(`${path}.display.reducer`, "Unsupported sample reduction");
  if (d.min !== undefined || d.max !== undefined) {
    if (
      ![
        "timeseries",
        "gauge",
        "bar_gauge",
        "bar",
        "scatter",
        "heatmap",
      ].includes(p.type)
    )
      add(`${path}.display`, "Bounds are not supported by this chart");
    for (const bound of ["min", "max"] as const)
      if (d[bound] !== undefined && !Number.isFinite(d[bound]))
        add(`${path}.display.${bound}`, "Bound must be finite");
    if (d.min !== undefined && d.max !== undefined && d.min >= d.max)
      add(`${path}.display`, "Minimum must be less than maximum");
  }
  if (d.draw_style && !["line", "area", "bar"].includes(d.draw_style))
    add(`${path}.display.draw_style`, "Unsupported draw style");
  if ((d.draw_style || d.stack || d.smooth) && p.type !== "timeseries")
    add(
      `${path}.display`,
      "Style, stacking and smoothing require a time series",
    );
  if (d.draw_style === "bar" && d.smooth)
    add(`${path}.display.smooth`, "Bar time series cannot be smoothed");
}
