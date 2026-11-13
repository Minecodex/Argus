import type { DashboardSchemas } from "@argus/api-client";
export type KubernetesDashboardSearch = {
  cluster_scope?: string;
  namespace_scope?: string;
};
// Keep the full authorized namespace set in a copied/expanded link. The server
// validates every reference again; a malformed scope must never become cluster-wide.
export function parseKubernetesDashboardScope(
  search: Record<string, unknown>,
): KubernetesDashboardSearch {
  return {
    cluster_scope:
      typeof search.cluster_scope === "string"
        ? search.cluster_scope
        : undefined,
    namespace_scope:
      typeof search.namespace_scope === "string"
        ? search.namespace_scope
        : undefined,
  };
}
export function dashboardKubernetesScope(search: KubernetesDashboardSearch) {
  if (search.cluster_scope === undefined) return undefined;
  const namespaceIds =
    search.namespace_scope === undefined || search.namespace_scope === ""
      ? []
      : search.namespace_scope.split(",");
  return {
    resource_mode: "explicit" as const,
    resource_ids: namespaceIds.length ? namespaceIds : [search.cluster_scope],
    resource_types: namespaceIds.length
      ? ["kubernetes_namespace" as const]
      : ["kubernetes_cluster" as const],
    kubernetes_scope: {
      cluster_id: search.cluster_scope,
      namespace_ids: namespaceIds,
    },
  };
}
export function dashboardKubernetesSearch(
  scope: NonNullable<
    DashboardSchemas["DashboardExecutionInput"]["kubernetes_scope"]
  >,
): KubernetesDashboardSearch {
  return {
    cluster_scope: scope.cluster_id,
    namespace_scope: scope.namespace_ids?.join(",") ?? "",
  };
}
