import type { DashboardSchemas } from "@argus/api-client";

type Builder = DashboardSchemas["DashboardBuilder"];
export type MetricMetadata = DashboardSchemas["DashboardMetricDescriptor"];

export function mergeMetricCatalog(previous: MetricMetadata[], incoming: MetricMetadata[]) {
  const rows = new Map<string, MetricMetadata>();
  for (const metric of [...previous, ...incoming])
    rows.set(JSON.stringify([metric.name, metric.type, metric.unit, metric.source_type,
      [...(metric.source_keys ?? [])].sort()]), metric);
  return [...rows.values()];
}

/** A query selects a name; describe all returned variants instead of picking one. */
export function metricCatalogOptions(metrics: MetricMetadata[]) {
  const names = new Map<string, Set<string>>();
  for (const metric of metrics) {
    const descriptions = names.get(metric.name) ?? new Set<string>();
    descriptions.add([metric.type, metric.unit].filter(Boolean).join(" · "));
    names.set(metric.name, descriptions);
  }
  return [...names].map(([name, descriptions]) => ({
    value: name, label: name, description: [...descriptions].join(" / "),
  }));
}

export function catalogMetricMetadata(metrics: MetricMetadata[], name: string, complete: boolean) {
  const matches = metrics.filter((item) => item.name === name);
  if (!matches.length) return undefined;
  const conflict = new Set(matches.map((item) => JSON.stringify([item.type, item.unit]))).size > 1;
  if (conflict || !complete)
    return { name, type: "mixed", unit: "", labels: [...new Set(matches.flatMap(item => item.labels))] };
  return matches[0];
}

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
  return (
    !required || (!actual && metadata?.type !== "mixed") || actual === required
  );
}

/** Never copy the previous metric's declaration to a newly selected metric. */
export function selectMetric(builder: Builder, metric: string): Builder {
  return metric === builder.metric
    ? builder
    : { ...builder, metric, metric_type: undefined };
}
