import type { BaseContext } from "./context";
import type {
  DashboardExecution,
  DashboardItem,
  DashboardSchemas,
  DashboardSpec,
} from "../dashboard";
import type { ModelUsagePoint } from "../types";
import { resolvePermissions } from "./permissions";
import { dashboardFailure, modelGovernancePermission } from "./dashboard-state";

// Only the explicitly selected mock API uses seeded samples. Real mode always
// calls the private server/metric-service pipeline.
export function mockModelGovernanceExecution(
  ctx: BaseContext,
  item: DashboardItem,
  spec: DashboardSpec,
  input: DashboardSchemas["DashboardExecutionInput"],
): DashboardExecution {
  if (!modelGovernancePermission(ctx))
    dashboardFailure("DASHBOARD_DENIED", 403);
  const enterprise = ctx.enterpriseId();
  const permissions = resolvePermissions(
    ctx.db,
    enterprise,
    ctx.actor().id,
    ctx.nowIso(),
  );
  const admin = permissions.has("*") || permissions.has("model.manage");
  const department = ctx.db.enterpriseUsers.find(
    (user) =>
      user.userId === ctx.actor().id && user.enterpriseId === enterprise,
  )?.departmentId;
  const models = ctx.db.models.filter(
    (model) => model.enterpriseId === enterprise,
  );
  const month = input.variables?.month?.values[0] ?? ctx.nowIso().slice(0, 7);
  if (!/^\d{4}-\d{2}$/.test(month)) dashboardFailure("DASHBOARD_INVALID", 400);
  const selection = input.variables?.model ?? { all: true, values: [] };
  if (selection.values.some((id) => !models.some((model) => model.id === id)))
    dashboardFailure("DASHBOARD_DENIED", 403);
  const points = ctx.db.usagePoints.filter(
    (point) =>
      point.date.startsWith(month) &&
      models.some((model) => model.id === point.modelId) &&
      (admin || point.departmentId === department) &&
      (selection.all || selection.values.includes(point.modelId)),
  );
  const from = new Date(`${month}-01T00:00:00Z`),
    to = new Date(from);
  to.setUTCMonth(to.getUTCMonth() + 1);
  const sum = (rows: ModelUsagePoint[], key: keyof ModelUsagePoint) =>
    rows.reduce((total, point) => total + Number(point[key]), 0);
  const scalar = (id: string, rows: ModelUsagePoint[]) => {
    const count = sum(rows, "requestCount");
    const values: Record<string, number> = {
      requests: count,
      tokens: sum(rows, "inputTokens") + sum(rows, "outputTokens"),
      amount: sum(rows, "amount"),
      success: sum(rows, "successCount") / count,
      errors: sum(rows, "errorCount"),
      latency:
        rows.reduce((n, p) => n + p.avgLatencyMs * p.requestCount, 0) / count,
    };
    return values[id];
  };
  const hash = "a".repeat(64),
    at = Date.parse(ctx.nowIso()) / 1000;
  return {
    dashboard_id: item.id,
    revision_id: item.active_revision_id,
    execution_id: crypto.randomUUID(),
    execution_hash: hash,
    scope_signature: hash,
    from: from.toISOString(),
    to: to.toISOString(),
    resources: [],
    scope_versions: { resources: {}, relations: {} },
    partial: false,
    variables: { model: selection, month: { all: false, values: [month] } },
    local_values: {},
    variable_candidates: {},
    local_candidates: {},
    model_governance: {
      from: from.toISOString(),
      to: to.toISOString(),
      timezone: "UTC",
      models: models.map(({ id, name }) => ({ id, name })),
      subjects: [
        ...ctx.db.departments
          .filter(
            (d) =>
              d.enterprise_id === enterprise && (admin || d.id === department),
          )
          .map((d) => ({
            id: d.id,
            name: d.name,
            kind: "department" as const,
            department_id: d.id,
          })),
        ...ctx.db.enterpriseUsers
          .filter(
            (u) =>
              u.enterpriseId === enterprise &&
              (admin || u.departmentId === department),
          )
          .map((u) => ({
            id: u.userId,
            name:
              ctx.db.users.find((user) => user.id === u.userId)?.displayName ??
              u.userId,
            kind: "user" as const,
            department_id: u.departmentId,
          })),
      ],
      pending: 0,
      projected_at: ctx.nowIso(),
      usage_complete: true,
      estimated_input: 0,
      estimated_output: 0,
      scope_hash: hash,
    },
    panels: input.candidates_only
      ? []
      : spec.panels
          .filter(
            (panel) =>
              !input.panel_ids?.length || input.panel_ids.includes(panel.id),
          )
          .map((panel) => {
            const value = scalar(panel.id, points),
              data =
                points.length && Number.isFinite(value)
                  ? [{ metric: {}, value: [at, String(value)] }]
                  : [];
            return {
              id: panel.id,
              status: data.length ? "success" : "no_data",
              sources: [],
              detail_sources: {},
              targets: [
                {
                  id: "a",
                  status: "success",
                  query_hash: hash,
                  result_type: "vector",
                  data,
                  meta: {
                    engine: "mock",
                    engine_version: "mock",
                    sample_time_basis: "mock_seed",
                    ingestion_status: "ready",
                    cache_hit: false,
                    plan_hash: hash,
                    scanned_bytes: 0,
                    scanned_rows: 0,
                    returned_rows: data.length,
                    loaded_samples: data.length,
                    elapsed_millis: 0,
                    partial: false,
                    warnings: [],
                  },
                },
              ],
            };
          }),
  };
}
