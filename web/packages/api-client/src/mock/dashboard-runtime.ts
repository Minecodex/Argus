import type {
  DashboardExecution,
  DashboardPanel,
  DashboardSchemas,
  DashboardSpec,
  DashboardTarget,
} from "../dashboard";
import type { BaseContext } from "./context";
import {
  copyDashboard as copy,
  dashboardFailure as fail,
  dashboardState,
  dashboardObjectGrant,
} from "./dashboard-state";

const contexts = new Map<
  string,
  {
    actor: string;
    dashboard: string;
    revision: string;
    spec: DashboardSpec;
    execution: DashboardExecution;
    expires: number;
  }
>();
const rowData = (
  panel: DashboardPanel,
  target: DashboardTarget,
  from: number,
  to: number,
  resource: string,
): unknown => {
  const at = new Date(to - 30_000).toISOString();
  const source = `mock-source-${resource}`;
  if (panel.signal === "logs")
    return [
      {
        timestamp: at,
        resource_id: resource,
        source_id: source,
        source_revision: 1,
        source_type: panel.source_binding.source_type,
        service_name: "checkout",
        severity_text: "INFO",
        body: "Request completed",
        trace_id: "mock-trace",
        span_id: "mock-span",
        event_id: "mock-event",
      },
    ];
  if (panel.signal === "traces") {
    const span = {
      traceId: "mock-trace",
      spanId: "mock-span",
      parentSpanId: "",
      sourceId: source,
      resourceId: resource,
      serviceName: "checkout",
      operationName: "GET /orders",
      startTime: at,
      duration: 120,
      status: "ok",
      attributes: "{}",
      events: "[]",
      links: "[]",
    };
    if (panel.type === "apm_topology")
      return {
        queryAPMTopology: {
          basis: "received_destination_entry_spans",
          status: "available",
          nodes: [
            {
              id: "checkout",
              serviceName: "checkout",
              sourceId: source,
              resourceId: resource,
            },
            {
              id: "storage",
              serviceName: "storage",
              sourceId: source,
              resourceId: resource,
            },
          ],
          edges: [
            {
              sourceNodeId: "checkout",
              targetNodeId: "storage",
              sampleCount: 24,
              errorCount: 1,
              errorRate: 1 / 24,
              durationP95Ms: 120,
            },
          ],
          coverage: { observedSpanCount: 48, missingParentCount: 0 },
        },
      };
    if (panel.type.startsWith("apm_"))
      return {
        result: {
          basis: "received_entry_spans",
          status: "available",
          percentileMethod: "tdigest",
          rows: Array.from(
            { length: panel.type === "apm_red" ? 12 : 3 },
            (_, i) => ({
              sourceId: source,
              resourceId: resource,
              serviceName: "checkout",
              instanceId: `instance-${i + 1}`,
              instanceName: `instance-${i + 1}`,
              operationName: ["GET /orders", "POST /orders", "GET /health"][
                i % 3
              ],
              timestamp: new Date(from + ((to - from) * i) / 12).toISOString(),
              intervalSeconds: (to - from) / 12000,
              sampleCount: 24 + i,
              errorCount: i === 1 ? 1 : 0,
              errorRate: i === 1 ? 0.04 : 0,
              samplesPerSecond: 1 + i / 10,
              durationMeanMs: 65,
              durationP95Ms: 120 + i * 2,
              durationP99Ms: 190,
            }),
          ),
          coverage: { observedSpanCount: 80, requestSampleCount: 72 },
        },
      };
    if (panel.type === "trace_detail")
      return {
        queryTraceGraph: {
          traceId: "mock-trace",
          completeness: "observed_connected",
          missingParentCount: 0,
          ambiguousParentCount: 0,
          excludedSpanCount: 0,
          spans: [span],
          edges: [],
        },
      };
    return {
      queryTraces: {
        total: 1,
        traces: [
          {
            ...span,
            rootService: span.serviceName,
            rootOperation: span.operationName,
            spanCount: 1,
            errorCount: 0,
          },
        ],
      },
    };
  }
  const histogram = panel.type === "histogram" || panel.type === "heatmap";
  return Array.from({ length: histogram ? 5 : 2 }, (_, series) => {
    const metric = {
      __name__:
        target.source_definition.builder?.metric ?? "system_cpu_utilization",
      host: histogram ? "node-1" : `node-${series + 1}`,
      ...(histogram ? { le: ["0.1", "0.5", "1", "2", "+Inf"][series] } : {}),
    };
    const value = (i: number) =>
      histogram
        ? String((series + 1) * 12 + i)
        : String(35 + series * 20 + Math.sin(i / 3) * 12);
    return target.query_mode === "instant"
      ? { metric, value: [to / 1000, value(12)] }
      : {
          metric,
          values: Array.from({ length: 24 }, (_, i) => [
            Math.floor((from + ((to - from) * i) / 23) / 1000),
            value(i),
          ]),
        };
  });
};
export function mockDashboardExecution(
  ctx: BaseContext,
  spec: DashboardSpec,
  input: DashboardSchemas["DashboardExecutionInput"],
  id = "mock-draft",
  revision = "mock-unpublished",
): DashboardExecution {
  const to = input.to ? Date.parse(input.to) : Date.now(),
    from = input.from
      ? Date.parse(input.from)
      : to - (spec.default_time_range.seconds ?? 3600) * 1000;
  const available = [
    ...ctx.db.hosts
      .filter(
        (r) =>
          r.enterpriseId === ctx.enterpriseId() &&
          (!r.status || r.status === "active") &&
          dashboardObjectGrant(ctx, "host", r.id),
      )
      .map((r) => ({ id: r.id, type: "host" as const })),
    ...ctx.db.clusters
      .filter(
        (r) =>
          r.enterpriseId === ctx.enterpriseId() &&
          dashboardObjectGrant(ctx, "kubernetes_cluster", r.id),
      )
      .map((r) => ({ id: r.id, type: "kubernetes_cluster" as const })),
  ];
  if (input.resource_ids?.some((id) => !available.some((r) => r.id === id)))
    fail("DASHBOARD_DENIED", 403);
  const resources = input.resource_ids?.length
    ? available.filter((r) => input.resource_ids!.includes(r.id))
    : available;
  if (!resources.length) fail("DASHBOARD_DENIED", 403);
  const resource = resources[0]!.id;
  for (const [key, value] of contexts)
    if (value.expires < Date.now()) contexts.delete(key);
  if (contexts.size >= 128) contexts.delete(contexts.keys().next().value!);
  const token = crypto.randomUUID();
  const result: DashboardExecution = {
    dashboard_id: id,
    revision_id: revision,
    execution_id: crypto.randomUUID(),
    execution_hash: crypto.randomUUID(),
    context_token: token,
    context_expires_at: new Date(Date.now() + 900000).toISOString(),
    from: new Date(from).toISOString(),
    to: new Date(to).toISOString(),
    resources,
    partial: false,
    variables: {},
    local_values: {},
    variable_candidates: {},
    local_candidates: {},
    panels: [],
  };
  for (const variable of spec.variables) {
    const selection = input.variables?.[variable.name] ?? variable.default;
    const values = ["production", "staging"];
    const reset =
      !selection.all && selection.values.some((v) => !values.includes(v));
    result.variables[variable.name] = reset
      ? { all: true, values: [] }
      : copy(selection);
    result.variable_candidates[variable.name] = {
      status: "success",
      values,
      complete: true,
      selected_exists: Object.fromEntries(
        selection.values.map((v) => [v, values.includes(v)]),
      ),
      reset,
      sources: [],
    };
  }
  for (const panel of spec.panels) {
    result.local_values[panel.id] = Object.fromEntries(
      panel.local_filters.map((f) => [
        f.id,
        input.local_values?.[panel.id]?.[f.id] ?? f.default,
      ]),
    );
    result.local_candidates[panel.id] = {};
    if (
      input.candidates_only ||
      (input.panel_ids?.length && !input.panel_ids.includes(panel.id))
    )
      continue;
    result.panels.push({
      id: panel.id,
      status: "success",
      sources: [
        {
          id: `mock-source-${resource}`,
          resource_id: resource,
          revision: 1,
          generation: "mock-installation",
          type: panel.source_binding.source_type,
          capability_version: "v1",
        },
      ],
      detail_sources: {},
      targets: panel.targets.map((target) => ({
        id: target.id,
        status: "success",
        query_hash: `mock-${target.id}`,
        result_type:
          panel.signal === "metrics"
            ? target.query_mode === "instant"
              ? "vector"
              : "matrix"
            : panel.signal === "logs"
              ? "log_entries"
              : panel.type.startsWith("apm_")
                ? panel.type
                : "traces",
        data: rowData(panel, target, from, to, resource),
        meta: { engine: "mock-dashboard", warnings: ["MOCK_DATA"] },
      })),
    });
  }
  contexts.set(token, {
    actor: ctx.actor().id,
    dashboard: id,
    revision,
    spec: copy(spec),
    execution: copy(result),
    expires: Date.now() + 900000,
  });
  return result;
}
export { mockDashboardCatalog } from "./dashboard-catalog";
export function mockGenerateDrilldowns(
  spec: DashboardSpec,
  panelId: string,
): string[] {
  const panel = spec.panels.find((p) => p.id === panelId) ?? fail();
  const added: string[] = [];
  for (const target of panel.targets) {
    const id = `inspect_${target.id}`;
    if (panel.drilldowns.some((d) => d.id === id)) continue;
    panel.detail_query_targets.push({
      ...copy(target),
      id: `detail_${target.id}`,
      signal: panel.signal,
      source_binding: panel.source_binding,
    });
    panel.drilldowns.push({
      id,
      title: "",
      kind: "inspect",
      origin_query_ref: target.id,
      detail_query_ref: `detail_${target.id}`,
      scope_policy: "inherit",
      inputs: {},
    });
    added.push(id);
  }
  return added;
}
export function mockDashboardDrilldown(
  ctx: BaseContext,
  id: string,
  input: DashboardSchemas["DashboardDrilldownInput"],
): DashboardSchemas["DashboardDrilldownExecution"] {
  const prior = contexts.get(input.context_token);
  if (!prior || prior.expires < Date.now())
    fail("DASHBOARD_CONTEXT_EXPIRED", 409);
  if (prior.actor !== ctx.actor().id || prior.dashboard !== id)
    fail("DASHBOARD_DENIED", 403);
  const panel =
    prior.spec.panels.find((p) => p.id === input.panel_id) ?? fail();
  const drill =
    panel.drilldowns.find((d) => d.id === input.drilldown_id) ?? fail();
  const target =
    panel.detail_query_targets.find((t) => t.id === drill.detail_query_ref) ??
    fail();
  const record = dashboardState(ctx).items.find((r) => r.value.id === id);
  if (record?.value.lifecycle !== "active") fail("DASHBOARD_ARCHIVED", 409);
  return {
    dashboard_id: id,
    revision_id: prior.revision,
    parent_execution_id: prior.execution.execution_id,
    panel_id: panel.id,
    drilldown_id: drill.id,
    scope_policy: "inherit",
    from: prior.execution.from,
    to: prior.execution.to,
    resources: prior.execution.resources,
    sources: [],
    result: {
      id: target.id,
      status: "success",
      query_hash: "mock-detail",
      result_type:
        panel.signal === "metrics"
          ? "matrix"
          : panel.signal === "logs"
            ? "log_entries"
            : panel.type.startsWith("apm_")
              ? panel.type
              : "traces",
      data: rowData(
        panel,
        target,
        Date.parse(prior.execution.from),
        Date.parse(prior.execution.to),
        prior.execution.resources[0]?.id ?? "mock-resource",
      ),
      meta: { engine: "mock-dashboard", warnings: ["MOCK_DATA"] },
    },
    context_token: input.context_token,
    context_expires_at: prior.execution.context_expires_at!,
    execution_hash: crypto.randomUUID(),
  };
}
