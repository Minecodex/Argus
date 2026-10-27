import type { components } from "./generated/kubernetesapi";
import type { PendingActionPublic } from "./types";
export type KubernetesMutationInput =
  components["schemas"]["KubernetesMutationInput"];
export type KubernetesRollbackReference =
  components["schemas"]["KubernetesRollbackReference"];
export type KubernetesOperation = components["schemas"]["KubernetesOperation"];
export type KubernetesHelmRelease =
  components["schemas"]["KubernetesHelmRelease"];
export type KubernetesHelmPreviewInput =
  components["schemas"]["KubernetesHelmPreviewInput"];
export type KubernetesInventorySchemas = components["schemas"];
export type KubernetesStreamInput =
  components["schemas"]["KubernetesStreamInput"];
export type KubernetesStreamSession =
  components["schemas"]["KubernetesStreamSession"];
export type KubernetesStreamTicket =
  components["schemas"]["KubernetesStreamTicket"];
export type KubernetesLogSessionInput =
  components["schemas"]["KubernetesLogSessionInput"];
export type KubernetesLogSession =
  components["schemas"]["KubernetesLogSession"];
export type KubernetesLogSessionBatch =
  components["schemas"]["KubernetesLogSessionBatch"];
export type KubernetesInventoryObject =
  KubernetesInventorySchemas["KubernetesInventoryObject"];
export type KubernetesInventoryDefinition =
  KubernetesInventorySchemas["KubernetesInventoryDefinition"];

export function kubernetesRollbackHistoryType(
  type: string,
): string | undefined {
  if (type === "deployment") return "replicaset";
  if (type === "statefulset" || type === "daemonset")
    return "controllerrevision";
  return undefined;
}

export function kubernetesRollbackReference(
  workload: KubernetesInventoryObject,
  history: KubernetesInventoryObject,
): KubernetesRollbackReference | undefined {
  if (
    history.definition.key !==
      kubernetesRollbackHistoryType(workload.definition.key) ||
    history.namespace !== workload.namespace ||
    history.namespace_uid !== workload.namespace_uid ||
    !history.owners.some(
      (owner) =>
        owner.controller &&
        owner.uid === workload.uid &&
        owner.kind === workload.definition.kind &&
        owner.name === workload.name &&
        owner.apiVersion === "apps/v1",
    )
  )
    return undefined;
  const metadata = history.data.metadata as Record<string, unknown> | undefined;
  const annotations = metadata?.annotations as
    Record<string, unknown> | undefined;
  const revision =
    history.definition.key === "replicaset"
      ? Number(annotations?.["deployment.kubernetes.io/revision"])
      : Number(history.data.revision);
  if (!Number.isSafeInteger(revision) || revision < 1) return undefined;
  return {
    name: history.name,
    uid: history.uid,
    resource_version: history.resource_version,
    revision,
  };
}

export type KubernetesContainerChoice = {
  name: string;
  kind: "containers" | "initContainers" | "ephemeralContainers";
};
export function kubernetesObjectContainers(
  object: KubernetesInventoryObject,
): KubernetesContainerChoice[] {
  const record = (value: unknown): Record<string, unknown> =>
    value !== null && typeof value === "object" && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : {};
  let spec = record(object.data.spec);
  if (object.definition.key === "cronjob")
    spec = record(record(spec.jobTemplate).spec);
  if (object.definition.key !== "pod")
    spec = record(record(spec.template).spec);
  const result: KubernetesContainerChoice[] = [];
  for (const kind of [
    "containers",
    "initContainers",
    "ephemeralContainers",
  ] as const) {
    const values = spec[kind];
    if (Array.isArray(values))
      for (const value of values) {
        const name = record(value).name;
        if (
          typeof name === "string" &&
          !result.some((row) => row.name === name)
        )
          result.push({ name, kind });
      }
  }
  return result;
}
export type KubernetesInventoryScope =
  KubernetesInventorySchemas["KubernetesInventoryScope"];
export type KubernetesDrainProgress =
  KubernetesInventorySchemas["KubernetesDrainProgress"];
export type KubernetesInventoryInput = {
  type: string;
  namespace_mode?: "all_authorized" | "explicit";
  namespace_ids?: string[];
  name?: string;
  labels?: string;
  limit?: number;
  cursor?: string;
};
export type KubernetesObjectInput = {
  type: string;
  name: string;
  uid: string;
  namespace_id?: string;
};
export interface KubernetesWorkbenchDomains {
  kubernetesWorkbench: {
    helmReleases(
      cluster: string,
      namespaceId: string,
      signal?: AbortSignal,
    ): Promise<{ items: KubernetesHelmRelease[] }>;
    helmHistory(
      cluster: string,
      namespaceId: string,
      name: string,
      signal?: AbortSignal,
    ): Promise<{ items: KubernetesHelmRelease[] }>;
    previewHelm(
      cluster: string,
      input: KubernetesHelmPreviewInput,
    ): Promise<PendingActionPublic>;
    createStreamSession(
      cluster: string,
      input: KubernetesStreamInput,
    ): Promise<KubernetesStreamTicket>;
    getStreamSession(
      cluster: string,
      id: string,
      signal?: AbortSignal,
    ): Promise<KubernetesStreamSession>;
    closeStreamSession(
      cluster: string,
      id: string,
    ): Promise<KubernetesStreamSession>;
    issueStreamTicket(
      cluster: string,
      id: string,
    ): Promise<KubernetesStreamTicket>;
    getLogSession(
      clusterId: string,
      sessionId: string,
      signal?: AbortSignal,
    ): Promise<KubernetesLogSession>;
    createLogSession(
      clusterId: string,
      input: KubernetesLogSessionInput,
    ): Promise<components["schemas"]["KubernetesLogSessionTicket"]>;
    attachLogSession(
      clusterId: string,
      sessionId: string,
      ticket: string,
    ): Promise<KubernetesLogSession>;
    readLogSession(
      clusterId: string,
      sessionId: string,
      cursor?: string,
      signal?: AbortSignal,
    ): Promise<KubernetesLogSessionBatch>;
    closeLogSession(
      clusterId: string,
      sessionId: string,
    ): Promise<KubernetesLogSession>;
    cancelDrain(
      clusterId: string,
      id: string,
      expectedVersion: number,
    ): Promise<KubernetesOperation>;
    preview(
      clusterId: string,
      input: KubernetesMutationInput,
    ): Promise<PendingActionPublic>;
    operations(
      clusterId: string,
      input?: { limit?: number; cursor?: string },
      signal?: AbortSignal,
    ): Promise<KubernetesInventorySchemas["KubernetesOperationPage"]>;
    operation(
      clusterId: string,
      id: string,
      signal?: AbortSignal,
    ): Promise<KubernetesOperation>;
    watch(
      clusterId: string,
      token: string,
      signal?: AbortSignal,
    ): Promise<KubernetesInventorySchemas["KubernetesInventoryWatchResult"]>;
    scope(
      clusterId: string,
      signal?: AbortSignal,
    ): Promise<KubernetesInventoryScope>;
    types(
      clusterId: string,
      signal?: AbortSignal,
    ): Promise<KubernetesInventorySchemas["KubernetesInventoryTypes"]>;
    list(
      clusterId: string,
      input: KubernetesInventoryInput,
      signal?: AbortSignal,
    ): Promise<KubernetesInventorySchemas["KubernetesInventoryPage"]>;
    object(
      clusterId: string,
      input: KubernetesObjectInput,
      signal?: AbortSignal,
    ): Promise<KubernetesInventoryObject>;
  };
}
