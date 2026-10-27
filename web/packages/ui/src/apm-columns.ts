export type ObservationColumn = {
  key: string;
  label: string;
  format?: (value: unknown) => string;
};

const number = (value: unknown, scale = 1, suffix = "") =>
  typeof value === "number" && Number.isFinite(value)
    ? `${(value * scale).toFixed(2)}${suffix}`
    : "—";

// Summary tables show the requested grouping and received-sample statistics.
// The original row remains intact for published drilldowns and query details.
export function apmColumns(
  type: string,
  text: (zh: string, en: string) => string,
): ObservationColumn[] | undefined {
  if (type === "trace_list")
    return [
      { key: "rootService", label: text("入口服务", "Entry service") },
      { key: "rootOperation", label: text("入口操作", "Entry operation") },
      {
        key: "duration",
        label: text("耗时", "Duration"),
        format: (value) => number(value, 1, " ms"),
      },
      { key: "status", label: text("状态", "Status") },
      { key: "startTime", label: text("开始时间", "Start time") },
      { key: "spanCount", label: text("Span 数", "Spans") },
      { key: "errorCount", label: text("错误数", "Errors") },
      { key: "traceId", label: text("调用链 ID", "Trace ID") },
    ];
  if (!["apm_services", "apm_instances", "apm_endpoints"].includes(type))
    return undefined;
  const columns: ObservationColumn[] = [
    {
      key: "serviceNamespace",
      label: text("服务命名空间", "Service namespace"),
    },
    { key: "serviceName", label: text("服务", "Service") },
    {
      key: "sourceId",
      label: text("采集来源", "Collection source"),
      format: (value) => (typeof value === "string" ? value.slice(0, 8) : "—"),
    },
  ];
  if (type === "apm_instances")
    columns.push({ key: "instanceId", label: text("实例", "Instance") });
  if (type === "apm_endpoints")
    columns.push({ key: "operationName", label: text("接口", "Endpoint") });
  return [
    ...columns,
    { key: "sampleCount", label: text("请求样本", "Request samples") },
    { key: "errorCount", label: text("错误样本", "Error samples") },
    {
      key: "errorRate",
      label: text("样本错误率", "Sample error rate"),
      format: (value) => number(value, 100, "%"),
    },
    {
      key: "durationMeanMs",
      label: text("平均耗时", "Mean duration"),
      format: (value) => number(value, 1, " ms"),
    },
    {
      key: "durationP95Ms",
      label: text("P95 耗时", "P95 duration"),
      format: (value) => number(value, 1, " ms"),
    },
  ];
}
