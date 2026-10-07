import type { DashboardSchemas } from "@argus/api-client";

type Builder = DashboardSchemas["DashboardBuilder"];
export type MetricMetadata = DashboardSchemas["DashboardMetricDescriptor"];

export function knownMetricType(
  metadata?: MetricMetadata,
): Builder["metric_type"] {
  return metadata && ["gauge", "counter", "histogram"].includes(metadata.type)
    ? (metadata.type as Builder["metric_type"])
    : undefined;
}

export function requiredMetricType(operation: string) {
  return ["rate", "error_rate"].includes(operation)
    ? "counter"
    : operation === "p95"
      ? "histogram"
      : undefined;
}

export function metricOperationCompatible(
  operation: string,
  metadata?: MetricMetadata,
) {
  const required = requiredMetricType(operation),
    actual = knownMetricType(metadata);
  return !required || !actual || actual === required;
}

/** Never copy the previous metric's declaration to a newly selected metric. */
export function selectMetric(builder: Builder, metric: string): Builder {
  return metric === builder.metric
    ? builder
    : { ...builder, metric, metric_type: undefined };
}
