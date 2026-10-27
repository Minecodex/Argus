import { expect, it } from "vitest";
import { createMockApiClient } from "./mock";

it("keeps repository and OCI credentials out of approvals and persists only source metadata", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  await api.auth.login({ username: "root", password: "123456" });
  const cluster = "k8s-prod-east";
  const page = await api.kubernetesWorkbench.list(cluster, {
    type: "namespace",
    limit: 30,
  });
  const ns =
    page.namespace_references[
      page.items.find((r) => r.name === "production")!.uid
    ]!;
  for (const kind of ["repository", "oci"] as const) {
    const action = await api.kubernetesWorkbench.previewHelm(cluster, {
      namespace_id: ns,
      name: `source-${kind}`,
      operation: "install",
      expected_revision: 0,
      timeout_seconds: 120,
      reason: "Verify source metadata",
      source: {
        kind,
        url:
          kind === "repository"
            ? "https://charts.example/team"
            : "oci://registry.example/team/sample",
        chart: kind === "repository" ? "sample" : undefined,
        version: "1.0.0",
      },
      source_credentials: {
        username: "reader",
        password: "source-private-credential-canary",
        ca_pem: "public-ca-fixture-canary",
      },
      values: "password: values-private-canary",
    });
    const serialized = JSON.stringify(action);
    for (const value of [
      "source-private-credential-canary",
      "values-private-canary",
      "public-ca-fixture-canary",
      "source_credentials",
    ])
      expect(serialized).not.toContain(value);
    expect(serialized).toContain("not-verified-in-mock");
    await api.approvals.confirm(action.action_ref);
    expect(
      (await api.kubernetesWorkbench.helmReleases(cluster, ns)).items.find(
        (r) => r.name === `source-${kind}`,
      ),
    ).toMatchObject({
      chart: "sample",
      chart_version: "1.0.0",
      status: "deployed",
    });
  }
});

it("keeps protected Helm inputs out of persisted plans and applies revision-bound lifecycle after confirmation", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  await api.auth.login({ username: "root", password: "123456" });
  const cluster = "k8s-prod-east",
    page = await api.kubernetesWorkbench.list(cluster, {
      type: "namespace",
      limit: 30,
    }),
    namespace =
      page.namespace_references[
        page.items.find((r) => r.name === "production")!.uid
      ]!;
  const input = {
    namespace_id: namespace,
    name: "owned-release",
    operation: "install" as const,
    expected_revision: 0,
    timeout_seconds: 120,
    reason: "owned test",
    chart_archive: btoa("protected-chart-canary"),
    values: "password: protected-value-canary",
  };
  const install = await api.kubernetesWorkbench.previewHelm(cluster, input);
  expect(JSON.stringify(install)).not.toContain("protected-value-canary");
  expect(
    (await api.kubernetesWorkbench.helmReleases(cluster, namespace)).items,
  ).toHaveLength(0);
  await api.approvals.confirm(install.action_ref);
  const upgrade = await api.kubernetesWorkbench.previewHelm(cluster, {
    ...input,
    operation: "upgrade",
    expected_revision: 1,
  });
  await api.approvals.confirm(upgrade.action_ref);
  await expect(
    api.kubernetesWorkbench.previewHelm(cluster, {
      ...input,
      operation: "upgrade",
      expected_revision: 1,
    }),
  ).rejects.toThrow();
  const rollback = await api.kubernetesWorkbench.previewHelm(cluster, {
    ...input,
    chart_archive: undefined,
    values: undefined,
    operation: "rollback",
    expected_revision: 2,
    rollback_revision: 1,
  });
  await api.approvals.confirm(rollback.action_ref);
  expect(
    (await api.kubernetesWorkbench.helmReleases(cluster, namespace)).items,
  ).toMatchObject([{ revision: 3, status: "deployed" }]);
  expect(
    (await api.kubernetesWorkbench.helmHistory(cluster, namespace, input.name))
      .items,
  ).toHaveLength(3);
  const uninstall = await api.kubernetesWorkbench.previewHelm(cluster, {
    ...input,
    chart_archive: undefined,
    values: undefined,
    operation: "uninstall",
    expected_revision: 3,
  });
  await api.approvals.confirm(uninstall.action_ref);
  await expect(
    api.approvals.approve(uninstall.action_ref, "self"),
  ).rejects.toThrow("separation of duty");
  await api.auth.login({ username: "chenxi", password: "123456" });
  await api.approvals.approve(uninstall.action_ref, "reviewed");
  await api.auth.login({ username: "root", password: "123456" });
  expect(
    (await api.kubernetesWorkbench.helmReleases(cluster, namespace)).items,
  ).toMatchObject([{ revision: 3, status: "uninstalled" }]);
  const operations = await api.kubernetesWorkbench.operations(cluster);
  expect(
    operations.items.filter((r) => r.type === "helm_release"),
  ).toHaveLength(4);
});
