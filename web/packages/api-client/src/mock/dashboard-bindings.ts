import type {
  DashboardDomains,
  DashboardResourceType,
  DashboardSchemas,
  DashboardBindingView,
} from "../dashboard";
import type { BaseContext, MockContext } from "./context";
import {
  dashboardState,
  dashboardPermission,
  dashboardAccess,
  dashboardObjectGrant,
  dashboardFailure as fail,
  copyDashboard as copy,
  mockDashboard,
} from "./dashboard-state";
import { resolvePermissions } from "./permissions";
const bindings = (ctx: BaseContext) => (dashboardState(ctx).bindings ??= []);
function resource(
  ctx: BaseContext,
  type: DashboardResourceType,
  id: string,
  manage = false,
) {
  const permissions = resolvePermissions(
    ctx.db,
    ctx.enterpriseId(),
    ctx.actor().id,
    ctx.nowIso(),
  );
  const permission =
    type === "host"
      ? manage
        ? "host.manage"
        : "host.read"
      : manage
        ? "kubernetes.manage"
        : "kubernetes.read";
  if (!permissions.has("*") && !permissions.has(permission))
    fail("DASHBOARD_DENIED", 403);
  if (!dashboardObjectGrant(ctx, type, id)) fail("DASHBOARD_DENIED", 403);
  const row =
    type === "host"
      ? ctx.db.hosts.find(
          (h) => h.id === id && h.enterpriseId === ctx.enterpriseId(),
        )
      : ctx.db.clusters.find(
          (c) => c.id === id && c.enterpriseId === ctx.enterpriseId(),
        );
  if (!row || ("status" in row && row.status && row.status !== "active"))
    fail("DASHBOARD_NOT_FOUND", 404);
  return row;
}
type BindingPlan = DashboardSchemas["DashboardBindingInput"] & {
  resource_type: DashboardResourceType;
  resource_id: string;
  id: string;
};
function validate(ctx: BaseContext, plan: BindingPlan) {
  if (
    !["host", "kubernetes_cluster"].includes(plan.resource_type) ||
    !["attach", "detach"].includes(plan.operation) ||
    (plan.operation === "attach" &&
      (plan.binding_id || plan.expected_binding_version)) ||
    (plan.operation === "detach" &&
      (!plan.binding_id || !plan.expected_binding_version))
  )
    fail();
  dashboardPermission(ctx);
  const row = resource(ctx, plan.resource_type, plan.resource_id, true),
    dashboard = mockDashboard(ctx, plan.dashboard_id);
  if (
    (row.resourceVersion ?? 1) !== plan.expected_resource_version ||
    dashboard.version !== plan.expected_dashboard_version
  )
    fail("DASHBOARD_VERSION_CONFLICT", 409);
  if (plan.operation === "attach" && dashboard.lifecycle !== "active")
    fail("DASHBOARD_ARCHIVED", 409);
  const current = bindings(ctx).find(
    (b) =>
      b.enterprise === ctx.enterpriseId() &&
      b.dashboard_id === dashboard.id &&
      b.resource_type === plan.resource_type &&
      b.resource_id === row.id,
  );
  if (
    plan.operation === "attach"
      ? Boolean(current)
      : !current ||
        current.id !== plan.binding_id ||
        current.version !== plan.expected_binding_version
  )
    fail("DASHBOARD_VERSION_CONFLICT", 409);
  return { row, dashboard };
}
export function createDashboardBindingMethods(
  ctx: MockContext,
): Pick<
  DashboardDomains["dashboards"],
  "bindings" | "resourceBindings" | "previewBinding"
> {
  const entry = (
    binding: ReturnType<typeof bindings>[number],
  ): DashboardBindingView => {
    const row = resource(ctx, binding.resource_type, binding.resource_id);
    return {
      id: binding.id,
      version: binding.version,
      resource_type: binding.resource_type,
      resource_id: binding.resource_id,
      resource_name: row.name,
      resource_version: row.resourceVersion ?? 1,
      dashboard: copy(mockDashboard(ctx, binding.dashboard_id)),
    };
  };
  return {
    async resourceBindings(type, id) {
      dashboardPermission(ctx);
      const row = resource(ctx, type, id);
      return {
        resource_type: type,
        resource_id: id,
        resource_name: row.name,
        resource_version: row.resourceVersion ?? 1,
        items: bindings(ctx)
          .filter(
            (b) =>
              b.enterprise === ctx.enterpriseId() &&
              b.resource_type === type &&
              b.resource_id === id &&
              dashboardAccess(ctx, b.dashboard_id),
          )
          .filter(
            (b) => mockDashboard(ctx, b.dashboard_id).lifecycle === "active",
          )
          .map(entry),
      };
    },
    async bindings(id) {
      const item = mockDashboard(ctx, id);
      if (item.lifecycle !== "active") return [];
      return bindings(ctx)
        .filter(
          (b) => b.enterprise === ctx.enterpriseId() && b.dashboard_id === id,
        )
        .flatMap((b) => {
          try {
            return [entry(b)];
          } catch {
            return [];
          }
        });
    },
    async previewBinding(type, id, input) {
      const plan: BindingPlan = {
        ...input,
        resource_type: type,
        resource_id: id,
        id: input.binding_id ?? crypto.randomUUID(),
      };
      const { row, dashboard } = validate(ctx, plan);
      const action = ctx.createPendingAction({
        tool: `telemetry.dashboard.binding.${input.operation}`,
        title: dashboard.name,
        risk: "write",
        input_data: plan,
      });
      action.preview = {
        name: dashboard.name,
        resource_name: row.name,
        resource_type: type,
        operation: input.operation,
      };
      action.diff = [
        {
          kind: input.operation === "attach" ? "add" : "remove",
          text: `${row.name} → ${dashboard.name}`,
        },
      ];
      ctx.save();
      return copy(action);
    },
  };
}
export function commitDashboardBinding(
  ctx: BaseContext,
  input: Record<string, unknown>,
) {
  const plan = input as BindingPlan;
  validate(ctx, plan);
  if (plan.operation === "attach")
    bindings(ctx).push({
      enterprise: ctx.enterpriseId(),
      id: plan.id,
      version: 1,
      dashboard_id: plan.dashboard_id,
      resource_type: plan.resource_type,
      resource_id: plan.resource_id,
    });
  else
    dashboardState(ctx).bindings = bindings(ctx).filter(
      (b) => b.id !== plan.binding_id,
    );
  ctx.audit(`dashboard.binding.${plan.operation}`, {
    resourceType: plan.resource_type,
    resourceId: plan.resource_id,
    summary: plan.dashboard_id,
  });
}
