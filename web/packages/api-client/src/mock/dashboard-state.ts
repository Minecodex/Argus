import type {
  DashboardDraft,
  DashboardFolder,
  DashboardItem,
  DashboardRevision,
  DashboardSpec,
  DashboardExecution,
  DashboardSchemas,
} from "../dashboard";
import type { BaseContext } from "./context";
import { ApiError } from "../transport/errors";
import { resolvePermissions } from "./permissions";
import { validateMockDisplay } from "./dashboard-display";

export interface MockDashboards {
  items: Array<{ enterprise: string; creator: string; value: DashboardItem }>;
  drafts: Array<{ enterprise: string; editor: string; value: DashboardDraft }>;
  revisions: DashboardRevision[];
  folders: Array<{ enterprise: string; value: DashboardFolder }>;
  queryJobs?: Array<{
    enterprise: string;
    owner: string;
    conversation: string;
    key: string;
    input: string;
    execution: DashboardExecution;
    spec: DashboardSpec;
    drilldown?: Record<string, unknown>;
    value: DashboardSchemas["DashboardQueryJobView"];
  }>;
  bindings?: Array<{
    enterprise: string;
    id: string;
    version: number;
    dashboard_id: string;
    resource_type: "host" | "kubernetes_cluster";
    resource_id: string;
  }>;
}
export const copyDashboard = <T>(value: T): T => structuredClone(value);
export const dashboardState = (ctx: BaseContext): MockDashboards =>
  (ctx.db.dashboards ??= { items: [], drafts: [], revisions: [], folders: [] });
