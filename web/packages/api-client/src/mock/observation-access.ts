import type { BaseContext } from "./context";
import type { ObservationResourceRef, ObservationScope } from "../observation";
import { authorizationCatalog } from "./authorization-catalog";
import {
  dashboardObjectGrant,
  dashboardFailure as fail,
} from "./dashboard-state";
import { resolvePermissions } from "./permissions";

export const observationKinds: ObservationResourceRef["type"][] = [
  "host",
  "kubernetes_cluster",
  "kubernetes_namespace",
  "application",
  "monitoring_target",
];
const permission = (kind: ObservationResourceRef["type"]) =>
  ({
    host: "host.read",
    kubernetes_cluster: "kubernetes.read",
    kubernetes_namespace: "kubernetes.read",
    application: "application.read",
    monitoring_target: "monitoring.read",
  })[kind];
export function canReadObservationKind(
  ctx: BaseContext,
  kind: ObservationResourceRef["type"],
) {
  const permissions = resolvePermissions(
    ctx.db,
    ctx.enterpriseId(),
    ctx.actor().id,
    ctx.nowIso(),
  );
  return permissions.has("*") || permissions.has(permission(kind));
}
export function requireObservationKind(
  ctx: BaseContext,
  kind: ObservationResourceRef["type"],
) {
  if (!canReadObservationKind(ctx, kind)) fail("AUTHORIZATION_DENIED", 403);
}
export function resolveObservationResources(
  ctx: BaseContext,
  scope?: ObservationScope,
  legacy?: string[],
): ObservationResourceRef[] {
  if (scope && legacy !== undefined) fail("QUERY_INVALID", 400);
  const all = observationKinds.flatMap((type) =>
    canReadObservationKind(ctx, type)
      ? authorizationCatalog(ctx, type)
          .filter(
            (r) =>
              dashboardObjectGrant(ctx, type, r.id) ||
              (type === "kubernetes_namespace" &&
                !!r.parent_id &&
                dashboardObjectGrant(ctx, "kubernetes_cluster", r.parent_id)),
          )
          .map((r) => ({ type, id: r.id }))
      : [],
  );
  if (!scope) {
    if (!legacy?.length) fail("QUERY_SCOPE_DENIED", 403);
    const refs = all.filter((r) => legacy.includes(r.id));
    if (refs.length !== legacy.length) fail("QUERY_SCOPE_DENIED", 403);
    return refs;
  }
  if (scope.mode === "all_authorized") {
    if (scope.resources.length) fail("QUERY_INVALID", 400);
    return all.filter(
      (r) =>
        !scope.resource_types?.length || scope.resource_types.includes(r.type),
    );
  }
  if (scope.mode !== "explicit" || scope.resources.length > 1000)
    fail("QUERY_INVALID", 400);
  const seen = new Set<string>();
  for (const ref of scope.resources) {
    if (
      seen.has(ref.id) ||
      !all.some((r) => r.id === ref.id && r.type === ref.type)
    )
      fail("QUERY_SCOPE_DENIED", 403);
    seen.add(ref.id);
  }
  return structuredClone(scope.resources);
}
