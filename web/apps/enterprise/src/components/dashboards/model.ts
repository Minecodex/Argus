import type {
  DashboardPanel,
  DashboardSpec,
  DashboardTarget,
  DashboardDraft,
  DashboardSchemas,
} from "@argus/api-client";
export const metricCharts = [
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
];
export const traceCharts = [
  "trace_list",
  "trace_detail",
  "apm_services",
  "apm_instances",
  "apm_endpoints",
  "apm_red",
  "apm_topology",
];
export const sources = {
  metrics: ["hostmetrics", "prometheus", "otlp", "kubeletstats", "k8s_cluster"],
  logs: ["otlp", "filelog", "journald", "windowseventlog"],
  traces: ["otlp", "skywalking", "jaeger"],
};
export function isDashboardSignal(
  value: string,
): value is DashboardPanel["signal"] {
  return Object.hasOwn(sources, value);
}
export function newTarget(
  signal: string,
  chart: string,
  mode: "builder" | "dsl",
): DashboardTarget {
  const language =
    signal === "metrics"
      ? "promql"
      : signal === "logs"
        ? "kql"
        : "skywalking_graphql";
  const operation = chart.startsWith("apm_")
    ? chart
    : signal === "metrics"
      ? "value"
      : signal === "logs"
        ? "records"
        : chart === "trace_detail"
          ? "detail"
          : "list";
  return {
    id: "main",
    language,
    query_mode:
      signal === "metrics"
        ? ["timeseries", "state_timeline", "scatter", "heatmap"].includes(chart)
          ? "range"
          : "instant"
        : "",
    range_step_policy: {
      kind: "auto",
      target_points: 300,
      min_step_seconds: 1,
    },
    parameter_bindings: [],
    source_definition:
      mode === "builder"
        ? {
            builder: {
              operation:
                operation as DashboardSchemas["DashboardBuilder"]["operation"],
              metric:
                signal === "metrics" ? "system_cpu_utilization" : undefined,
              filters: [],
              group_by: [],
              ...(chart === "apm_red" ? { bucket_seconds: 60 } : {}),
            },
          }
        : {
            dsl: {
              expression:
                signal === "metrics"
                  ? "system_cpu_utilization"
                  : signal === "logs"
                    ? "* | limit 100"
                    : "query { queryTraces(pageSize:100) { total traces { traceId sourceId resourceId rootService rootOperation startTime duration status spanCount errorCount } } }",
            },
          },
  };
}
export function newPanel(spec: DashboardSpec, title: string): DashboardPanel {
  return {
    id: crypto.randomUUID(),
    title,
    description: "",
    type: "timeseries",
    signal: "metrics",
    authoring_mode: "builder",
    applicable_resource_types: ["host", "kubernetes_cluster"],
    source_binding: { source_type: "hostmetrics", capability_version: "v1" },
    local_filters: [],
    targets: [newTarget("metrics", "timeseries", "builder")],
    detail_query_targets: [],
    drilldowns: [],
    layout: {
      x: 0,
      y: Math.max(0, ...spec.panels.map((p) => p.layout.y + p.layout.h)),
      w: 6,
      h: 38,
      min_w: 3,
      min_h: 24,
    },
    unit: "",
    decimals: 2,
    legend: true,
    thresholds: [],
  };
}
export function draftInput(
  d: DashboardDraft,
): DashboardSchemas["DashboardDraftInput"] {
  return {
    dashboard_id: d.dashboard_id,
    name: d.name,
    description: d.description,
    folder_id: d.folder_id,
    spec: d.spec,
    proposed_bindings: d.proposed_bindings,
    expected_version: d.draft_version,
  };
}
export function scalarPointer(
  row: Record<string, unknown>,
  pointer: string,
): string | undefined {
  let value: unknown = row;
  for (const encoded of pointer.slice(1).split("/")) {
    const key = encoded.replaceAll("~1", "/").replaceAll("~0", "~");
    if (typeof value !== "object" || value === null) return undefined;
    value = (value as Record<string, unknown>)[key];
  }
  return ["string", "number", "boolean"].includes(typeof value)
    ? String(value)
    : undefined;
}
