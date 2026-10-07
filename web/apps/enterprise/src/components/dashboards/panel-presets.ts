import type {
  DashboardPanel,
  DashboardSchemas,
  DashboardSpec,
} from "@argus/api-client";
import { newPanel, newTarget } from "./model";
type Builder = DashboardSchemas["DashboardBuilder"];
export type PanelPreset = {
  id: string;
  signal: DashboardPanel["signal"];
  type: string;
  source: string;
  metric?: string;
  unit?: string;
  metricType?: string;
  operation?: Builder["operation"];
  filters?: Builder["filters"];
  requires: "host" | "disk" | "kubelet" | "application" | "logs" | "traces";
};
/** Templates declare query requirements; choosing a card never enables a Collector. */
export const panelPresets: PanelPreset[] = [
  {
    id: "cpu",
    signal: "metrics",
    type: "timeseries",
    source: "hostmetrics",
    metric: "system_cpu_utilization",
    unit: "percent_ratio",
    metricType: "gauge",
    requires: "host",
  },
  {
    id: "memory",
    signal: "metrics",
    type: "stat",
    source: "hostmetrics",
    metric: "system_memory_usage",
    unit: "bytes",
    requires: "host",
  },
  {
    id: "memoryRatio",
    signal: "metrics",
    type: "gauge",
    source: "hostmetrics",
    metric: "system_memory_utilization",
    unit: "percent_ratio",
    metricType: "gauge",
    requires: "host",
  },
  {
    id: "load",
    signal: "metrics",
    type: "timeseries",
    source: "hostmetrics",
    metric: "system_cpu_load_average_1m",
    requires: "host",
  },
  {
    id: "filesystem",
    signal: "metrics",
    type: "bar_gauge",
    source: "hostmetrics",
    metric: "system_filesystem_usage",
    unit: "bytes",
    requires: "host",
  },
  {
    id: "network",
    signal: "metrics",
    type: "timeseries",
    source: "hostmetrics",
    metric: "system_network_io",
    unit: "bytes",
    operation: "rate",
    metricType: "counter",
    requires: "host",
  },
  {
    id: "disk",
    signal: "metrics",
    type: "timeseries",
    source: "hostmetrics",
    metric: "system_disk_io",
    unit: "bytes",
    operation: "rate",
    metricType: "counter",
    requires: "disk",
  },
  {
    id: "nodeCpu",
    signal: "metrics",
    type: "timeseries",
    source: "kubeletstats",
    metric: "k8s_node_cpu_usage",
    metricType: "gauge",
    requires: "kubelet",
  },
  {
    id: "nodeMemory",
    signal: "metrics",
    type: "timeseries",
    source: "kubeletstats",
    metric: "k8s_node_memory_usage",
    unit: "bytes",
    metricType: "gauge",
    requires: "kubelet",
  },
  {
    id: "podCpu",
    signal: "metrics",
    type: "timeseries",
    source: "kubeletstats",
    metric: "k8s_pod_cpu_usage",
    metricType: "gauge",
    requires: "kubelet",
  },
  {
    id: "podMemory",
    signal: "metrics",
    type: "timeseries",
    source: "kubeletstats",
    metric: "k8s_pod_memory_usage",
    unit: "bytes",
    metricType: "gauge",
    requires: "kubelet",
  },
  {
    id: "requestRate",
    signal: "metrics",
    type: "timeseries",
    source: "prometheus",
    metric: "http_requests_total",
    operation: "rate",
    metricType: "counter",
    requires: "application",
  },
  {
    id: "latency",
    signal: "metrics",
    type: "timeseries",
    source: "prometheus",
    metric: "http_request_duration_seconds_bucket",
    operation: "p95",
    metricType: "histogram",
    unit: "s",
    requires: "application",
  },
  {
    id: "errors",
    signal: "logs",
    type: "timeseries",
    source: "otlp",
    operation: "count_over_time",
    filters: [{ field: "severity_text", operator: "=", value: "ERROR" }],
    requires: "logs",
  },
  {
    id: "logs",
    signal: "logs",
    type: "logs",
    source: "otlp",
    requires: "logs",
  },
  {
    id: "traces",
    signal: "traces",
    type: "trace_list",
    source: "otlp",
    requires: "traces",
  },
  {
    id: "services",
    signal: "traces",
    type: "apm_services",
    source: "otlp",
    requires: "traces",
  },
  {
    id: "instances",
    signal: "traces",
    type: "apm_instances",
    source: "otlp",
    requires: "traces",
  },
  {
    id: "endpoints",
    signal: "traces",
    type: "apm_endpoints",
    source: "otlp",
    requires: "traces",
  },
  {
    id: "red",
    signal: "traces",
    type: "apm_red",
    source: "otlp",
    requires: "traces",
  },
  {
    id: "topology",
    signal: "traces",
    type: "apm_topology",
    source: "otlp",
    requires: "traces",
  },
];
export function createPresetPanel(
  spec: DashboardSpec,
  preset: PanelPreset,
  title: string,
): DashboardPanel {
  const panel = newPanel(spec, title);
  panel.signal = preset.signal;
  panel.type = preset.type;
  panel.source_binding = {
    source_type: preset.source,
    capability_version: "v1",
  };
  panel.targets = [newTarget(preset.signal, preset.type, "builder")];
  panel.unit = preset.unit ?? "";
  const builder = panel.targets[0]!.source_definition.builder!;
  if (preset.metric) builder.metric = preset.metric;
  if (preset.metricType) builder.metric_type = preset.metricType;
  if (preset.operation) builder.operation = preset.operation;
  if (preset.filters) builder.filters = structuredClone(preset.filters);
  if (["rate", "p95", "error_rate"].includes(builder.operation))
    builder.window_seconds = 300;
  if (builder.operation === "count_over_time") builder.bucket_seconds = 60;
  return panel;
}
export function createChartPanel(
  spec: DashboardSpec,
  type: string,
  title: string,
): DashboardPanel {
  const panel = newPanel(spec, title);
  panel.type = type;
  panel.targets = [newTarget("metrics", type, "builder")];
  panel.targets[0]!.source_definition.builder!.metric = "";
  return panel;
}
