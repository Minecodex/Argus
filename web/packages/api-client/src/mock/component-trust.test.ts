import { expect, it } from "vitest";
import { createMockApiClient } from "./index";

it("preserves component trust across independent builtin switches", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  await api.auth.login({ username: "root", password: "123456" });
  const target = { kind: "kubernetes_cluster" as const, id: "k8s-prod-east" };
  let scheduler = (await api.monitoring.builtins(target)).find(
    (row) => row.definition.key === "kubernetes_scheduler_metrics",
  )!;
  expect(scheduler.definition.component_tls_supported).toBe(true);
  let action = await api.monitoring.previewBuiltin({
    target,
    key: scheduler.definition.key,
    expected_version: 0,
    enabled: false,
    config: {
      interval: "30s",
      component_tls: { server_name: "scheduler.internal", ca_pem: "" },
    },
  });
  await api.approvals.confirm(action.action_ref);
  scheduler = (await api.monitoring.builtins(target)).find(
    (row) => row.id === scheduler.id,
  )!;
  expect(scheduler.collection?.config.component_tls).toMatchObject({
    server_name: "scheduler.internal",
  });
  action = await api.monitoring.previewBuiltin({
    target,
    key: scheduler.definition.key,
    expected_version: scheduler.collection!.version,
    enabled: true,
  });
  await api.approvals.confirm(action.action_ref);
  scheduler = (await api.monitoring.builtins(target)).find(
    (row) => row.id === scheduler.id,
  )!;
  expect(scheduler.collection?.desired_enabled).toBe(true);
  expect(scheduler.collection?.config.component_tls).toMatchObject({
    server_name: "scheduler.internal",
  });
});
