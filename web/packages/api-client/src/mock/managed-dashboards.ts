import type { BaseContext } from "./context";
import type { DashboardSpec, DashboardRevision } from "../dashboard";
import { managedDashboardTemplates } from "../generated/dashboard-templates";
import {
  dashboardObjectGrant,
  dashboardState,
  validateMockDashboard,
} from "./dashboard-state";

// Mock definitions share the locked Go-generated contract; sample data remains
// explicitly labelled by the existing mock query backend.
export function ensureManagedDashboards(ctx: BaseContext) {
  const state = dashboardState(ctx),
    targets = ctx.db.monitoring?.[ctx.enterpriseId()]?.targets ?? [];
  const own = ctx.db.monitoring?.[ctx.enterpriseId()]?.collections ?? [];
  for (const template of managedDashboardTemplates) {
    if (
      !own.some(
        (row) =>
          row.status === "active" &&
          template.integration_keys.some((key) => key === row.integration) &&
          dashboardObjectGrant(ctx, row.target.kind, row.target.id),
      ) &&
      !targets.some(
        (target) =>
          target.status === "active" &&
          template.integration_keys.some((key) => key === target.engine) &&
          dashboardObjectGrant(ctx, "monitoring_target", target.id),
      )
    )
      continue;
    const current = state.items.find(
      (row) =>
        row.enterprise === ctx.enterpriseId() &&
        row.value.managed_template_key === template.key,
    );
    if (current?.value.managed_template_version === template.version) continue;
    const id = current?.value.id ?? crypto.randomUUID(),
      revisionId = crypto.randomUUID(),
      spec = structuredClone(template.spec) as unknown as DashboardSpec;
    const revision: DashboardRevision = {
      id: revisionId,
      dashboard_id: id,
      revision_number:
        state.revisions.filter((row) => row.dashboard_id === id).length + 1,
      name: template.name,
      description: template.description,
      spec,
      spec_hash: revisionId,
      validation: validateMockDashboard(spec),
      sample: { status: "not_run", basis: "system_definition_has_no_samples" },
      created_at: ctx.nowIso(),
    };
    state.revisions.push(revision);
    const item = {
      id,
      name: template.name,
      description: template.description,
      active_revision_id: revisionId,
      version: (current?.value.version ?? 0) + 1,
      lifecycle: "active" as const,
      updated_at: ctx.nowIso(),
      managed_template_key: template.key,
      managed_template_version: template.version,
    };
    if (current) current.value = item;
    else
      state.items.push({
        enterprise: ctx.enterpriseId(),
        creator: "system",
        value: item,
      });
    ctx.save();
  }
}
