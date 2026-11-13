import type { BaseContext } from "./context";
import type { DashboardSchemas } from "../dashboard";
import {
  dashboardState,
  dashboardPermission,
  dashboardAccess,
  dashboardFailure,
} from "./dashboard-state";
import { ensureManagedDashboards } from "./managed-dashboards";
import { resolvePermissions } from "./permissions";
import type { KubernetesInventoryScope } from "../kubernetes-workbench";
const key = "kubernetes.operations-overview";
export function mockOverview(
  ctx: BaseContext,
  scope: KubernetesInventoryScope,
): DashboardSchemas["KubernetesOverviewSelection"] {
  dashboardPermission(ctx);
  ensureManagedDashboards(ctx);
  const state = dashboardState(ctx);
  const normal = state.items.find(
    (row) =>
      row.enterprise === ctx.enterpriseId() &&
      row.value.managed_template_key === key,
  )?.value;
  if (!normal) dashboardFailure("DASHBOARD_UNAVAILABLE", 503);
  const choice = state.overviews?.find(
    (row) =>
      row.enterprise === ctx.enterpriseId() && row.cluster === scope.cluster_id,
  );
  const board = state.items.find(
    (row) =>
      row.enterprise === ctx.enterpriseId() &&
      row.value.id === (choice?.dashboard_id ?? normal.id),
  )?.value;
  const revision = state.revisions.find(
    (row) => row.id === board?.active_revision_id,
  );
  const readable =
    !!board &&
    dashboardAccess(ctx, board.id) &&
    board.lifecycle === "active" &&
    !!revision?.spec.panels.length &&
    revision.spec.panels.every((p) =>
      p.applicable_resource_types.some(
        (k) => k === "kubernetes_cluster" || k === "kubernetes_namespace",
      ),
    );
  return {
    cluster_id: scope.cluster_id,
    resource_version: scope.cluster_version,
    selection_version: choice?.version ?? 0,
    mode: choice && choice.dashboard_id !== normal.id ? "custom" : "default",
    default_dashboard: normal,
    dashboard: readable ? board : undefined,
    unavailable_code: readable ? undefined : "DASHBOARD_UNAVAILABLE",
  };
}
export function validateOverviewChoice(
  ctx: BaseContext,
  scope: KubernetesInventoryScope,
  input: DashboardSchemas["KubernetesOverviewInput"],
) {
  const permissions = resolvePermissions(
    ctx.db,
    ctx.enterpriseId(),
    ctx.actor().id,
    ctx.nowIso(),
  );
  if (
    !scope.cluster_wide ||
    (!permissions.has("*") && !permissions.has("kubernetes.manage"))
  )
    dashboardFailure("DASHBOARD_DENIED", 403);
  const current = mockOverview(ctx, scope);
  if (
    current.selection_version !== input.expected_selection_version ||
    scope.cluster_version !== input.expected_resource_version
  )
    dashboardFailure("DASHBOARD_VERSION_CONFLICT", 409);
  if (!dashboardAccess(ctx, input.dashboard_id))
    dashboardFailure("DASHBOARD_DENIED", 403);
  const board = dashboardState(ctx).items.find(
    (row) =>
      row.enterprise === ctx.enterpriseId() &&
      row.value.id === input.dashboard_id,
  )?.value;
  if (!board) dashboardFailure("DASHBOARD_UNAVAILABLE", 404);
  if (
    board.version !== input.expected_dashboard_version ||
    board.lifecycle !== "active"
  )
    dashboardFailure("DASHBOARD_VERSION_CONFLICT", 409);
  const revision = dashboardState(ctx).revisions.find(
    (row) => row.id === board.active_revision_id,
  );
  if (
    !revision ||
    !revision.spec.panels.length ||
    revision.spec.panels.some(
      (p) =>
        !p.applicable_resource_types.some(
          (kind) =>
            kind === "kubernetes_cluster" || kind === "kubernetes_namespace",
        ),
    )
  )
    dashboardFailure();
  if (input.reset && input.dashboard_id !== current.default_dashboard.id)
    dashboardFailure();
  return { current, board };
}
export function commitOverviewChoice(
  ctx: BaseContext,
  input: Record<string, unknown>,
) {
  const cluster = ctx.db.clusters.find(
    (row) =>
      row.id === input.cluster_id && row.enterpriseId === ctx.enterpriseId(),
  );
  if (!cluster) dashboardFailure("DASHBOARD_DENIED", 403);
  const choice =
    input as unknown as DashboardSchemas["KubernetesOverviewInput"];
  const scope: KubernetesInventoryScope = {
    cluster_id: cluster.id,
    cluster_name: cluster.name,
    cluster_version: cluster.resourceVersion ?? 1,
    cluster_wide: true,
    namespaces: [],
  };
  const { current } = validateOverviewChoice(ctx, scope, choice);
  const rows = (dashboardState(ctx).overviews ??= []);
  const old = rows.find(
    (row) =>
      row.enterprise === ctx.enterpriseId() && row.cluster === cluster.id,
  );
  if (old) {
    old.dashboard_id = choice.dashboard_id;
    old.version++;
  } else
    rows.push({
      enterprise: ctx.enterpriseId(),
      cluster: cluster.id,
      dashboard_id: choice.dashboard_id,
      version: current.selection_version + 1,
    });
}
