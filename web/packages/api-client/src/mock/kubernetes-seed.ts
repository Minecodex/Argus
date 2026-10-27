import type { BaseContext } from "./context";
import type {
  KubernetesInventoryObject,
  KubernetesInventoryDefinition,
} from "../kubernetes-workbench";
import { kubernetesInventoryTypes } from "../generated/kubernetes-inventory-registry";

export function ensureKubernetesInventory(ctx: BaseContext, id: string) {
  const definitions = structuredClone(
    kubernetesInventoryTypes,
  ) as unknown as KubernetesInventoryDefinition[];
  const registry = (ctx.db.kubernetesInventory ??= {});
  if (registry[id]) return registry[id]!;
  const namespaces = ["production", "staging"].map((name) => ({
    id: crypto.randomUUID(),
    uid: crypto.randomUUID(),
    name,
    version: 1,
  }));
  const objects: KubernetesInventoryObject[] = [];
  for (const definition of definitions) {
    const scopes = definition.namespaced ? namespaces : [undefined];
    for (const namespace of scopes) {
      const name =
        definition.key === "namespace" ? "" : `${definition.key}-example`;
      if (definition.key === "namespace") continue;
      const uid = crypto.randomUUID(),
        at = ctx.nowIso();
      const data: Record<string, unknown> = {
        apiVersion: definition.group
          ? `${definition.group}/${definition.version}`
          : definition.version,
        kind: definition.kind,
        metadata: {
          name,
          ...(namespace ? { namespace: namespace.name } : {}),
          uid,
          resourceVersion: "1",
          labels: { app: "example" },
        },
        status: { phase: "Active" },
      };
      if (definition.key === "secret") data.data = { password: "[REDACTED]" };
      if (
        [
          "pod",
          "deployment",
          "replicaset",
          "daemonset",
          "statefulset",
          "job",
          "cronjob",
        ].includes(definition.key)
      ) {
        const spec = {
          containers: [{ name: "main" }, { name: "sidecar" }],
          initContainers: [{ name: "init" }],
        };
        data.spec =
          definition.key === "pod"
            ? spec
            : definition.key === "cronjob"
              ? {
                  schedule: "0 * * * *",
                  suspend: true,
                  jobTemplate: {
                    spec: {
                      template: {
                        spec: {
                          ...spec,
                          restartPolicy: "Never",
                          automountServiceAccountToken: false,
                        },
                      },
                    },
                  },
                }
              : { template: { spec } };
        if (definition.key === "pod")
          data.status = {
            phase: "Running",
            containerStatuses: spec.containers.map((c) => ({
              name: c.name,
              containerID: `containerd://${uid}-${c.name}`,
              restartCount: 1,
              state: { running: { startedAt: at } },
              lastState: {
                terminated: {
                  containerID: `containerd://${uid}-${c.name}-previous`,
                  exitCode: 0,
                },
              },
            })),
            initContainerStatuses: [
              {
                name: "init",
                containerID: `containerd://${uid}-init`,
                restartCount: 0,
                state: { terminated: { exitCode: 0 } },
              },
            ],
          };
      }
      objects.push({
        definition,
        uid,
        name,
        namespace: namespace?.name ?? "",
        namespace_uid: namespace?.uid,
        resource_version: "1",
        generation: 1,
        labels: { app: "example" },
        owners: [],
        created_at: at,
        observed_at: at,
        redacted: definition.key === "secret",
        data,
      });
    }
  }
  for (const workload of [...objects]) {
    if (
      !["deployment", "statefulset", "daemonset"].includes(
        workload.definition.key,
      )
    )
      continue;
    const historyType =
      workload.definition.key === "deployment"
        ? "replicaset"
        : "controllerrevision";
    const historyDefinition = definitions.find(
      (item) => item.key === historyType,
    )!;
    const uid = crypto.randomUUID(),
      name = workload.name + "-revision-1";
    const template = {
      metadata: { labels: { app: "example" } },
      spec: {
        automountServiceAccountToken: false,
        containers: [{ name: "main", image: "alpine:3.20" }],
      },
    };
    const owners = [
      {
        apiVersion: "apps/v1",
        kind: workload.definition.kind,
        name: workload.name,
        uid: workload.uid,
        controller: true,
      },
    ];
    objects.push({
      ...structuredClone(workload),
      definition: historyDefinition,
      uid,
      name,
      owners,
      data: {
        apiVersion: "apps/v1",
        kind: historyDefinition.kind,
        metadata: {
          name,
          namespace: workload.namespace,
          uid,
          resourceVersion: "1",
          ownerReferences: owners,
          annotations: { "deployment.kubernetes.io/revision": "1" },
        },
        ...(historyType === "replicaset"
          ? { spec: { template } }
          : {
              revision: 1,
              data: { spec: { template: { ...template, $patch: "replace" } } },
            }),
      },
    });
  }
  const definition = definitions.find((v) => v.key === "namespace")!;
  for (const namespace of namespaces) {
    const at = ctx.nowIso();
    objects.push({
      definition,
      uid: namespace.uid,
      name: namespace.name,
      namespace: "",
      resource_version: "1",
      generation: 1,
      labels: {},
      owners: [],
      created_at: at,
      observed_at: at,
      redacted: false,
      data: {
        apiVersion: "v1",
        kind: "Namespace",
        metadata: {
          name: namespace.name,
          uid: namespace.uid,
          resourceVersion: "1",
        },
        status: { phase: "Active" },
      },
    });
  }
  const node = objects.find((o) => o.definition.key === "node");
  if (node) {
    node.data.spec = { unschedulable: false };
    for (const pod of objects.filter((o) => o.definition.key === "pod")) {
      (pod.data.spec as Record<string, unknown>).nodeName = node.name;
      const controller = objects.find(
        (o) =>
          o.definition.key === "replicaset" &&
          o.namespace_uid === pod.namespace_uid,
      );
      if (controller) {
        pod.owners = [
          {
            apiVersion: "apps/v1",
            kind: "ReplicaSet",
            name: controller.name,
            uid: controller.uid,
            controller: true,
          },
        ];
        (pod.data.metadata as Record<string, unknown>).ownerReferences =
          structuredClone(pod.owners);
      }
    }
  }
  registry[id] = { namespaces, objects };
  ctx.save();
  return registry[id]!;
}
