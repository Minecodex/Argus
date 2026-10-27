import { expect, it } from "vitest";
import { createMockApiClient } from "./index";

it("offers a read-only native default and creates an independent personal copy", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  await api.auth.login({ username: "root", password: "123456" });
  const collector = (
    await api.monitoring.collectors({ limit: 100 })
  ).items.find(
    (row) => row.resource.kind === "host" && row.components.includes("mysql"),
  )!;
  const action = await api.monitoring.preview({
    kind: "collection",
    operation: "create",
    collection: {
      name: "Managed default fixture",
      target_name: "Managed MySQL",
      integration: "mysql",
      target_identity: { endpoint: "db.test:3306" },
      collector_id: collector.id,
      config: { interval: "30s", timeout: "10s" },
      enabled: true,
      credential_update: "replace",
    },
    credentials: { username: "monitor", password: "fixture-only" },
  });
  await api.approvals.confirm(action.action_ref);
  const managed = (await api.dashboards.list()).find(
    (row) => row.managed_template_key === "mysql.overview",
  )!;
  expect(managed).toBeTruthy();
  const current = await api.dashboards.get(managed.id);
  expect(current.revision.spec.panels).toHaveLength(6);
  await expect(
    api.dashboards.previewLifecycle(managed.id, {
      operation: "archive",
      expected_version: managed.version,
      name: managed.name,
      description: managed.description,
      sort_order: 0,
    }),
  ).rejects.toMatchObject({ code: "DASHBOARD_DENIED", status: 403 });
  const copy = await api.dashboards.createDraft({
    dashboard_id: managed.id,
    name: "",
    description: "",
    spec: current.revision.spec,
    proposed_bindings: [],
  });
  expect(copy.dashboard_id).toBeUndefined();
  expect(copy.base_object_version).toBe(0);
  expect(copy.source_template_key).toBe("mysql.overview");
  const updated = structuredClone(copy.spec);
  updated.panels[0]!.title = "My custom panel";
  await api.dashboards.saveDraft(copy.id, {
    expected_version: copy.draft_version,
    name: copy.name,
    description: copy.description,
    spec: updated,
    proposed_bindings: [],
  });
  expect(
    (await api.dashboards.get(managed.id)).revision.spec.panels[0]!.title,
  ).not.toBe("My custom panel");
});
