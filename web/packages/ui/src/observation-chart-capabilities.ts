import {
  histogramObservations,
  metricObservations,
  logAggregateObservations,
} from "./observation-data";
import {
  reduceObservation,
  type ObservationDisplay,
} from "./observation-display";

export const metricChartTypes = [
  "timeseries",
  "stat",
  "gauge",
  "bar_gauge",
  "bar",
  "pie",
  "histogram",
  "heatmap",
  "state_timeline",
  "scatter",
  "table",
] as const;
export type ChartReason =
  | "runFirst"
  | "noData"
  | "incomplete"
  | "numericRequired"
  | "rangeRequired"
  | "bucketRequired"
  | "negativeSlices"
  | "logRecords"
  | "logAggregates"
  | "traceShape"
  | "rangeResult"
  | "snapshotResult"
  | "bucketResult"
  | "compatible";
export type ChartCapability = {
  type: string;
  state: "recommended" | "compatible" | "unverified" | "incompatible";
  reason: ChartReason;
};
export type ChartTarget = {
  id: string;
  status: string;
  result_type?: string;
  data?: unknown;
};

/** Shared with renderers: use actual finite values and the selected display reducer. */
export function observationChartCapabilities(
  signal: string,
  types: readonly string[],
  targets: ChartTarget[] | undefined,
  queries: Array<{ id: string; query_mode?: string }>,
  display: ObservationDisplay = {},
): ChartCapability[] {
  const success =
    targets?.filter((t) => ["success", "partial"].includes(t.status)) ?? [];
  const unavailable: ChartReason = !targets ? "runFirst" : "noData";
  if (!success.length)
    return types.map((type) => ({
      type,
      state: "unverified",
      reason: unavailable,
    }));
  const incomplete =
    queries.some((q) => !success.some((t) => t.id === q.id)) ||
    success.some((t) => t.status === "partial");
  if (signal === "traces")
    return types.map((type) => {
      const compatible = success.every(
        (t) =>
          t.result_type === type ||
          (type === "trace_list" && t.result_type === "traces") ||
          (type === "trace_detail" && t.result_type === "trace_graph"),
      );
      return {
        type,
        state: compatible
          ? incomplete
            ? "compatible"
            : "recommended"
          : "incompatible",
        reason: compatible
          ? incomplete
            ? "incomplete"
            : "compatible"
          : "traceShape",
      };
    });
  if (signal === "logs" && success.some((t) => t.result_type === "log_entries"))
    return types.map((type) => ({
      type,
      state:
        ["logs", "table"].includes(type) &&
        success.every((t) => t.result_type === "log_entries")
          ? incomplete
            ? "compatible"
            : "recommended"
          : "incompatible",
      reason: "logRecords",
    }));
  const groups = success.map((t) =>
    signal === "logs"
      ? logAggregateObservations(t.data)
      : metricObservations(t.data),
  );
  if (
    groups.some(
      (group) =>
        !group.length ||
        group.every((s) => s.points.every((p) => p[1] === null)),
    )
  )
    return types.map((type) => ({
      type,
      state: type === "table" ? "compatible" : "unverified",
      reason:
        signal === "metrics" && success.some((t) => t.result_type === "string")
          ? "numericRequired"
          : "noData",
    }));
  const series = groups.flat(),
    range = success.every(
      (t, i) =>
        (signal === "logs" ||
          queries.find((q) => q.id === t.id)?.query_mode === "range" ||
          t.result_type === "matrix") &&
        groups[i]!.some((s) => new Set(s.points.map((p) => p[0])).size > 1),
    ),
    buckets =
      groups.every((group) => histogramObservations(group) !== null) &&
      histogramObservations(series) !== null,
    negative = series.some(
      (s) => (reduceObservation(s, display.reducer) ?? 0) < 0,
    );
  return types.map((type) => {
    let reason: ChartReason = "compatible",
      compatible = true,
      recommended = false;
    if (
      ["timeseries", "heatmap", "state_timeline", "scatter"].includes(type) &&
      !range
    ) {
      compatible = false;
      reason = "rangeRequired";
    } else if (type === "histogram" && !buckets) {
      compatible = false;
      reason = "bucketRequired";
    } else if (type === "pie" && negative) {
      compatible = false;
      reason = "negativeSlices";
    } else if (type === "logs") {
      compatible = false;
      reason = "logAggregates";
    } else if (type === "histogram" && buckets) {
      recommended = true;
      reason = "bucketResult";
    } else if (range && ["timeseries", "heatmap"].includes(type)) {
      recommended = true;
      reason = "rangeResult";
    } else if (!range && ["stat", "bar", "table"].includes(type)) {
      recommended = true;
      reason = "snapshotResult";
    }
    return {
      type,
      state: !compatible
        ? "incompatible"
        : incomplete
          ? "compatible"
          : recommended
            ? "recommended"
            : "compatible",
      reason: compatible && incomplete ? "incomplete" : reason,
    };
  });
}
