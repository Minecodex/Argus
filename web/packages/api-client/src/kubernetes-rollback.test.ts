import { expect, it } from "vitest";
import { createMockApiClient } from "./mock";
import { kubernetesRollbackReference } from "./kubernetes-workbench";

it.each(["deployment", "statefulset", "daemonset"])(
  "freezes only controller-owned %s history and restores its template through confirmation",
  async (type) => {
    const api = createMockApiClient({ persist: false, delay: 0 });
    await api.auth.login({ username: "root", password: "123456" });
    const cluster = "k8s-prod-east";
    const namespaces = await api.kubernetesWorkbench.list(cluster, {
      type: "namespace",
      limit: 30,
    });
    const namespace = namespaces.items.find(
      (row) => row.name === "production",
    )!;
    const namespaceId = namespaces.namespace_references[namespace.uid]!;
    const selection = {
      namespace_mode: "explicit" as const,
      namespace_ids: [namespaceId],
      limit: 30,
    };
    const workload = (
      await api.kubernetesWorkbench.list(cluster, { ...selection, type })
    ).items[0]!;
    const history = (
      await api.kubernetesWorkbench.list(cluster, {
        ...selection,
        type: type === "deployment" ? "replicaset" : "controllerrevision",
      })
    ).items.find((row) => kubernetesRollbackReference(workload, row))!;
    const ref = kubernetesRollbackReference(workload, history)!;
    expect(ref).toMatchObject({
      uid: history.uid,
      resource_version: "1",
      revision: 1,
    });
    expect(
      kubernetesRollbackReference(workload, {
        ...history,
        namespace_uid: "replacement",
      }),
    ).toBeUndefined();
    expect(
      kubernetesRollbackReference(workload, {
        ...history,
        owners: history.owners.map((owner) => ({
          ...owner,
          uid: "another-controller",
        })),
      }),
    ).toBeUndefined();
    const input = {
      operation: "rollback" as const,
      type,
      name: workload.name,
      uid: workload.uid,
      expected_resource_version: workload.resource_version,
      namespace_id: namespaceId,
      rollback: ref,
    };
    await expect(
      api.kubernetesWorkbench.preview(cluster, {
        ...input,
        rollback: { ...ref, resource_version: "stale" },
      }),
    ).rejects.toThrow();
    const action = await api.kubernetesWorkbench.preview(cluster, input);
    const before = await api.kubernetesWorkbench.object(cluster, {
      type,
      name: workload.name,
      uid: workload.uid,
      namespace_id: namespaceId,
    });
    expect(JSON.stringify(before.data)).not.toContain("alpine:3.20");
    expect(JSON.stringify(action.diff)).toContain("alpine:3.20");
    await api.approvals.confirm(action.action_ref);
    const after = await api.kubernetesWorkbench.object(cluster, {
      type,
      name: workload.name,
      uid: workload.uid,
      namespace_id: namespaceId,
    });
    expect(JSON.stringify(after.data)).toContain("alpine:3.20");
    expect(JSON.stringify(after.data)).not.toContain("$patch");
    expect(after.uid).toBe(workload.uid);
  },
);
