import { expect, it } from "vitest";
import { createMockApiClient } from "./mock";

it("requires separate approval, retains creator scope and stops later drain batches", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  await api.auth.login({ username: "root", password: "123456" });
  const cluster = "k8s-prod-east";
  const node = (await api.kubernetesWorkbench.list(cluster, { type: "node" }))
    .items[0]!;
  const pending = await api.kubernetesWorkbench.preview(cluster, {
    operation: "drain",
    type: "node",
    name: node.name,
    uid: node.uid,
    expected_resource_version: node.resource_version,
  });
  expect(pending.risk).toBe("dangerous");
  expect(JSON.stringify(pending.preview)).toContain("pod-example");
  expect(
    pending.diff
      .filter((line) => line.kind === "remove")
      .map((line) => line.text)
      .join("\n"),
  ).not.toContain(node.uid);
  await api.approvals.confirm(pending.action_ref);
  await expect(
    api.approvals.approve(pending.action_ref, "self approval"),
  ).rejects.toThrow("separation of duty");
  await api.auth.login({ username: "chenxi", password: "123456" });
  await api.approvals.approve(
    pending.action_ref,
    "Reviewed maintenance window",
  );
  // The approver has approval privileges but does not inherit the creator's
  // Cluster data grant. Execution must still use the frozen creator scope.
  await expect(api.kubernetesWorkbench.operations(cluster)).rejects.toThrow();
  await api.auth.login({ username: "root", password: "123456" });
  const row = (await api.kubernetesWorkbench.operations(cluster)).items.find(
    (o) => o.operation === "drain",
  )!;
  expect(row.status).toBe("observing");
  expect(row.result.drain?.pods).toHaveLength(2);
  await expect(
    api.kubernetesWorkbench.cancelDrain(cluster, row.id, row.version + 1),
  ).rejects.toThrow();
  const cancelled = await api.kubernetesWorkbench.cancelDrain(
    cluster,
    row.id,
    row.version,
  );
  expect(cancelled.status).toBe("cancelled");
  expect(
    (await api.kubernetesWorkbench.cancelDrain(cluster, row.id, row.version))
      .version,
  ).toBe(cancelled.version);
  const current = await api.kubernetesWorkbench.object(cluster, {
    type: "node",
    name: node.name,
    uid: node.uid,
  });
  expect(current.data.spec).toMatchObject({ unschedulable: true });
});
