import type { KubernetesInventoryDefinition } from "@argus/api-client";
const formKinds = new Set([
  "pod",
  "deployment",
  "statefulset",
  "daemonset",
  "replicaset",
  "job",
  "cronjob",
  "service",
  "ingress",
  "ingressclass",
  "hpa",
  "pdb",
  "networkpolicy",
  "pvc",
  "resourcequota",
  "limitrange",
  "namespace",
  "configmap",
  "secret",
  "role",
  "clusterrole",
  "rolebinding",
  "clusterrolebinding",
  "serviceaccount",
]);
export const hasResourceForm = (kind: string) => formKinds.has(kind);
export function createResourceDraft(
  definition: KubernetesInventoryDefinition,
  namespace?: string,
): Record<string, unknown> {
  const draft: Record<string, unknown> = {
    apiVersion: definition.group
      ? definition.group + "/" + definition.version
      : definition.version,
    kind: definition.kind,
    metadata: { name: "", ...(namespace ? { namespace } : {}) },
  };
  const container = { name: "app", image: "" };
  const template = {
    metadata: { labels: {} },
    spec: { containers: [container] },
  };
  if (["deployment", "replicaset", "statefulset"].includes(definition.key))
    draft.spec = { replicas: 1, selector: { matchLabels: {} }, template };
  if (definition.key === "daemonset")
    draft.spec = { selector: { matchLabels: {} }, template };
  if (definition.key === "pod") draft.spec = { containers: [container] };
  if (definition.key === "job")
    draft.spec = {
      template: {
        ...template,
        spec: { ...template.spec, restartPolicy: "Never" },
      },
      backoffLimit: 3,
    };
  if (definition.key === "cronjob")
    draft.spec = {
      schedule: "",
      concurrencyPolicy: "Forbid",
      jobTemplate: {
        spec: {
          template: {
            ...template,
            spec: { ...template.spec, restartPolicy: "Never" },
          },
        },
      },
    };
  if (definition.key === "service")
    draft.spec = {
      type: "ClusterIP",
      selector: {},
      ports: [{ port: 80, targetPort: 80, protocol: "TCP" }],
    };
  if (definition.key === "pvc")
    draft.spec = {
      accessModes: ["ReadWriteOnce"],
      resources: { requests: { storage: "1Gi" } },
    };
  if (definition.key === "secret") draft.type = "Opaque";
  return draft;
}

export function suggestWorkloadSelector(
  before: Record<string, unknown>,
  next: Record<string, unknown>,
  kind: string,
): Record<string, unknown> {
  if (!["deployment", "replicaset", "statefulset", "daemonset"].includes(kind))
    return next;
  const record = (v: unknown) =>
    v && typeof v === "object" && !Array.isArray(v)
      ? (v as Record<string, unknown>)
      : {};
  const name = record(next.metadata).name,
    old = record(before.metadata).name;
  if (typeof name !== "string" || !name) return next;
  const spec = record(next.spec),
    selector = record(spec.selector),
    labels = record(selector.matchLabels),
    template = record(spec.template),
    metadata = record(template.metadata),
    podLabels = record(metadata.labels);
  if (
    (!Object.keys(labels).length && !Object.keys(podLabels).length) ||
    (Object.keys(labels).length === 1 &&
      labels.app === old &&
      podLabels.app === old)
  ) {
    return {
      ...next,
      spec: {
        ...spec,
        selector: { ...selector, matchLabels: { ...labels, app: name } },
        template: {
          ...template,
          metadata: { ...metadata, labels: { ...podLabels, app: name } },
        },
      },
    };
  }
  return next;
}
