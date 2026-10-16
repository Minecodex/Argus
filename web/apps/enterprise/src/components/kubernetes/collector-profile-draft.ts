import type { CollectionClaim, CollectionProfile } from "@argus/api-client";

export const CLUSTER_PROFILE_KEYS = [
  "nodeContainer",
  "clusterMetrics",
  "otlpGateway",
] as const;
export type ClusterProfileKey = (typeof CLUSTER_PROFILE_KEYS)[number];
export const CLUSTER_PROFILE_CATALOG: Record<ClusterProfileKey, string> = {
  nodeContainer: "k8s-node-container",
  clusterMetrics: "k8s-cluster",
  otlpGateway: "k8s-otlp-gateway",
};
export function clusterProfileBaseline(
  collector: string,
  claims: CollectionClaim[],
  catalog: CollectionProfile[],
): Record<ClusterProfileKey, boolean> {
  const configured = new Set(
    claims
      .filter(
        (c) =>
          c.collector_id === collector &&
          (c.status === "active" || c.status === "conflict"),
      )
      .map((c) => c.profile_id),
  );
  return Object.fromEntries(
    CLUSTER_PROFILE_KEYS.map((key) => [
      key,
      catalog.some(
        (p) => p.key === CLUSTER_PROFILE_CATALOG[key] && configured.has(p.id),
      ),
    ]),
  ) as Record<ClusterProfileKey, boolean>;
}
export function clusterProfileIds(
  draft: Record<ClusterProfileKey, boolean>,
  catalog: CollectionProfile[],
): string[] {
  const keys = new Set(
    CLUSTER_PROFILE_KEYS.filter((key) => draft[key]).map(
      (key) => CLUSTER_PROFILE_CATALOG[key],
    ),
  );
  return catalog
    .filter((profile) => keys.has(profile.key))
    .map((profile) => profile.id);
}
