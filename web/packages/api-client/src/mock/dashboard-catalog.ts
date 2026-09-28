import type { DashboardSchemas } from "../dashboard";
import { dashboardFailure as fail } from "./dashboard-state";

// Deterministic editor fixtures; result metadata labels these as mock data.
export function mockDashboardCatalog(
  input: DashboardSchemas["DashboardCatalogInput"],
): DashboardSchemas["DashboardCatalogResult"] {
  const metrics = [
    {
      name: "system_cpu_utilization",
      type: "gauge",
      unit: "%",
      labels: ["host", "state"],
    },
    {
      name: "http_requests_total",
      type: "counter",
      unit: "requests",
      labels: ["service", "status"],
    },
    {
      name: "http_request_duration_seconds",
      type: "histogram",
      unit: "s",
      labels: ["service", "le"],
    },
  ];
  const names =
    input.signal === "metrics"
      ? [
          "__name__",
          ...new Set(
            metrics
              .filter((m) => !input.metric || input.metric === m.name)
              .flatMap((m) => m.labels),
          ),
        ]
      : input.signal === "logs"
        ? [
            "body",
            "timestamp",
            "service_name",
            "severity_number",
            "severity_text",
            "trace_id",
            "resource_attributes.deployment.environment",
            ...Array.from(
              { length: 30 },
              (_, i) =>
                `structured_metadata.field_${String(i).padStart(2, "0")}`,
            ),
          ]
        : [
            "service_name",
            "operation",
            "span_kind",
            "status",
            "trace_id",
            "attributes.http.status_code",
            "resource_attributes.service.instance.id",
          ];
  const allValues =
    input.field === "status"
      ? ["200", "500"]
      : input.field === "service_name" || input.field === "service"
        ? ["checkout", "storage"]
        : input.field === "severity_number"
          ? ["9", "17"]
          : input.field === "body"
            ? ["Request completed", "Connection refused"]
            : ["production", "staging", "checkout", "storage"];
  const { cursor, limit, selected_values: selected, ...scope } = input;
  const context = JSON.stringify(scope);
  let after = "";
  if (cursor) {
    try {
      const decoded = JSON.parse(decodeURIComponent(cursor)) as {
        context: string;
        after: string;
      };
      if (decoded.context !== context || !decoded.after)
        fail("DASHBOARD_INVALID", 400);
      after = decoded.after;
    } catch {
      fail("DASHBOARD_INVALID", 400);
    }
  }
  const fieldType = (name: string) =>
    name === "timestamp"
      ? "datetime"
      : ["severity_number", "span_kind"].includes(name)
        ? "number"
        : "string";
  const entries = (
    input.kind === "metrics"
      ? metrics.filter((m) => !input.metric || m.name === input.metric)
      : input.kind === "fields"
        ? names.map((name) => ({ name, type: fieldType(name) }))
        : allValues.map((name) => ({ name }))
  )
    .filter(
      (m) =>
        m.name.toLowerCase().includes((input.search ?? "").toLowerCase()) &&
        m.name > after,
    )
    .sort((a, b) => (a.name < b.name ? -1 : a.name === b.name ? 0 : 1));
  const page = entries.slice(0, limit),
    hasMore = entries.length > limit;
  return {
    metrics:
      input.kind === "metrics"
        ? (page as DashboardSchemas["DashboardMetricDescriptor"][])
        : [],
    fields:
      input.kind === "fields"
        ? (page as DashboardSchemas["DashboardCatalogField"][])
        : [],
    values: input.kind === "values" ? page.map((v) => v.name) : [],
    complete: !input.search && !cursor && !hasMore,
    has_more: hasMore,
    next_cursor: hasMore
      ? encodeURIComponent(
          JSON.stringify({ context, after: page.at(-1)!.name }),
        )
      : undefined,
    selected_exists: Object.fromEntries(
      selected.map((v) => [v, allValues.includes(v)]),
    ),
    meta: { engine: "mock-dashboard", warnings: ["MOCK_DATA"] },
  };
}
