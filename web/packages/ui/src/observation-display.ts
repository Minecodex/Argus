import type { ObservationSeries } from "./observation-data";

export type ObservationDisplay = {
  reducer?: "last" | "min" | "max" | "mean" | "sum";
  min?: number;
  max?: number;
  draw_style?: "line" | "area" | "bar";
  stack?: boolean;
  smooth?: boolean;
};
export type ObservationThreshold = {
  value: number;
  tone: "info" | "success" | "warning" | "danger";
};

// Never backfill a missing latest sample. Other reductions use only returned,
// finite samples, independently per series (including its source identity).
export function reduceObservation(
  series: ObservationSeries,
  reducer: ObservationDisplay["reducer"] = "last",
): number | null {
  if (reducer === "last") return series.points.at(-1)?.[1] ?? null;
  let count = 0,
    total = 0,
    min = Infinity,
    max = -Infinity;
  for (const [, value] of series.points) {
    if (value === null || !Number.isFinite(value)) continue;
    count++;
    total += value;
    min = Math.min(min, value);
    max = Math.max(max, value);
  }
  if (!count) return null;
  const result =
    reducer === "min"
      ? min
      : reducer === "max"
        ? max
        : reducer === "mean"
          ? total / count
          : total;
  return Number.isFinite(result) ? result : null;
}

export function observationTone(
  value: number | null,
  thresholds: ObservationThreshold[],
) {
  if (value === null) return undefined;
  let tone: ObservationThreshold["tone"] | undefined;
  for (const threshold of thresholds) {
    if (value < threshold.value) break;
    tone = threshold.tone;
  }
  return tone;
}

// Resolve token colors at render time, so Canvas follows the active theme too.
// Functions and their closures are preserved; this is not a JSON clone.
export function resolveObservationColors(
  value: unknown,
  color: (token: string) => string,
): unknown {
  if (typeof value === "string" && /^var\(--[a-z-]+\)$/.test(value))
    return color(value.slice(4, -1));
  if (Array.isArray(value))
    return value.map((v) => resolveObservationColors(v, color));
  if (
    value !== null &&
    typeof value === "object" &&
    Object.getPrototypeOf(value) === Object.prototype
  )
    return Object.fromEntries(
      Object.entries(value).map(([k, v]) => [
        k,
        resolveObservationColors(v, color),
      ]),
    );
  return value;
}
