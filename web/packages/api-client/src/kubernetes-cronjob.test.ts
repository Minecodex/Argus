import { expect, it } from "vitest";
import { createMockApiClient } from "./mock";

it("creates a separate Job from a frozen, scoped CronJob only after confirmation", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  await api.auth.login({ username: "root", password: "123456" });
  const cluster = "k8s-prod-east";
  const namespaces = await api.kubernetesWorkbench.list(cluster, {
    type: "namespace",
    limit: 30,
  });
  const ns = namespaces.items.find((row) => row.name === "production")!;
  const namespaceId = namespaces.namespace_references[ns.uid]!;
  const selection = {
    namespace_mode: "explicit" as const,
    namespace_ids: [namespaceId],
    limit: 30,
  };
  const source = (
    await api.kubernetesWorkbench.list(cluster, {
      ...selection,
      type: "cronjob",
    })
  ).items[0]!;
  const input = {
    operation: "create" as const,
    type: "job",
    name: "manual-job",
    namespace_id: namespaceId,
    cronjob_source: {
      name: source.name,
      uid: source.uid,
      resource_version: source.resource_version,
    },
  };
  await expect(
    api.kubernetesWorkbench.preview(cluster, {
      ...input,
      cronjob_source: { ...input.cronjob_source, uid: "replacement" },
    }),
  ).rejects.toThrow();
  const pending = await api.kubernetesWorkbench.preview(cluster, input);
  expect(
    (
      await api.kubernetesWorkbench.list(cluster, {
        ...selection,
        type: "job",
        name: "manual-job",
      })
    ).items,
  ).toHaveLength(0);
  await api.approvals.confirm(pending.action_ref);
  const created = (
    await api.kubernetesWorkbench.list(cluster, {
      ...selection,
      type: "job",
      name: "manual-job",
    })
  ).items[0]!;
  expect(created.owners).toMatchObject([{ kind: "CronJob", uid: source.uid }]);
  expect(created.data.spec).toEqual(
    (
      (source.data.spec as Record<string, unknown>).jobTemplate as Record<
        string,
        unknown
      >
    ).spec,
  );
  const unchanged = await api.kubernetesWorkbench.object(cluster, {
    type: "cronjob",
    name: source.name,
    uid: source.uid,
    namespace_id: namespaceId,
  });
  expect(unchanged.resource_version).toBe(source.resource_version);
});
