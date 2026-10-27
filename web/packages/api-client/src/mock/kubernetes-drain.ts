import type {
  KubernetesDrainProgress,
  KubernetesInventoryObject,
} from "../kubernetes-workbench";
import type { MockKubernetesInventory } from "./kubernetes-workbench";

export function mockNodeMaintenanceData(
  data: Record<string, unknown>,
  operation: "drain" | "cordon" | "uncordon",
): Record<string, unknown> {
  return {
    ...structuredClone(data),
    spec: {
      ...((data.spec as Record<string, unknown>) ?? {}),
      unschedulable: operation !== "uncordon",
    },
  };
}

// Mock mode models UI state only. Production impact comes from the native API
// snapshot, PDB admission and UID-bound evictions, never this adapter.
export function mockDrainProgress(
  state: MockKubernetesInventory,
  node: KubernetesInventoryObject,
): KubernetesDrainProgress {
  const pods = state.objects
    .filter(
      (o) =>
        o.definition.key === "pod" &&
        (o.data.spec as Record<string, unknown>)?.nodeName === node.name,
    )
    .map((p) => {
      const metadata = p.data.metadata as Record<string, unknown>;
      const annotations = metadata.annotations as
        Record<string, string> | undefined;
      const spec = p.data.spec as Record<string, unknown>;
      const owner = p.owners.find((o) => o.controller);
      const disposition = annotations?.["kubernetes.io/config.mirror"]
        ? "mirror"
        : owner?.kind === "DaemonSet"
          ? "daemonset"
          : !owner
            ? "unmanaged"
            : undefined;
      const volumes = (Array.isArray(spec.volumes) ? spec.volumes : []).flatMap(
        (v: Record<string, unknown>) =>
          Object.keys(v)
            .filter(
              (k) =>
                ![
                  "name",
                  "projected",
                  "secret",
                  "configMap",
                  "downwardAPI",
                  "image",
                ].includes(k),
            )
            .map((kind) => ({ name: String(v.name), kind, local: true })),
      );
      const blocked = disposition === "unmanaged" || volumes.length > 0;
      return {
        namespace: p.namespace,
        namespace_uid: p.namespace_uid ?? "",
        name: p.name,
        uid: p.uid,
        state: blocked
          ? ("blocked" as const)
          : disposition
            ? ("skipped" as const)
            : ("pending" as const),
        reason: blocked ? (disposition ?? "local_storage") : disposition,
        volumes,
        budgets: [],
      };
    });
  return {
    phase: pods.some((p) => p.state === "blocked")
      ? "blocked"
      : pods.some((p) => p.state === "pending")
        ? "draining"
        : "complete",
    pods,
  };
}
