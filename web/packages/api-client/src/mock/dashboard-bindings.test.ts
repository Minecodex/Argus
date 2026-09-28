import { expect, it } from "vitest";
import { createMockApiClient } from "./index";
import { emptyDashboardSpec } from "../dashboard";
it("keeps resource shortcuts separate from grants, publication and queries", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  await api.auth.login({ username: "root", password: "123456" });
  const draft = await api.dashboards.createDraft({
    name: "Bound",
    description: "",
    spec: emptyDashboardSpec(),
    proposed_bindings: [],
  });
  await api.approvals.confirm(
    (await api.dashboards.preview(draft.id, 1)).action_ref,
  );
  const dashboard = (await api.dashboards.list())[0]!,
    host = (await api.hosts.list()).items[0]!,
    cluster = (await api.kubernetes.listClusters()).items[0]!;
  for (const [type, id] of [
    ["host", host.id],
    ["kubernetes_cluster", cluster.id],
  ] as const) {
    const scope = await api.dashboards.resourceBindings(type, id);
    const input = {
      operation: "attach" as const,
      dashboard_id: dashboard.id,
      expected_dashboard_version: dashboard.version,
      expected_resource_version: scope.resource_version,
    };
    const first = await api.dashboards.previewBinding(type, id, input),
      second = await api.dashboards.previewBinding(type, id, input);
    expect(
      (await api.dashboards.resourceBindings(type, id)).items,
    ).toHaveLength(0);
    await api.approvals.confirm(first.action_ref);
    await expect(
      api.approvals.confirm(second.action_ref),
    ).rejects.toMatchObject({ code: "DASHBOARD_VERSION_CONFLICT" });
  }
  expect(await api.dashboards.bindings(dashboard.id)).toHaveLength(2);
  expect((await api.dashboards.get(dashboard.id)).dashboard.version).toBe(
    dashboard.version,
  );
  const linked = (await api.dashboards.resourceBindings("host", host.id))
    .items[0]!;
  const detach = await api.dashboards.previewBinding("host", host.id, {
    operation: "detach",
    dashboard_id: dashboard.id,
    expected_dashboard_version: dashboard.version,
    expected_resource_version: linked.resource_version,
    binding_id: linked.id,
    expected_binding_version: linked.version,
  });
  await api.approvals.confirm(detach.action_ref);
  expect(await api.dashboards.bindings(dashboard.id)).toHaveLength(1);
  expect((await api.dashboards.get(dashboard.id)).revision.spec).toEqual(
    emptyDashboardSpec(),
  );
  await api.org.updateDataAuthorization(
    "user",
    "u-chenxi",
    "dashboard",
    [dashboard.id],
    false,
    1,
  );
  await api.auth.logout();
  await api.auth.login({ username: "chenxi", password: "123456" });
  expect((await api.dashboards.list()).map((d) => d.id)).toContain(
    dashboard.id,
  );
  await expect(api.dashboards.execute(dashboard.id, {})).rejects.toMatchObject({
    code: "DASHBOARD_DENIED",
  });
  await expect(
    api.dashboards.resourceBindings("kubernetes_cluster", cluster.id),
  ).rejects.toMatchObject({ code: "DASHBOARD_DENIED" });
});
