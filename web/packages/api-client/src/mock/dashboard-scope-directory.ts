import { requireObservationKind } from "./observation-access";
import type { DashboardDomains, DashboardSchemas } from "../dashboard";
import type { BaseContext } from "./context";
import { authorizationCatalog } from "./authorization-catalog";
import {
  dashboardObjectGrant,
  dashboardPermission,
  dashboardFailure as fail,
} from "./dashboard-state";
import { ensureKubernetesInventory } from "./kubernetes-seed";

export function mockDashboardScopeDirectory(
  ctx: BaseContext,
  resourceQuery = false,
): DashboardDomains["dashboards"]["scopeResources"] {
  return async (input) => {
    if(resourceQuery)requireObservationKind(ctx,input.kind);else dashboardPermission(ctx);
    const limit = input.limit ?? 30,
      search = input.search?.trim() ?? "";
    if (limit < 1 || limit > 100 || search.length > 200) fail();
    if (input.kind === "kubernetes_namespace")
      for (const cluster of ctx.db.clusters.filter(
        (row) => row.enterpriseId === ctx.enterpriseId(),
      ))
        ensureKubernetesInventory(ctx, cluster.id);
    const binding = JSON.stringify({
      tenant: ctx.enterpriseId(),
      actor: ctx.actor().id,
      version:
        ctx.db.dataAuthorizationVersions?.[`user:${ctx.actor().id}`] ?? 1,
      kind: input.kind,
      search,
    });
    let after = "";
    if (input.cursor) {
      try {
        const cursor = JSON.parse(decodeURIComponent(input.cursor)) as {
          binding: string;
          after: string;
        };
        if (cursor.binding !== binding || !cursor.after) fail();
        after = cursor.after;
      } catch {
        fail();
      }
    }
    const rows = authorizationCatalog(ctx, input.kind)
      .filter(
        (row) =>
          dashboardObjectGrant(ctx, input.kind, row.id) ||
          (input.kind === "kubernetes_namespace" &&
            !!row.parent_id &&
            dashboardObjectGrant(ctx, "kubernetes_cluster", row.parent_id)),
      )
      .filter(
        (row) =>
          row.id.localeCompare(after) > 0 &&
          `${row.name} ${row.parent_name ?? ""} ${row.source_uid ?? ""} ${row.id}`
            .toLowerCase()
            .includes(search.toLowerCase()),
      )
      .sort((a, b) => a.id.localeCompare(b.id));
    const items: DashboardSchemas["DashboardScopeDirectoryItem"][] = rows
      .slice(0, limit)
      .map((row) => ({
        ...row,
        type: input.kind,
        environment: row.environment ?? "",
      }));
    return {
      items,
      ...(rows.length > limit
        ? {
            next_cursor: encodeURIComponent(
              JSON.stringify({ binding, after: items.at(-1)!.id }),
            ),
          }
        : {}),
    };
  };
}