export function dashboardFailure(
  code = "DASHBOARD_INVALID",
  status = 400,
): never {
  throw new ApiError(
    {
      code,
      message_key: `errors.dashboard.${code.replace("DASHBOARD_", "").toLowerCase()}`,
      request_id: crypto.randomUUID(),
      retryable: false,
    },
    status,
  );
}
export function dashboardPermission(ctx: BaseContext, manage = false) {
  const permissions = resolvePermissions(
    ctx.db,
    ctx.enterpriseId(),
    ctx.actor().id,
    ctx.nowIso(),
  );
  if (
    !permissions.has("*") &&
    !permissions.has(
      manage ? "telemetry.dashboard.manage" : "telemetry.dashboard.read",
    )
  )
    dashboardFailure("DASHBOARD_DENIED", 403);
}
export function dashboardAccess(ctx: BaseContext, id: string) {
  const record = dashboardState(ctx).items.find(
    (r) => r.enterprise === ctx.enterpriseId() && r.value.id === id,
  );
  if (!record) return false;
  return dashboardObjectGrant(ctx, "dashboard", id);
}
export function dashboardObjectGrant(
  ctx: BaseContext,
  kind: string,
  id: string,
) {
  const department = ctx.db.enterpriseUsers.find(
    (u) => u.userId === ctx.actor().id && u.enterpriseId === ctx.enterpriseId(),
  )?.departmentId;
  const roles = ctx.db.roleBindings
    .filter(
      (b) =>
        b.enterprise_id === ctx.enterpriseId() &&
        b.status === "active" &&
        ctx.db.roles.some(
          (role) =>
            role.id === b.role_id &&
            role.enterprise_id === ctx.enterpriseId() &&
            role.status === "active",
        ) &&
        (!b.valid_from || b.valid_from <= ctx.nowIso()) &&
        (!b.valid_until || b.valid_until > ctx.nowIso()) &&
        ((b.subject_type === "user" && b.subject_id === ctx.actor().id) ||
          (b.subject_type === "department" && b.subject_id === department)),
    )
    .map((b) => b.role_id);
  return (ctx.db.dataAuthorizationGrants ?? []).some(
    (g) =>
      g.active &&
      g.resource_type === kind &&
      g.resource_id === id &&
      ((g.subject_type === "user" && g.subject_id === ctx.actor().id) ||
        (g.subject_type === "department" && g.subject_id === department) ||
        (g.subject_type === "role" && roles.includes(g.subject_id))),
  );
}
export function mockDashboard(ctx: BaseContext, id: string) {
  dashboardPermission(ctx);
  if (!dashboardAccess(ctx, id)) dashboardFailure("DASHBOARD_DENIED", 403);
  return dashboardState(ctx).items.find((r) => r.value.id === id)!.value;
}
export function mockDraft(ctx: BaseContext, id: string) {
  dashboardPermission(ctx, true);
  const record = dashboardState(ctx).drafts.find(
    (r) =>
      r.enterprise === ctx.enterpriseId() &&
      r.editor === ctx.actor().id &&
      r.value.id === id,
  );
  if (!record) dashboardFailure("DASHBOARD_NOT_FOUND", 404);
  if (
    record.value.dashboard_id &&
    !dashboardAccess(ctx, record.value.dashboard_id)
  )
    dashboardFailure("DASHBOARD_DENIED", 403);
  return record.value;
}
export function draftBaseline(ctx: BaseContext, draft: DashboardDraft) {
  if (draft.status !== "editing")
    dashboardFailure("DASHBOARD_VERSION_CONFLICT", 409);
  if (draft.dashboard_id) {
    const item = mockDashboard(ctx, draft.dashboard_id);
    if (item.lifecycle !== "active")
      dashboardFailure("DASHBOARD_ARCHIVED", 409);
    if (
      draft.base_revision_id !== item.active_revision_id ||
      draft.base_object_version !== item.version
    )
      dashboardFailure("DASHBOARD_VERSION_CONFLICT", 409);
  }
  if (draft.folder_id) {
    const folder = dashboardState(ctx).folders.find(
      (f) =>
        f.enterprise === ctx.enterpriseId() && f.value.id === draft.folder_id,
    )?.value;
    if (!folder || folder.status !== "active")
      dashboardFailure("DASHBOARD_ARCHIVED", 409);
  }
}
export function validateMockDashboard(spec: DashboardSpec) {
  const issues: Array<{ path: string; code: string; message: string }> = [];
  const add = (path: string, message: string) =>
    issues.push({ path, code: "DASHBOARD_INVALID", message });
  if (
    spec.schema_version !== "argus.telemetry_dashboard/v1" ||
    spec.layout.columns !== 12
  )
    add("layout", "Invalid schema or column count");
  if (spec.panels.length > 64 || spec.variables.length > 32)
    add("spec", "Configuration budget exceeded");
  const names = new Set<string>();
  for (const v of spec.variables) {
    if (!/^[A-Za-z_][A-Za-z0-9_]{0,63}$/.test(v.name) || names.has(v.name))
      add(`variables.${v.name}`, "Invalid variable name");
    names.add(v.name);
  }
  const visiting = new Set<string>(),
    visited = new Set<string>();
  const visit = (name: string) => {
    if (visiting.has(name)) {
      add("variables", "Cyclic dependency");
      return;
    }
    if (visited.has(name)) return;
    visiting.add(name);
    for (const filter of spec.variables.find((v) => v.name === name)?.query
      .filters ?? []) {
      if (filter.variable) {
        if (!names.has(filter.variable)) add("variables", "Undefined variable");
        else visit(filter.variable);
      }
    }
    visiting.delete(name);
    visited.add(name);
  };
  names.forEach(visit);
  const panelIds = new Set<string>();
  spec.panels.forEach((p, index) => {
    validateMockDisplay(p, `panels[${index}]`, add);
    if (!p.title.trim() || !p.id || panelIds.has(p.id))
      add(`panels.${p.id}`, "Panel identity and title required");
    panelIds.add(p.id);
    const r = p.layout;
    if (r.x < 0 || r.y < 0 || r.w < r.min_w || r.h < r.min_h || r.x + r.w > 12)
      add(`panels.${p.id}.layout`, "Invalid rectangle");
    for (const prior of spec.panels.slice(0, index)) {
      const q = prior.layout;
      if (
        r.x < q.x + q.w &&
        q.x < r.x + r.w &&
        r.y < q.y + q.h &&
        q.y < r.y + r.h
      )
        add(`panels.${p.id}.layout`, "Overlapping panels");
    }
    if (!p.targets.length) add(`panels.${p.id}`, "Query required");
    for (const target of p.targets) {
      const d = target.source_definition;
      if (Boolean(d.builder) === Boolean(d.dsl))
        add(`panels.${p.id}`, "Choose one query source");
      if (d.dsl && !d.dsl.expression.trim())
        add(`panels.${p.id}`, "Query required");
      if (d.builder && p.signal === "metrics" && !d.builder.metric)
        add(`panels.${p.id}`, "Metric required");
    }
  });
  return {
    valid: issues.length === 0,
    issues,
    compiler_version: "mock-structure-only/v1",
  };
}
