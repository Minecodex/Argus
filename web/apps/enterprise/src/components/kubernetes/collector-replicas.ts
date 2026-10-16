import type { KubernetesResource } from "@argus/api-client";

export function collectorReplicaStatus(
  items: KubernetesResource[],
  collectorId: string,
) {
  const workload = items.find(
    (item) =>
      item.resource_type === "daemonset" &&
      item.labels["argus.io/collector-id"] === collectorId,
  );
  const status = workload?.summary.status as
    Record<string, unknown> | undefined;
  const desired = status?.desiredNumberScheduled,
    ready = status?.numberReady;
  if (
    typeof desired !== "number" ||
    typeof ready !== "number" ||
    !Number.isSafeInteger(desired) ||
    !Number.isSafeInteger(ready) ||
    desired < 0 ||
    ready < 0 ||
    ready > desired
  )
    return undefined;
  return {
    desired,
    ready,
    percent: desired === 0 ? 0 : Math.round((100 * ready) / desired),
  };
}
